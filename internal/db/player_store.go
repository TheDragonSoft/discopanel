package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Player tracking operations.
//
// Playtime and session counts are intentionally computed at query time via
// SQL aggregates instead of stored denormalized counters on the player row.

// UpsertPlayer returns the player row for the given username, creating it on
// first sight and refreshing last_seen in both cases.
func (s *Store) UpsertPlayer(ctx context.Context, name string) (*Player, error) {
	var player Player
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&player).Error
	if err == nil {
		if err := s.db.WithContext(ctx).Model(&Player{}).Where("id = ?", player.ID).
			Update("last_seen", time.Now().UTC()).Error; err != nil {
			return nil, fmt.Errorf("failed to update player last_seen: %w", err)
		}
		player.LastSeen = time.Now().UTC()
		return &player, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	player = Player{
		ID:       uuid.New().String(),
		Name:     name,
		LastSeen: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&player).Error; err != nil {
		// A concurrent connection for the same new player may have won the
		// unique index; retry as an update.
		var existing Player
		if findErr := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; findErr == nil {
			if updErr := s.db.WithContext(ctx).Model(&Player{}).Where("id = ?", existing.ID).
				Update("last_seen", time.Now().UTC()).Error; updErr != nil {
				return nil, fmt.Errorf("failed to update player last_seen: %w", updErr)
			}
			existing.LastSeen = time.Now().UTC()
			return &existing, nil
		}
		return nil, fmt.Errorf("failed to create player: %w", err)
	}
	return &player, nil
}

// GetPlayerByID returns a single player by ID.
func (s *Store) GetPlayerByID(ctx context.Context, id string) (*Player, error) {
	var player Player
	err := s.db.WithContext(ctx).First(&player, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("player not found")
		}
		return nil, err
	}
	return &player, nil
}

// PlayerAggregate is a player row joined with its computed session stats.
type PlayerAggregate struct {
	ID                string
	Name              string
	FirstSeen         time.Time
	LastSeen          time.Time
	TotalPlaytimeSecs int64
	SessionsCount     int64
}

