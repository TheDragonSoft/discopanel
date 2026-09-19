package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/nickheyer/discopanel/internal/metrics"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that MetricService implements the interface
var _ discopanelv1connect.MetricServiceHandler = (*MetricService)(nil)

// MetricService implements the Metric service
type MetricService struct {
	store     *storage.Store
	collector *metrics.Collector
	log       *logger.Logger
}

// NewMetricService creates a new metric service
func NewMetricService(store *storage.Store, collector *metrics.Collector, log *logger.Logger) *MetricService {
	return &MetricService{
		store:     store,
		collector: collector,
		log:       log,
	}
}

// dbAlertMetricToProto converts a database metric name to proto
func dbAlertMetricToProto(m string) v1.AlertMetric {
	switch m {
	case metrics.MetricCPUPercent:
		return v1.AlertMetric_ALERT_METRIC_CPU_PERCENT
	case metrics.MetricMemoryPercent:
		return v1.AlertMetric_ALERT_METRIC_MEMORY_PERCENT
	case metrics.MetricTPS:
		return v1.AlertMetric_ALERT_METRIC_TPS
	case metrics.MetricPlayersOnline:
		return v1.AlertMetric_ALERT_METRIC_PLAYERS_ONLINE
	case metrics.MetricDiskPercent:
		return v1.AlertMetric_ALERT_METRIC_DISK_PERCENT
	default:
		return v1.AlertMetric_ALERT_METRIC_UNSPECIFIED
	}
}

// protoAlertMetricToDB converts a proto metric enum to a database name
func protoAlertMetricToDB(m v1.AlertMetric) string {
	switch m {
	case v1.AlertMetric_ALERT_METRIC_CPU_PERCENT:
		return metrics.MetricCPUPercent
	case v1.AlertMetric_ALERT_METRIC_MEMORY_PERCENT:
		return metrics.MetricMemoryPercent
	case v1.AlertMetric_ALERT_METRIC_TPS:
		return metrics.MetricTPS
	case v1.AlertMetric_ALERT_METRIC_PLAYERS_ONLINE:
		return metrics.MetricPlayersOnline
	case v1.AlertMetric_ALERT_METRIC_DISK_PERCENT:
		return metrics.MetricDiskPercent
	default:
		return ""
	}
}

// dbAlertComparatorToProto converts a database comparator name to proto
func dbAlertComparatorToProto(c string) v1.AlertComparator {
	switch c {
	case metrics.ComparatorBelow:
		return v1.AlertComparator_ALERT_COMPARATOR_BELOW
	case metrics.ComparatorAbove:
		return v1.AlertComparator_ALERT_COMPARATOR_ABOVE
	default:
		return v1.AlertComparator_ALERT_COMPARATOR_UNSPECIFIED
	}
}

// protoAlertComparatorToDB converts a proto comparator enum to a database name
func protoAlertComparatorToDB(c v1.AlertComparator) string {
	switch c {
	case v1.AlertComparator_ALERT_COMPARATOR_BELOW:
		return metrics.ComparatorBelow
	case v1.AlertComparator_ALERT_COMPARATOR_ABOVE:
		return metrics.ComparatorAbove
	default:
		return ""
	}
}

// dbAlertStateToProto converts a database alert state to proto
func dbAlertStateToProto(s string) v1.AlertState {
	switch s {
	case metrics.AlertStateFiring:
		return v1.AlertState_ALERT_STATE_FIRING
	case metrics.AlertStateResolved:
		return v1.AlertState_ALERT_STATE_RESOLVED
	default:
		return v1.AlertState_ALERT_STATE_UNSPECIFIED
	}
}

// dbAlertRuleToProto converts a database alert rule to proto
func dbAlertRuleToProto(rule *storage.AlertRule) *v1.AlertRule {
	if rule == nil {
		return nil
	}
	return &v1.AlertRule{
		Id:           rule.ID,
		Name:         rule.Name,
		Description:  rule.Description,
		ServerId:     rule.ServerID,
		Metric:       dbAlertMetricToProto(rule.Metric),
		Comparator:   dbAlertComparatorToProto(rule.Comparator),
		Threshold:    rule.Threshold,
		DurationSecs: int32(rule.DurationSecs),
		CooldownSecs: int32(rule.CooldownSecs),
		Enabled:      rule.Enabled,
		CreatedAt:    timestamppb.New(rule.CreatedAt),
		UpdatedAt:    timestamppb.New(rule.UpdatedAt),
	}
}

