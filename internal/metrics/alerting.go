package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	storage "github.com/nickheyer/discopanel/internal/db"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

// Canonical metric names stored on AlertRule.Metric
const (
	MetricCPUPercent    = "cpu_percent"
	MetricMemoryPercent = "memory_percent"
	MetricTPS           = "tps"
	MetricPlayersOnline = "players_online"
	MetricDiskPercent   = "disk_percent"
)

// Canonical comparator names stored on AlertRule.Comparator
const (
	ComparatorBelow = "below"
	ComparatorAbove = "above"
)

// Alert event states stored on AlertEventRecord.State
const (
	AlertStateFiring   = "firing"
	AlertStateResolved = "resolved"
)

// Default cooldown between repeat firings when the rule doesn't specify one
const defaultCooldownSecs = 300

// alertState tracks the evaluation state of one (rule, server) pair
type alertState struct {
	ruleID      string
	ruleName    string
	metric      string
	comparator  string
	threshold   float64
	serverID    string
	serverName  string
	value       float64   // last observed metric value
	breachStart time.Time // start of the current continuous breach (zero = not breaching)
	firing      bool
	lastFired   time.Time // last time this pair fired (drives the cooldown)
}

// alertKey builds the state map key for a (rule, server) pair
func alertKey(ruleID, serverID string) string {
	return ruleID + "\x00" + serverID
}

// History recording

// recordHistoryLoop periodically persists in-memory metrics as history samples
// and prunes samples past the retention window
func (c *Collector) recordHistoryLoop() {
	defer c.wg.Done()

	sampleTicker := time.NewTicker(c.collectorConfig.SampleInterval)
	defer sampleTicker.Stop()
	pruneTicker := time.NewTicker(time.Hour)
	defer pruneTicker.Stop()

	for {
		select {
		case <-sampleTicker.C:
			c.recordSamples()
		case <-pruneTicker.C:
			c.pruneSamples()
		case <-c.stopChan:
			return
		}
	}
}

// recordSamples snapshots the in-memory metrics and batch-inserts sample rows
func (c *Collector) recordSamples() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Memory allocations come from server config
	servers, err := c.store.ListServers(ctx)
	if err != nil {
		c.log.Debug("Metrics history: failed to list servers: %v", err)
		return
	}
	memByServer := make(map[string]int, len(servers))
	nameByServer := make(map[string]string, len(servers))
	for _, server := range servers {
		memByServer[server.ID] = server.Memory
		nameByServer[server.ID] = server.Name
	}

	now := time.Now().UTC()

	c.mu.RLock()
	records := make([]*storage.MetricSampleRecord, 0, len(c.metrics))
	for _, m := range c.metrics {
		if m == nil {
			continue
		}
		records = append(records, &storage.MetricSampleRecord{
			ServerID:      m.ServerID,
			Timestamp:     now,
			CPUPercent:    m.CPUPercent,
			MemoryUsage:   m.MemoryUsage,
			MemoryTotal:   float64(memByServer[m.ServerID]),
			TPS:           m.TPS,
			PlayersOnline: m.PlayersOnline,
			DiskUsage:     m.DiskUsage,
			DiskTotal:     m.DiskTotal,
		})
	}
	c.mu.RUnlock()

	if len(records) == 0 {
		return
	}
	if err := c.store.InsertMetricSamples(ctx, records); err != nil {
		c.log.Debug("Metrics history: failed to insert %d samples: %v", len(records), err)
	}
}

// pruneSamples deletes history samples older than the retention window
func (c *Collector) pruneSamples() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deleted, err := c.store.PruneMetricSamples(ctx, time.Now().UTC().Add(-c.collectorConfig.Retention))
	if err != nil {
		c.log.Debug("Metrics history: failed to prune samples: %v", err)
		return
	}
	if deleted > 0 {
		c.log.Debug("Metrics history: pruned %d samples", deleted)
	}
}

// Alert evaluation

// evaluateAlertsLoop periodically evaluates enabled alert rules against live metrics
func (c *Collector) evaluateAlertsLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.collectorConfig.AlertInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.evaluateAlerts()
		case <-c.stopChan:
			return
		}
	}
}

