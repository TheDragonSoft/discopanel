package services

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/tracker"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that PlayerService implements the interface
var _ discopanelv1connect.PlayerServiceHandler = (*PlayerService)(nil)

// PlayerService implements the Player service
type PlayerService struct {
	store   *storage.Store
	tracker *tracker.Tracker
	log     *logger.Logger
}

// NewPlayerService creates a new player service
func NewPlayerService(store *storage.Store, playerTracker *tracker.Tracker, log *logger.Logger) *PlayerService {
	return &PlayerService{
		store:   store,
		tracker: playerTracker,
		log:     log,
	}
}

// playerToProto converts an aggregated player row to proto using pre-fetched
// per-server stats, marking players that currently have a tracked connection
// as online.
func (s *PlayerService) playerToProto(agg *storage.PlayerAggregate, stats []*storage.PlayerServerStat, serverNames map[string]string) *v1.Player {
	player := &v1.Player{
		Id:                agg.ID,
		Name:              agg.Name,
		FirstSeen:         timestamppb.New(agg.FirstSeen),
		LastSeen:          timestamppb.New(agg.LastSeen),
		TotalPlaytimeSecs: agg.TotalPlaytimeSecs,
		SessionsCount:     int32(agg.SessionsCount),
	}

	if s.tracker != nil {
		player.Online = s.tracker.IsOnline(agg.Name)
	}

	for _, stat := range stats {
		player.ServerStats = append(player.ServerStats, &v1.PlayerServerStats{
			ServerId:      stat.ServerID,
			ServerName:    serverNames[stat.ServerID],
			PlaytimeSecs:  stat.PlaytimeSecs,
			SessionsCount: int32(stat.SessionsCount),
			LastSeen:      timestamppb.New(stat.LastSeen),
		})
	}
	return player
}

// buildPlayers converts aggregated player rows to proto, fetching per-server
// stats for all players in one query and server names in another.
func (s *PlayerService) buildPlayers(ctx context.Context, aggs []*storage.PlayerAggregate) ([]*v1.Player, error) {
	playerIDs := make([]string, 0, len(aggs))
	for _, agg := range aggs {
		playerIDs = append(playerIDs, agg.ID)
	}

	stats, err := s.store.ListPlayerServerStats(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	statsByPlayer := make(map[string][]*storage.PlayerServerStat, len(aggs))
	for _, stat := range stats {
		statsByPlayer[stat.PlayerID] = append(statsByPlayer[stat.PlayerID], stat)
	}

	serverNames, err := s.serverNameMap(ctx)
	if err != nil {
		return nil, err
	}

	players := make([]*v1.Player, 0, len(aggs))
	for _, agg := range aggs {
		players = append(players, s.playerToProto(agg, statsByPlayer[agg.ID], serverNames))
	}
	return players, nil
}

// serverNameMap returns a server ID -> name lookup table.
func (s *PlayerService) serverNameMap(ctx context.Context) (map[string]string, error) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(servers))
	for _, server := range servers {
		names[server.ID] = server.Name
	}
	return names, nil
}

// dbSessionRecordToProto converts a joined session row to proto.
func dbSessionRecordToProto(record *storage.PlayerSessionRecord) *v1.PlayerSession {
	session := &v1.PlayerSession{
		Id:           record.ID,
		PlayerId:     record.PlayerID,
		PlayerName:   record.PlayerName,
		ServerId:     record.ServerID,
		ServerName:   record.ServerName,
		JoinedAt:     timestamppb.New(record.JoinedAt),
		DurationSecs: record.DurationSecs,
		BytesIn:      record.BytesIn,
		BytesOut:     record.BytesOut,
	}
	if record.LeftAt != nil {
		session.LeftAt = timestamppb.New(*record.LeftAt)
	}
	return session
}