// dbAlertEventToProto converts a database alert event to proto
func dbAlertEventToProto(event *storage.AlertEventRecord) *v1.AlertEvent {
	if event == nil {
		return nil
	}
	return &v1.AlertEvent{
		Id:        event.ID,
		RuleId:    event.RuleID,
		RuleName:  event.RuleName,
		ServerId:  event.ServerID,
		State:     dbAlertStateToProto(event.State),
		Value:     event.Value,
		Threshold: event.Threshold,
		Message:   event.Message,
		CreatedAt: timestamppb.New(event.CreatedAt),
	}
}

// dbMetricSampleToProto converts a database metric sample to proto
func dbMetricSampleToProto(sample *storage.MetricSampleRecord) *v1.MetricSample {
	if sample == nil {
		return nil
	}
	return &v1.MetricSample{
		ServerId:      sample.ServerID,
		Timestamp:     timestamppb.New(sample.Timestamp),
		CpuPercent:    sample.CPUPercent,
		MemoryUsage:   sample.MemoryUsage,
		MemoryTotal:   sample.MemoryTotal,
		Tps:           sample.TPS,
		PlayersOnline: int32(sample.PlayersOnline),
		DiskUsage:     sample.DiskUsage,
		DiskTotal:     sample.DiskTotal,
	}
}

// ListMetricHistory lists recorded metric samples for a server
func (s *MetricService) ListMetricHistory(ctx context.Context, req *connect.Request[v1.ListMetricHistoryRequest]) (*connect.Response[v1.ListMetricHistoryResponse], error) {
	msg := req.Msg

	if msg.ServerId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server_id is required"))
	}

	rangeSecs := int(msg.RangeSecs)
	if rangeSecs <= 0 {
		rangeSecs = 3600 // default 1 hour
	}
	since := time.Now().UTC().Add(-time.Duration(rangeSecs) * time.Second)

	samples, err := s.store.ListMetricSamples(ctx, msg.ServerId, since, int(msg.MaxPoints))
	if err != nil {
		s.log.Error("Failed to list metric samples: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list metric samples"))
	}

	protoSamples := make([]*v1.MetricSample, len(samples))
	for i, sample := range samples {
		protoSamples[i] = dbMetricSampleToProto(sample)
	}

	return connect.NewResponse(&v1.ListMetricHistoryResponse{
		Samples: protoSamples,
	}), nil
}

// ListAlertRules lists all alert rules, optionally filtered by server
func (s *MetricService) ListAlertRules(ctx context.Context, req *connect.Request[v1.ListAlertRulesRequest]) (*connect.Response[v1.ListAlertRulesResponse], error) {
	rules, err := s.store.ListAlertRules(ctx, req.Msg.ServerId)
	if err != nil {
		s.log.Error("Failed to list alert rules: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list alert rules"))
	}

	protoRules := make([]*v1.AlertRule, len(rules))
	for i, rule := range rules {
		protoRules[i] = dbAlertRuleToProto(rule)
	}

	return connect.NewResponse(&v1.ListAlertRulesResponse{
		Rules: protoRules,
	}), nil
}

// GetAlertRule gets a specific alert rule
func (s *MetricService) GetAlertRule(ctx context.Context, req *connect.Request[v1.GetAlertRuleRequest]) (*connect.Response[v1.GetAlertRuleResponse], error) {
	rule, err := s.store.GetAlertRule(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alert rule not found"))
	}

	return connect.NewResponse(&v1.GetAlertRuleResponse{
		Rule: dbAlertRuleToProto(rule),
	}), nil
}

// CreateAlertRule creates a new alert rule
func (s *MetricService) CreateAlertRule(ctx context.Context, req *connect.Request[v1.CreateAlertRuleRequest]) (*connect.Response[v1.CreateAlertRuleResponse], error) {
	msg := req.Msg

	// Validate required fields
	if msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	metric := protoAlertMetricToDB(msg.Metric)
	if metric == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid metric is required"))
	}
	comparator := protoAlertComparatorToDB(msg.Comparator)
	if comparator == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid comparator is required"))
	}

	// Validate server exists if scoped to one
	if msg.ServerId != "" {
		if _, err := s.store.GetServer(ctx, msg.ServerId); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
		}
	}

	rule := &storage.AlertRule{
		Name:         msg.Name,
		Description:  msg.Description,
		ServerID:     msg.ServerId,
		Metric:       metric,
		Comparator:   comparator,
		Threshold:    msg.Threshold,
		DurationSecs: int(msg.DurationSecs),
		CooldownSecs: int(msg.CooldownSecs),
		Enabled:      msg.Enabled,
	}

	if err := s.store.CreateAlertRule(ctx, rule); err != nil {
		s.log.Error("Failed to create alert rule: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create alert rule"))
	}

	s.log.Info("Created alert rule: %s (metric %s %s %v)", rule.Name, rule.Metric, rule.Comparator, rule.Threshold)

	return connect.NewResponse(&v1.CreateAlertRuleResponse{
		Rule: dbAlertRuleToProto(rule),
	}), nil
}