// evaluateAlerts runs one evaluation pass over all enabled alert rules
func (c *Collector) evaluateAlerts() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rules, err := c.store.ListAlertRules(ctx, "")
	if err != nil {
		c.log.Debug("Alert evaluator: failed to list rules: %v", err)
		return
	}
	servers, err := c.store.ListServers(ctx)
	if err != nil {
		c.log.Debug("Alert evaluator: failed to list servers: %v", err)
		return
	}

	now := time.Now()
	liveKeys := make(map[string]bool)

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, server := range servers {
			if rule.ServerID != "" && rule.ServerID != server.ID {
				continue
			}
			key := alertKey(rule.ID, server.ID)
			liveKeys[key] = true

			// Only evaluate running servers; a stopped server resolves any firing alert
			running := false
			if server.ContainerID != "" {
				status, err := c.docker.GetContainerStatus(ctx, server.ContainerID)
				running = err == nil && status == storage.StatusRunning
			}
			if !running {
				c.resolveIfFiring(key, now, 0, "server stopped")
				continue
			}

			value, ok := c.metricValueForRule(rule.Metric, server)
			if !ok {
				// No usable data yet - don't start or advance a breach, but leave
				// an existing firing state alone until data returns
				continue
			}

			breaching := isBreaching(rule.Comparator, value, rule.Threshold)
			c.updateAlertState(key, rule, server.Name, value, breaching, now)
		}
	}

	// Re-sync: any state whose (enabled) rule pair vanished means the rule was
	// deleted or disabled while firing - resolve it
	c.alertMu.Lock()
	stale := make([]*alertState, 0, len(c.alertStates))
	for key := range c.alertStates {
		if !liveKeys[key] {
			stale = append(stale, c.alertStates[key])
			delete(c.alertStates, key)
		}
	}
	c.alertMu.Unlock()
	for _, st := range stale {
		if st.firing {
			c.fireAlertEvent(ctx, st, st.lastFired, AlertStateResolved, resolveMessage(st.metric, st.serverName, st.value))
		}
	}
}

// metricValueForRule computes the current value of a rule's metric for a server.
// Returns ok=false when the metric can't be evaluated right now.
func (c *Collector) metricValueForRule(metric string, server *storage.Server) (float64, bool) {
	m, ok := c.GetMetricSnapshot(server.ID)
	if !ok {
		return 0, false
	}
	switch metric {
	case MetricCPUPercent:
		return m.CPUPercent, true
	case MetricMemoryPercent:
		if server.Memory <= 0 {
			return 0, false
		}
		return m.MemoryUsage / float64(server.Memory) * 100, true
	case MetricTPS:
		if m.TPS <= 0 {
			return 0, false // TPS unknown (no RCON data yet)
		}
		return m.TPS, true
	case MetricPlayersOnline:
		return float64(m.PlayersOnline), true
	case MetricDiskPercent:
		if m.DiskTotal == 0 {
			return 0, false
		}
		return float64(m.DiskUsage) / float64(m.DiskTotal) * 100, true
	default:
		return 0, false
	}
}

// ComputeMetricValue evaluates a named metric against a server's current
// in-memory metrics. Exported for the MetricService TestAlertRule RPC.
// Returns ok=false when the metric can't be evaluated right now.
func (c *Collector) ComputeMetricValue(serverID, metric string) (float64, bool) {
	server, err := c.store.GetServer(context.Background(), serverID)
	if err != nil {
		return 0, false
	}
	return c.metricValueForRule(metric, server)
}

// isBreaching applies the comparator against the threshold
func isBreaching(comparator string, value, threshold float64) bool {
	switch comparator {
	case ComparatorBelow:
		return value < threshold
	case ComparatorAbove:
		return value > threshold
	default:
		return false
	}
}

// updateAlertState advances the per-(rule, server) state machine:
// not breaching -> breaching for DurationSecs -> firing; breach ends -> resolved.
// A fired alert may not re-fire until it resolves and its cooldown elapses.
func (c *Collector) updateAlertState(key string, rule *storage.AlertRule, serverName string, value float64, breaching bool, now time.Time) {
	c.alertMu.Lock()
	st, ok := c.alertStates[key]
	if !ok {
		st = &alertState{
			ruleID:     rule.ID,
			ruleName:   rule.Name,
			metric:     rule.Metric,
			comparator: rule.Comparator,
			threshold:  rule.Threshold,
			serverID:   rule.ServerID,
			serverName: serverName,
		}
		if st.serverID == "" {
			// Global rule - key on the actual server being evaluated
			st.serverID = serverIDFromKey(key)
		}
		c.alertStates[key] = st
	}
	// Refresh rule metadata so messages reflect the current rule
	st.ruleName = rule.Name
	st.metric = rule.Metric
	st.comparator = rule.Comparator
	st.threshold = rule.Threshold
	st.serverName = serverName

	cooldown := time.Duration(rule.CooldownSecs) * time.Second
	if cooldown <= 0 {
		cooldown = time.Duration(defaultCooldownSecs) * time.Second
	}

	if !breaching {
		if st.firing {
			st.firing = false
			st.breachStart = time.Time{}
			st.value = value // report the recovered value
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			c.fireAlertEvent(ctx, st, now, AlertStateResolved, resolveMessage(st.metric, st.serverName, value))
			cancel()
		} else if st.breachStart.IsZero() && !st.lastFired.IsZero() && now.Sub(st.lastFired) >= cooldown {
			// Long since recovered - forget the cooldown so the state can be cleaned up
			delete(c.alertStates, key)
		}
		c.alertMu.Unlock()
		return
	}

	// Breaching
	if st.firing {
		st.value = value
		c.alertMu.Unlock()
		return
	}

	if st.breachStart.IsZero() {
		// Fresh breach (or re-breach after a resolve) - start the duration clock,
		// gated by the cooldown from the previous firing if there was one
		if !st.lastFired.IsZero() && now.Sub(st.lastFired) < cooldown {
			c.alertMu.Unlock()
			return
		}
		st.breachStart = now
	}

	duration := time.Duration(rule.DurationSecs) * time.Second
	if now.Sub(st.breachStart) < duration {
		c.alertMu.Unlock()
		return
	}

	// Fire
	st.firing = true
	st.lastFired = now
	st.value = value
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	c.fireAlertEvent(ctx, st, now, AlertStateFiring, firingMessage(st.metric, st.comparator, st.serverName, value, st.threshold))
	cancel()
	c.alertMu.Unlock()
}

