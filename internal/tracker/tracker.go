// Package tracker observes player connections made through the Minecraft
// proxy, keeps the online state in memory and persists player/session rows to
// the database.
package tracker

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/pkg/logger"
)

// OnlineSession is a snapshot of one currently connected player connection.
type OnlineSession struct {
	SessionID  string    // Same ID as the persisted PlayerSession row
	PlayerID   string    // Player row ID (empty until the row is confirmed)
	Name       string    // Minecraft username
	ServerID   string    // Server the connection is routed to
	RemoteAddr string    // Client address as seen by the proxy
	JoinedAt   time.Time // When the Login Start packet was observed
}

// onlineConn is the in-memory state of one tracked connection.
type onlineConn struct {
	OnlineSession
	bytesIn  int64
	bytesOut int64
}

// Tracker records player sessions observed by the proxy.
//
// Session persistence strategy: the session row is inserted at connect time
// (so it survives a crash) and finalized at disconnect; sessions still open
// after a restart are closed by CloseStaleSessions (called from Start).
type Tracker struct {
	store  *db.Store
	log    *logger.Logger
	mu     sync.Mutex
	online map[string]*onlineConn // keyed by session ID
}

// NewTracker creates a new player tracker.
func NewTracker(store *db.Store, log *logger.Logger) *Tracker {
	return &Tracker{
		store:  store,
		log:    log,
		online: make(map[string]*onlineConn),
	}
}

// Start performs startup recovery, closing any sessions left open by a
// previous panel run.
func (t *Tracker) Start() error {
	return t.CloseStaleSessions(context.Background())
}

// CloseStaleSessions marks every session with a NULL left_at as closed at the
// given time, computing its duration from joined_at.
func (t *Tracker) CloseStaleSessions(ctx context.Context) error {
	closed, err := t.store.CloseStalePlayerSessions(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	if closed > 0 {
		t.log.Info("Player tracker: closed %d stale session(s) from a previous run", closed)
	}
	return nil
}

// OnPlayerConnect registers a new player connection: upserts the Player row,
// inserts an open PlayerSession row and adds the connection to the online
// state. It returns the opaque session identifier the proxy must pass to
// OnBytes/OnPlayerDisconnect. If persistence fails, an empty ID is returned
// and the connection is simply not tracked.
func (t *Tracker) OnPlayerConnect(ctx context.Context, serverID, username, remoteAddr string) (string, error) {
	player, err := t.store.UpsertPlayer(ctx, username)
	if err != nil {
		t.log.Error("Player tracker: failed to upsert player %s: %v", username, err)
		return "", err
	}

	now := time.Now().UTC()
	session := &db.PlayerSession{
		ID:       uuid.New().String(),
		PlayerID: player.ID,
		ServerID: serverID,
		JoinedAt: now,
	}
	if err := t.store.CreatePlayerSession(ctx, session); err != nil {
		t.log.Error("Player tracker: failed to create session for %s on %s: %v", username, serverID, err)
		return "", err
	}

	t.mu.Lock()
	t.online[session.ID] = &onlineConn{
		OnlineSession: OnlineSession{
			SessionID:  session.ID,
			PlayerID:   player.ID,
			Name:       username,
			ServerID:   serverID,
			RemoteAddr: remoteAddr,
			JoinedAt:   now,
		},
	}
	t.mu.Unlock()

	t.log.Info("Player %s connected to server %s from %s", username, serverID, remoteAddr)
	return session.ID, nil
}

// OnBytes accumulates transferred byte counters for a session. Call sites may
// report once at disconnect; counters stay in memory and are persisted when
// the session is closed.
func (t *Tracker) OnBytes(sessionID string, in, out int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if conn, ok := t.online[sessionID]; ok {
		conn.bytesIn += in
		conn.bytesOut += out
	}
}

// OnPlayerDisconnect closes a session: computes its duration, persists the
// row (with accumulated byte counters) and removes the connection from the
// online state.
func (t *Tracker) OnPlayerDisconnect(sessionID string) {
	if sessionID == "" {
		return
	}

	t.mu.Lock()
	conn, ok := t.online[sessionID]
	if ok {
		delete(t.online, sessionID)
	}
	t.mu.Unlock()
	if !ok {
		return
	}

	leftAt := time.Now().UTC()
	duration := int64(leftAt.Sub(conn.JoinedAt).Seconds())
	if duration < 0 {
		duration = 0
	}
	if err := t.store.ClosePlayerSession(context.Background(), sessionID, leftAt, duration, conn.bytesIn, conn.bytesOut); err != nil {
		t.log.Error("Player tracker: failed to close session %s for %s: %v", sessionID, conn.Name, err)
		return
	}
	t.log.Info("Player %s disconnected from server %s (session %s, %ds, in %d bytes, out %d bytes)",
		conn.Name, conn.ServerID, sessionID, duration, conn.bytesIn, conn.bytesOut)
}

// ListOnline returns a snapshot of all currently online player connections,
// optionally filtered by server (empty serverID = all).
func (t *Tracker) ListOnline(serverID string) []OnlineSession {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]OnlineSession, 0, len(t.online))
	for _, conn := range t.online {
		if serverID != "" && conn.ServerID != serverID {
			continue
		}
		out = append(out, conn.OnlineSession)
	}
	return out
}

// IsOnline reports whether the given player (by name) currently has at least
// one tracked connection.
func (t *Tracker) IsOnline(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, conn := range t.online {
		if conn.Name == name {
			return true
		}
	}
	return false
}
