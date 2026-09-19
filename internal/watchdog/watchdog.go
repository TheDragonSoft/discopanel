// Package watchdog watches Minecraft server containers and automatically
// restarts them after unexpected crashes, with exponential backoff.
package watchdog

import (
	"context"
	"sync"
	"time"

	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/internal/events"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

const (
	// pollInterval is how often container states are compared against the
	// last-known state.
	pollInterval = 15 * time.Second

	// stabilityWindow is how long a server must stay up before its
	// consecutive-crash counter is reset.
	stabilityWindow = 5 * time.Minute

	// intentionalStopTTL is how long a user-initiated stop marker stays
	// valid. StopServer sets the marker just before stopping the container;
	// the exit is observed by the watchdog at most ~pollInterval later, so a
	// generous TTL absorbs slow stops without staying sticky forever.
	intentionalStopTTL = 10 * time.Minute

	// backoffCapMultiplier caps the backoff at base * 10.
	backoffCapMultiplier = 10
)

// nextBackoff computes the delay before the nth consecutive restart
// (n starts at 1): base doubled per consecutive crash, capped at 10x base.
// Pure function so it can be unit tested.
func nextBackoff(baseSecs, consecutive int) time.Duration {
	base := time.Duration(baseSecs) * time.Second
	if base <= 0 {
		base = 30 * time.Second
	}
	capD := base * backoffCapMultiplier

	backoff := base
	for i := 1; i < consecutive; i++ {
		backoff *= 2
		if backoff >= capD {
			return capD
		}
	}
	if backoff > capD {
		return capD
	}
	return backoff
}

// watchState is the watchdog's last-known view of one server.
type watchState struct {
	status storage.ServerStatus
	// upSince is when the server was first observed in its current "up"
	// (running/starting/unhealthy) streak; used for counter resets.
	upSince time.Time
}

// Watchdog restarts servers whose containers exit unexpectedly.
type Watchdog struct {
	store  *storage.Store
	docker *docker.Client
	bus    *events.Bus
	log    *logger.Logger

	mu     sync.Mutex
	states map[string]*watchState
	// intentionalStops records user-initiated stop requests (serverID ->
	// marker time). MarkIntentionalStop is called from the RPC layer.
	intentionalStops map[string]time.Time
	// consecutiveCrashes counts consecutive auto-restarts per server.
	consecutiveCrashes map[string]int
	// restarting guards against overlapping restarts of the same server.
	restarting map[string]bool

	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New creates a watchdog.
func New(store *storage.Store, dockerClient *docker.Client, bus *events.Bus, log *logger.Logger) *Watchdog {
	return &Watchdog{
		store:              store,
		docker:             dockerClient,
		bus:                bus,
		log:                log,
		states:             make(map[string]*watchState),
		intentionalStops:   make(map[string]time.Time),
		consecutiveCrashes: make(map[string]int),
		restarting:         make(map[string]bool),
		stopChan:           make(chan struct{}),
	}
}

// MarkIntentionalStop records that a server is being stopped on purpose, so
// the watchdog will not treat the upcoming container exit as a crash. Called
// from the RPC layer (StopServer, RestartServer, RecreateServer, DeleteServer).
func (w *Watchdog) MarkIntentionalStop(serverID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.intentionalStops[serverID] = time.Now()
}

// Start begins the watchdog polling loop.
func (w *Watchdog) Start() {
	w.wg.Add(1)
	go w.loop()
	w.log.Info("Crash watchdog started (poll interval %s)", pollInterval)
}

// Stop halts the watchdog.
func (w *Watchdog) Stop() {
	w.stopOnce.Do(func() { close(w.stopChan) })
	w.wg.Wait()
	w.log.Info("Crash watchdog stopped")
}

func (w *Watchdog) loop() {
	defer w.wg.Done()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Seed baselines immediately so already-running servers don't emit on boot
	w.poll()

	for {
		select {
		case <-ticker.C:
			w.poll()
		case <-w.stopChan:
			return
		}
	}
}

// poll compares every tracked server's current container status against the
// last-known state and reacts to unexpected exits.
func (w *Watchdog) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	servers, err := w.store.ListServers(ctx)
	if err != nil {
		w.log.Debug("Watchdog: failed to list servers: %v", err)
		return
	}

	now := time.Now()
	seen := make(map[string]bool, len(servers))

	for _, server := range servers {
		if server.ContainerID == "" {
			continue
		}
		seen[server.ID] = true

		status, err := w.docker.GetContainerStatus(ctx, server.ContainerID)
		if err != nil {
			// Container may be mid-recreation; keep the previous state.
			w.log.Debug("Watchdog: failed to get status for %s: %v", server.ID, err)
			continue
		}

		w.evaluate(ctx, server, status, now)
	}

	// Forget state for deleted servers
	w.mu.Lock()
	for id := range w.states {
		if !seen[id] {
			delete(w.states, id)
			delete(w.consecutiveCrashes, id)
			delete(w.intentionalStops, id)
		}
	}
	w.mu.Unlock()
}

// evaluate processes one state observation for a server.
func (w *Watchdog) evaluate(ctx context.Context, server *storage.Server, status storage.ServerStatus, now time.Time) {
	w.mu.Lock()
	prev, known := w.states[server.ID]
	if !known {
		// First sighting: seed the baseline without reacting.
		w.states[server.ID] = &watchState{status: status, upSince: now}
		w.mu.Unlock()
		return
	}

	// Expire stale intentional-stop markers
	if t, ok := w.intentionalStops[server.ID]; ok && now.Sub(t) > intentionalStopTTL {
		delete(w.intentionalStops, server.ID)
	}
	_, intentional := w.intentionalStops[server.ID]
	crashes := w.consecutiveCrashes[server.ID]
	_, busy := w.restarting[server.ID]

	up := status == storage.StatusRunning || status == storage.StatusStarting || status == storage.StatusUnhealthy
	prevUp := prev.status == storage.StatusRunning || prev.status == storage.StatusStarting || prev.status == storage.StatusUnhealthy

	// Reset the consecutive-crash counter after a stability window
	if up && crashes > 0 && !prev.upSince.IsZero() && now.Sub(prev.upSince) >= stabilityWindow {
		crashes = 0
		delete(w.consecutiveCrashes, server.ID)
		w.log.Info("Watchdog: server %s stable for %s, crash counter reset", server.ID, stabilityWindow)
	}

	w.states[server.ID] = &watchState{status: status, upSince: prev.upSince}
	if up && !prevUp {
		w.states[server.ID].upSince = now
	}
	w.mu.Unlock()

	// Only react to a down transition while auto-restart is enabled
	if !up && prevUp && !busy {
		if server.Detached {
			w.log.Debug("Watchdog: detached server %s exited, ignoring", server.ID)
			return
		}
		if !server.AutoRestart {
			return
		}
		if intentional {
			// User asked for this stop - not a crash.
			w.mu.Lock()
			delete(w.intentionalStops, server.ID)
			delete(w.consecutiveCrashes, server.ID)
			w.mu.Unlock()
			w.log.Info("Watchdog: server %s stopped intentionally, not restarting", server.ID)
			return
		}
		w.handleCrash(ctx, server, crashes+1)
	}
}

// handleCrash schedules (or refuses) an auto-restart for a crashed server.
func (w *Watchdog) handleCrash(ctx context.Context, server *storage.Server, consecutive int) {
	// Re-read the server: AutoRestart may have been toggled while we polled
	fresh, err := w.store.GetServer(ctx, server.ID)
	if err != nil {
		w.log.Debug("Watchdog: server %s vanished, not restarting", server.ID)
		return
	}
	if !fresh.AutoRestart || fresh.Detached {
		return
	}

	// Respect the consecutive-crash limit (0 = unlimited)
	if fresh.AutoRestartMaxRetries > 0 && consecutive > fresh.AutoRestartMaxRetries {
		w.log.Error("Watchdog: server %s (%s) crashed %d consecutive times, exceeding auto_restart_max_retries (%d) - GIVING UP, marking server as errored",
			fresh.Name, fresh.ID, consecutive, fresh.AutoRestartMaxRetries)
		fresh.Status = storage.StatusError
		if err := w.store.UpdateServer(ctx, fresh); err != nil {
			w.log.Error("Watchdog: failed to mark server %s as errored: %v", fresh.ID, err)
		}
		w.mu.Lock()
		delete(w.consecutiveCrashes, fresh.ID)
		w.mu.Unlock()
		return
	}

	backoff := nextBackoff(fresh.AutoRestartBackoffSecs, consecutive)
	w.log.Warn("Watchdog: server %s (%s) exited unexpectedly (consecutive crash #%d), auto-restarting in %s",
		fresh.Name, fresh.ID, consecutive, backoff)

	w.mu.Lock()
	w.consecutiveCrashes[fresh.ID] = consecutive
	w.restarting[fresh.ID] = true
	w.mu.Unlock()

	w.wg.Add(1)
	go func(serverID string, containerID string, consecutive int, backoff time.Duration) {
		defer w.wg.Done()
		defer func() {
			w.mu.Lock()
			delete(w.restarting, serverID)
			w.mu.Unlock()
		}()

		select {
		case <-time.After(backoff):
		case <-w.stopChan:
			return
		}

		restartCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Re-read: the server may have been stopped, deleted or reconfigured
		// during the backoff window.
		current, err := w.store.GetServer(restartCtx, serverID)
		if err != nil {
			w.log.Debug("Watchdog: server %s vanished during backoff, aborting restart", serverID)
			return
		}
		if !current.AutoRestart || current.Detached {
			w.log.Info("Watchdog: auto-restart disabled for server %s during backoff, aborting", serverID)
			w.mu.Lock()
			delete(w.consecutiveCrashes, serverID)
			w.mu.Unlock()
			return
		}
		if current.ContainerID == "" || current.ContainerID != containerID {
			w.log.Info("Watchdog: container for server %s changed during backoff, aborting restart", serverID)
			return
		}
		status, err := w.docker.GetContainerStatus(restartCtx, containerID)
		if err == nil && (status == storage.StatusRunning || status == storage.StatusStarting || status == storage.StatusUnhealthy) {
			// Someone else (user or wake-on-connect) started it meanwhile.
			w.log.Info("Watchdog: server %s already running again, aborting restart", serverID)
			w.mu.Lock()
			delete(w.consecutiveCrashes, serverID)
			w.mu.Unlock()
			return
		}

		if err := w.restartServer(restartCtx, current); err != nil {
			w.log.Error("Watchdog: failed to auto-restart server %s: %v", current.Name, err)
			current.Status = storage.StatusError
			if err := w.store.UpdateServer(restartCtx, current); err != nil {
				w.log.Error("Watchdog: failed to mark server %s as errored: %v", current.ID, err)
			}
			return
		}

		w.log.Info("Watchdog: server %s auto-restarted (consecutive crash #%d)", current.Name, consecutive)
	}(fresh.ID, fresh.ContainerID, consecutive, backoff)
}

// restartServer runs the same start path used by StartServer: container
// start, status update, and proxy route refresh.
func (w *Watchdog) restartServer(ctx context.Context, server *storage.Server) error {
	if err := w.docker.StartContainer(ctx, server.ContainerID); err != nil {
		return err
	}

	now := time.Now()
	server.Status = storage.StatusStarting
	server.LastStarted = &now
	if err := w.store.UpdateServer(ctx, server); err != nil {
		return err
	}

	// Announce the auto-restart on the event bus so webhooks/alerts see it
	if w.bus != nil {
		w.bus.Emit(ctx, events.Event{
			Type:     v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_SERVER_RESTART,
			ServerID: server.ID,
			Data:     map[string]any{"auto_restart": true, "reason": "crash"},
		})
	}
	return nil
}