// ListPlayers lists known players with aggregated stats
func (s *PlayerService) ListPlayers(ctx context.Context, req *connect.Request[v1.ListPlayersRequest]) (*connect.Response[v1.ListPlayersResponse], error) {
	msg := req.Msg

	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 100
	}

	players, total, err := s.store.ListPlayersWithStats(ctx, msg.Search, msg.ServerId, limit, int(msg.Offset))
	if err != nil {
		s.log.Error("Failed to list players: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list players"))
	}

	protoPlayers, err := s.buildPlayers(ctx, players)
	if err != nil {
		s.log.Error("Failed to build player stats: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list players"))
	}

	return connect.NewResponse(&v1.ListPlayersResponse{
		Players: protoPlayers,
		Total:   int32(total),
	}), nil
}

// GetPlayer gets a single player and their recent sessions
func (s *PlayerService) GetPlayer(ctx context.Context, req *connect.Request[v1.GetPlayerRequest]) (*connect.Response[v1.GetPlayerResponse], error) {
	playerRow, err := s.store.GetPlayerByID(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("player not found"))
	}

	sessions, err := s.store.ListPlayerSessions(ctx, playerRow.ID, "", 20)
	if err != nil {
		s.log.Error("Failed to list sessions for player %s: %v", playerRow.ID, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get player"))
	}

	protoSessions := make([]*v1.PlayerSession, 0, len(sessions))
	for _, record := range sessions {
		protoSessions = append(protoSessions, dbSessionRecordToProto(record))
	}

	agg := &storage.PlayerAggregate{
		ID:        playerRow.ID,
		Name:      playerRow.Name,
		FirstSeen: playerRow.FirstSeen,
		LastSeen:  playerRow.LastSeen,
	}
	protoPlayers, err := s.buildPlayers(ctx, []*storage.PlayerAggregate{agg})
	if err != nil {
		s.log.Error("Failed to build player stats for %s: %v", playerRow.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get player"))
	}

	return connect.NewResponse(&v1.GetPlayerResponse{
		Player:   protoPlayers[0],
		Sessions: protoSessions,
	}), nil
}

// ListPlayerSessions lists player sessions with optional filters
func (s *PlayerService) ListPlayerSessions(ctx context.Context, req *connect.Request[v1.ListPlayerSessionsRequest]) (*connect.Response[v1.ListPlayerSessionsResponse], error) {
	msg := req.Msg

	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 100
	}

	sessions, err := s.store.ListPlayerSessions(ctx, msg.PlayerId, msg.ServerId, limit)
	if err != nil {
		s.log.Error("Failed to list player sessions: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list player sessions"))
	}

	protoSessions := make([]*v1.PlayerSession, 0, len(sessions))
	for _, record := range sessions {
		protoSessions = append(protoSessions, dbSessionRecordToProto(record))
	}

	return connect.NewResponse(&v1.ListPlayerSessionsResponse{
		Sessions: protoSessions,
	}), nil
}

// ListOnlinePlayers lists players currently connected through the proxy
func (s *PlayerService) ListOnlinePlayers(ctx context.Context, req *connect.Request[v1.ListOnlinePlayersRequest]) (*connect.Response[v1.ListOnlinePlayersResponse], error) {
	if s.tracker == nil {
		return connect.NewResponse(&v1.ListOnlinePlayersResponse{}), nil
	}

	online := s.tracker.ListOnline(req.Msg.ServerId)

	serverNames, err := s.serverNameMap(ctx)
	if err != nil {
		s.log.Error("Failed to list servers for online players: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list online players"))
	}

	players := make([]*v1.OnlinePlayer, 0, len(online))
	for _, session := range online {
		players = append(players, &v1.OnlinePlayer{
			PlayerId:   session.PlayerID,
			Name:       session.Name,
			ServerId:   session.ServerID,
			ServerName: serverNames[session.ServerID],
			JoinedAt:   timestamppb.New(session.JoinedAt),
		})
	}

	return connect.NewResponse(&v1.ListOnlinePlayersResponse{
		Players: players,
	}), nil
}