// UpdateAlertRule updates an existing alert rule (partial update semantics)
func (s *MetricService) UpdateAlertRule(ctx context.Context, req *connect.Request[v1.UpdateAlertRuleRequest]) (*connect.Response[v1.UpdateAlertRuleResponse], error) {
	msg := req.Msg

	rule, err := s.store.GetAlertRule(ctx, msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alert rule not found"))
	}

	// Update fields if provided
	if msg.Name != nil {
		rule.Name = *msg.Name
	}
	if msg.Description != nil {
		rule.Description = *msg.Description
	}
	if msg.ServerId != nil {
		if *msg.ServerId != "" {
			if _, err := s.store.GetServer(ctx, *msg.ServerId); err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
			}
		}
		rule.ServerID = *msg.ServerId
	}
	if msg.Metric != nil {
		metric := protoAlertMetricToDB(*msg.Metric)
		if metric == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid metric is required"))
		}
		rule.Metric = metric
	}
	if msg.Comparator != nil {
		comparator := protoAlertComparatorToDB(*msg.Comparator)
		if comparator == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid comparator is required"))
		}
		rule.Comparator = comparator
	}
	if msg.Threshold != nil {
		rule.Threshold = *msg.Threshold
	}
	if msg.DurationSecs != nil {
		rule.DurationSecs = int(*msg.DurationSecs)
	}
	if msg.CooldownSecs != nil {
		rule.CooldownSecs = int(*msg.CooldownSecs)
	}
	if msg.Enabled != nil {
		rule.Enabled = *msg.Enabled
	}

	if rule.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	if err := s.store.UpdateAlertRule(ctx, rule); err != nil {
		s.log.Error("Failed to update alert rule: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update alert rule"))
	}

	s.log.Info("Updated alert rule: %s", rule.Name)

	return connect.NewResponse(&v1.UpdateAlertRuleResponse{
		Rule: dbAlertRuleToProto(rule),
	}), nil
}

// DeleteAlertRule deletes an alert rule
func (s *MetricService) DeleteAlertRule(ctx context.Context, req *connect.Request[v1.DeleteAlertRuleRequest]) (*connect.Response[v1.DeleteAlertRuleResponse], error) {
	if _, err := s.store.GetAlertRule(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alert rule not found"))
	}

	if err := s.store.DeleteAlertRule(ctx, req.Msg.Id); err != nil {
		s.log.Error("Failed to delete alert rule: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete alert rule"))
	}

	s.log.Info("Deleted alert rule: %s", req.Msg.Id)

	return connect.NewResponse(&v1.DeleteAlertRuleResponse{}), nil
}

// TestAlertRule evaluates a rule definition against the server's current metrics without persisting state
func (s *MetricService) TestAlertRule(ctx context.Context, req *connect.Request[v1.TestAlertRuleRequest]) (*connect.Response[v1.TestAlertRuleResponse], error) {
	msg := req.Msg

	if msg.ServerId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server_id is required"))
	}
	metric := protoAlertMetricToDB(msg.Metric)
	if metric == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid metric is required"))
	}
	comparator := protoAlertComparatorToDB(msg.Comparator)
	if comparator == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a valid comparator is required"))
	}

	value, ok := s.collector.ComputeMetricValue(msg.ServerId, metric)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no metrics available for server %s", msg.ServerId))
	}

	wouldFire := value < msg.Threshold
	if comparator == metrics.ComparatorAbove {
		wouldFire = value > msg.Threshold
	}

	return connect.NewResponse(&v1.TestAlertRuleResponse{
		WouldFire:    wouldFire,
		CurrentValue: value,
	}), nil
}

// ListAlertEvents lists recent alert firing/resolution events
func (s *MetricService) ListAlertEvents(ctx context.Context, req *connect.Request[v1.ListAlertEventsRequest]) (*connect.Response[v1.ListAlertEventsResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50 // Default limit
	}

	events, err := s.store.ListAlertEvents(ctx, req.Msg.ServerId, limit)
	if err != nil {
		s.log.Error("Failed to list alert events: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list alert events"))
	}

	protoEvents := make([]*v1.AlertEvent, len(events))
	for i, event := range events {
		protoEvents[i] = dbAlertEventToProto(event)
	}

	return connect.NewResponse(&v1.ListAlertEventsResponse{
		Events: protoEvents,
	}), nil
}