// resolveIfFiring resolves a firing state (e.g. the server stopped or the rule
// pairing disappeared) and persists the resolution
func (c *Collector) resolveIfFiring(key string, now time.Time, value float64, reason string) {
	c.alertMu.Lock()
	st, ok := c.alertStates[key]
	if !ok {
		c.alertMu.Unlock()
		return
	}
	if st.firing {
		st.firing = false
		st.breachStart = time.Time{}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		c.fireAlertEvent(ctx, st, now, AlertStateResolved, resolveMessage(st.metric, st.serverName, st.value)+" ("+reason+")")
		cancel()
	}
	c.alertMu.Unlock()
}

// fireAlertEvent emits an alert event on the bus and persists a record.
// Callers must hold c.alertMu (or otherwise guarantee st is not concurrently mutated).
func (c *Collector) fireAlertEvent(ctx context.Context, st *alertState, at time.Time, state string, message string) {
	eventType := v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_ALERT_TRIGGERED
	if state == AlertStateResolved {
		eventType = v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_ALERT_RESOLVED
	}

	c.emit(ctx, eventType, st.serverID, map[string]any{
		"rule_id":     st.ruleID,
		"rule_name":   st.ruleName,
		"metric":      st.metric,
		"value":       st.value,
		"threshold":   st.threshold,
		"message":     message,
		"server_name": st.serverName,
	})

	record := &storage.AlertEventRecord{
		RuleID:    st.ruleID,
		RuleName:  st.ruleName,
		ServerID:  st.serverID,
		State:     state,
		Value:     st.value,
		Threshold: st.threshold,
		Message:   message,
	}
	record.ID = uuid.New().String()
	record.CreatedAt = at.UTC()
	if err := c.store.InsertAlertEvent(ctx, record); err != nil {
		c.log.Debug("Alert evaluator: failed to persist %s event for rule %s: %v", state, st.ruleName, err)
	}
}

// serverIDFromKey extracts the server ID from an alert state key
func serverIDFromKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[i+1:]
		}
	}
	return ""
}

// Message crafting

// metricLabel returns a human-readable name for a metric
func metricLabel(metric string) string {
	switch metric {
	case MetricCPUPercent:
		return "CPU"
	case MetricMemoryPercent:
		return "memory"
	case MetricTPS:
		return "TPS"
	case MetricPlayersOnline:
		return "player count"
	case MetricDiskPercent:
		return "disk"
	default:
		return metric
	}
}

// formatMetricValue renders a value with its natural unit
func formatMetricValue(metric string, value float64) string {
	switch metric {
	case MetricTPS:
		return fmt.Sprintf("%.1f", value)
	case MetricPlayersOnline:
		return fmt.Sprintf("%.0f", value)
	default:
		return fmt.Sprintf("%.1f%%", value)
	}
}

// firingMessage crafts the message emitted when an alert fires,
// e.g. "High memory on MyServer: 92.3% > 90%"
func firingMessage(metric, comparator, serverName string, value, threshold float64) string {
	word := "High"
	op := ">"
	if comparator == ComparatorBelow {
		word = "Low"
		op = "<"
	}
	return fmt.Sprintf("%s %s on %s: %s %s %s",
		word, metricLabel(metric), serverName, formatMetricValue(metric, value), op, formatMetricValue(metric, threshold))
}

// resolveMessage crafts the message emitted when an alert resolves
func resolveMessage(metric, serverName string, value float64) string {
	return fmt.Sprintf("%s on %s back to normal: %s",
		metricLabel(metric), serverName, formatMetricValue(metric, value))
}