// ListPlayersWithStats returns players with total playtime (completed sessions)
// and session count, ordered by last_seen descending. When search is non-empty
// only players whose name contains it are returned; when serverID is non-empty
// only players with at least one session on that server are returned.
func (s *Store) ListPlayersWithStats(ctx context.Context, search, serverID string, limit, offset int) ([]*PlayerAggregate, int64, error) {
	conds := "1 = 1"
	var args []any
	if search != "" {
		conds += " AND players.name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if serverID != "" {
		conds += " AND players.id IN (SELECT player_id FROM player_sessions WHERE server_id = ?)"
		args = append(args, serverID)
	}

	var total int64
	if err := s.db.WithContext(ctx).Model(&Player{}).Where(conds, args...).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT players.id, players.name, players.first_seen, players.last_seen,
		       COALESCE(SUM(player_sessions.duration_secs), 0) AS total_playtime_secs,
		       COUNT(player_sessions.id) AS sessions_count
		FROM players
		LEFT JOIN player_sessions ON player_sessions.player_id = players.id
		WHERE %s
		GROUP BY players.id, players.name, players.first_seen, players.last_seen
		ORDER BY players.last_seen DESC`, conds)

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	var players []*PlayerAggregate
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&players).Error; err != nil {
		return nil, 0, err
	}
	return players, total, nil
}

// PlayerServerStat is per-server aggregated playtime for a set of players.
type PlayerServerStat struct {
	PlayerID      string
	ServerID      string
	PlaytimeSecs  int64
	SessionsCount int64
	LastSeen      time.Time
}

// ListPlayerServerStats aggregates sessions per player and server for the
// given player IDs (in-progress sessions contribute zero duration).
func (s *Store) ListPlayerServerStats(ctx context.Context, playerIDs []string) ([]*PlayerServerStat, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	var stats []*PlayerServerStat
	err := s.db.WithContext(ctx).Model(&PlayerSession{}).
		Select(`player_id, server_id,
		       COALESCE(SUM(duration_secs), 0) AS playtime_secs,
		       COUNT(*) AS sessions_count,
		       MAX(joined_at) AS last_seen`).
		Where("player_id IN ?", playerIDs).
		Group("player_id, server_id").
		Scan(&stats).Error
	return stats, err
}

// PlayerSessionRecord is a session row with the display names of its player
// and server resolved (empty when the referenced row is gone).
type PlayerSessionRecord struct {
	ID           string
	PlayerID     string
	ServerID     string
	JoinedAt     time.Time
	LeftAt       *time.Time
	DurationSecs int64
	BytesIn      int64
	BytesOut     int64
	PlayerName   string
	ServerName   string
}

// ListPlayerSessions returns sessions ordered by joined_at descending, with
// optional player/server filters and player/server names resolved.
func (s *Store) ListPlayerSessions(ctx context.Context, playerID, serverID string, limit int) ([]*PlayerSessionRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT ps.id, ps.player_id, ps.server_id, ps.joined_at, ps.left_at,
		       ps.duration_secs, ps.bytes_in, ps.bytes_out,
		       COALESCE(p.name, '') AS player_name,
		       COALESCE(s.name, '') AS server_name
		FROM player_sessions ps
		LEFT JOIN players p ON p.id = ps.player_id
		LEFT JOIN servers s ON s.id = ps.server_id
		WHERE 1 = 1`
	var args []any
	if playerID != "" {
		query += " AND ps.player_id = ?"
		args = append(args, playerID)
	}
	if serverID != "" {
		query += " AND ps.server_id = ?"
		args = append(args, serverID)
	}
	query += " ORDER BY ps.joined_at DESC LIMIT ?"
	args = append(args, limit)

	var sessions []*PlayerSessionRecord
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// CreatePlayerSession inserts a session row at connect time.
func (s *Store) CreatePlayerSession(ctx context.Context, session *PlayerSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	return s.db.WithContext(ctx).Create(session).Error
}

// ClosePlayerSession finalizes a session row: sets left_at, duration and byte
// counters, and refreshes the player's last_seen.
func (s *Store) ClosePlayerSession(ctx context.Context, sessionID string, leftAt time.Time, durationSecs, bytesIn, bytesOut int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session PlayerSession
		if err := tx.First(&session, "id = ?", sessionID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil // Nothing to close (e.g. row lost to a DB reset)
			}
			return err
		}

		updates := map[string]any{
			"left_at":       leftAt,
			"duration_secs": durationSecs,
			"bytes_in":      bytesIn,
			"bytes_out":     bytesOut,
		}
		if err := tx.Model(&PlayerSession{}).Where("id = ?", sessionID).Updates(updates).Error; err != nil {
			return err
		}

		return tx.Model(&Player{}).Where("id = ?", session.PlayerID).
			Update("last_seen", leftAt).Error
	})
}

// CloseStalePlayerSessions closes any sessions still open (left_at NULL),
// typically left behind when the panel restarted mid-session. Duration is
// computed as closedAt - joined_at. Returns the number of closed sessions.
func (s *Store) CloseStalePlayerSessions(ctx context.Context, closedAt time.Time) (int64, error) {
	var stale []*PlayerSession
	if err := s.db.WithContext(ctx).Where("left_at IS NULL").Find(&stale).Error; err != nil {
		return 0, err
	}

	var closed int64
	for _, session := range stale {
		duration := int64(closedAt.Sub(session.JoinedAt).Seconds())
		if duration < 0 {
			duration = 0
		}
		updates := map[string]any{
			"left_at":       closedAt,
			"duration_secs": duration,
		}
		if err := s.db.WithContext(ctx).Model(&PlayerSession{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}

// TrafficTotals holds aggregated proxy traffic for one server and window.
type TrafficTotals struct {
	BytesIn  int64
	BytesOut int64
	Sessions int64
}

// GetTrafficSummary aggregates recorded player-session traffic for a server
// since the given time.
func (s *Store) GetTrafficSummary(ctx context.Context, serverID string, since time.Time) (*TrafficTotals, error) {
	var totals TrafficTotals
	err := s.db.WithContext(ctx).Model(&PlayerSession{}).
		Select("COALESCE(SUM(bytes_in), 0) AS bytes_in, COALESCE(SUM(bytes_out), 0) AS bytes_out, COUNT(*) AS sessions").
		Where("server_id = ? AND joined_at >= ?", serverID, since).
		Scan(&totals).Error
	return &totals, err
}
