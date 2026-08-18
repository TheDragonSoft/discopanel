package services

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/tunnel"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ discopanelv1connect.TunnelServiceHandler = (*TunnelService)(nil)

type TunnelService struct {
	store   *storage.Store
	manager *tunnel.Manager
	log     *logger.Logger
}

func NewTunnelService(store *storage.Store, manager *tunnel.Manager, log *logger.Logger) *TunnelService {
	return &TunnelService{
		store:   store,
		manager: manager,
		log:     log,
	}
}

func (s *TunnelService) GetServerTunnels(ctx context.Context, req *connect.Request[v1.GetServerTunnelsRequest]) (*connect.Response[v1.GetServerTunnelsResponse], error) {
	if req.Msg.ServerId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server_id is required"))
	}

	tunnels, err := s.manager.GetServerTunnels(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch server tunnels: %w", err))
	}

	hasGlobalAccount, _, _ := s.manager.GetPlayitAccountConfig(ctx)

	protoTunnels := make([]*v1.Tunnel, 0, len(tunnels))
	for _, t := range tunnels {
		protoTunnels = append(protoTunnels, s.tunnelToProto(t))
	}

	return connect.NewResponse(&v1.GetServerTunnelsResponse{
		Tunnels:          protoTunnels,
		HasGlobalAccount: hasGlobalAccount,
	}), nil
}

func (s *TunnelService) ListTunnels(ctx context.Context, req *connect.Request[v1.ListTunnelsRequest]) (*connect.Response[v1.ListTunnelsResponse], error) {
	tunnels, err := s.manager.ListTunnels(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list tunnels: %w", err))
	}

	hasGlobalAccount, _, _ := s.manager.GetPlayitAccountConfig(ctx)

	protoTunnels := make([]*v1.Tunnel, 0, len(tunnels))
	for _, t := range tunnels {
		protoTunnels = append(protoTunnels, s.tunnelToProto(t))
	}

	return connect.NewResponse(&v1.ListTunnelsResponse{
		Tunnels:          protoTunnels,
		HasGlobalAccount: hasGlobalAccount,
	}), nil
}

func (s *TunnelService) GetTunnel(ctx context.Context, req *connect.Request[v1.GetTunnelRequest]) (*connect.Response[v1.GetTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	tunnel, err := s.manager.GetTunnel(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tunnel not found: %w", err))
	}

	return connect.NewResponse(&v1.GetTunnelResponse{
		Tunnel: s.tunnelToProto(tunnel),
	}), nil
}

func (s *TunnelService) CreateTunnel(ctx context.Context, req *connect.Request[v1.CreateTunnelRequest]) (*connect.Response[v1.CreateTunnelResponse], error) {
	if req.Msg.ServerId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server_id is required"))
	}

	targetPort := int(req.Msg.TargetPort)
	if targetPort <= 0 {
		targetPort = 25565
	}

	autoStart := true
	if req.Msg.AutoStart != nil {
		autoStart = *req.Msg.AutoStart
	}

	followLifecycle := true
	if req.Msg.FollowServerLifecycle != nil {
		followLifecycle = *req.Msg.FollowServerLifecycle
	}

	targetHost := ""
	if req.Msg.TargetHost != nil {
		targetHost = *req.Msg.TargetHost
	}

	created, err := s.manager.CreateTunnel(
		ctx,
		req.Msg.ServerId,
		req.Msg.Name,
		req.Msg.Provider,
		req.Msg.Protocol,
		targetPort,
		targetHost,
		autoStart,
		followLifecycle,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create tunnel: %w", err))
	}

	return connect.NewResponse(&v1.CreateTunnelResponse{
		Tunnel: s.tunnelToProto(created),
	}), nil
}

func (s *TunnelService) UpdateTunnel(ctx context.Context, req *connect.Request[v1.UpdateTunnelRequest]) (*connect.Response[v1.UpdateTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	t, err := s.manager.GetTunnel(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tunnel not found: %w", err))
	}

	if req.Msg.Name != nil && *req.Msg.Name != "" {
		t.Name = *req.Msg.Name
	}
	if req.Msg.AutoStart != nil {
		t.AutoStart = *req.Msg.AutoStart
	}
	if req.Msg.FollowServerLifecycle != nil {
		t.FollowServerLifecycle = *req.Msg.FollowServerLifecycle
	}

	if err := s.store.UpdateTunnel(ctx, t); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tunnel: %w", err))
	}

	return connect.NewResponse(&v1.UpdateTunnelResponse{
		Tunnel: s.tunnelToProto(t),
	}), nil
}

func (s *TunnelService) DeleteTunnel(ctx context.Context, req *connect.Request[v1.DeleteTunnelRequest]) (*connect.Response[v1.DeleteTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	if err := s.manager.DeleteTunnel(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete tunnel: %w", err))
	}

	return connect.NewResponse(&v1.DeleteTunnelResponse{
		Status: "deleted",
	}), nil
}

func (s *TunnelService) StartTunnel(ctx context.Context, req *connect.Request[v1.StartTunnelRequest]) (*connect.Response[v1.StartTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	t, err := s.manager.StartTunnel(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start tunnel: %w", err))
	}

	return connect.NewResponse(&v1.StartTunnelResponse{
		Tunnel: s.tunnelToProto(t),
	}), nil
}

func (s *TunnelService) StopTunnel(ctx context.Context, req *connect.Request[v1.StopTunnelRequest]) (*connect.Response[v1.StopTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	t, err := s.manager.StopTunnel(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to stop tunnel: %w", err))
	}

	return connect.NewResponse(&v1.StopTunnelResponse{
		Tunnel: s.tunnelToProto(t),
	}), nil
}

func (s *TunnelService) RestartTunnel(ctx context.Context, req *connect.Request[v1.RestartTunnelRequest]) (*connect.Response[v1.RestartTunnelResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	t, err := s.manager.RestartTunnel(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restart tunnel: %w", err))
	}

	return connect.NewResponse(&v1.RestartTunnelResponse{
		Tunnel: s.tunnelToProto(t),
	}), nil
}

func (s *TunnelService) GetTunnelLogs(ctx context.Context, req *connect.Request[v1.GetTunnelLogsRequest]) (*connect.Response[v1.GetTunnelLogsResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	logs, err := s.manager.GetTunnelLogs(ctx, req.Msg.Id, int(req.Msg.Tail))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get tunnel logs: %w", err))
	}

	return connect.NewResponse(&v1.GetTunnelLogsResponse{
		Logs: logs,
	}), nil
}

func (s *TunnelService) GetPlayitAccountConfig(ctx context.Context, req *connect.Request[v1.GetPlayitAccountConfigRequest]) (*connect.Response[v1.GetPlayitAccountConfigResponse], error) {
	isLinked, notice, err := s.manager.GetPlayitAccountConfig(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get account config: %w", err))
	}

	return connect.NewResponse(&v1.GetPlayitAccountConfigResponse{
		IsLinked: isLinked,
		Notice:   notice,
	}), nil
}

func (s *TunnelService) StartAccountLinkSession(ctx context.Context, req *connect.Request[v1.StartAccountLinkSessionRequest]) (*connect.Response[v1.StartAccountLinkSessionResponse], error) {
	session, err := s.manager.StartAccountLinkSession(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start account link session: %w", err))
	}

	return connect.NewResponse(&v1.StartAccountLinkSessionResponse{
		SessionId: session.SessionID,
		ClaimUrl:  session.ClaimURL,
		ClaimCode: session.ClaimCode,
		ExpiresAt: timestamppb.New(session.ExpiresAt),
	}), nil
}

func (s *TunnelService) CheckAccountLinkStatus(ctx context.Context, req *connect.Request[v1.CheckAccountLinkStatusRequest]) (*connect.Response[v1.CheckAccountLinkStatusResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	isLinked, statusMsg, err := s.manager.CheckAccountLinkStatus(ctx, req.Msg.SessionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check account link status: %w", err))
	}

	return connect.NewResponse(&v1.CheckAccountLinkStatusResponse{
		IsLinked: isLinked,
		Status:   statusMsg,
	}), nil
}

func (s *TunnelService) SetPlayitAccountSecret(ctx context.Context, req *connect.Request[v1.SetPlayitAccountSecretRequest]) (*connect.Response[v1.SetPlayitAccountSecretResponse], error) {
	if err := s.manager.SetPlayitAccountSecret(ctx, req.Msg.SecretKey); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to set account secret: %w", err))
	}

	return connect.NewResponse(&v1.SetPlayitAccountSecretResponse{
		Success: true,
	}), nil
}

func (s *TunnelService) UnlinkPlayitAccount(ctx context.Context, req *connect.Request[v1.UnlinkPlayitAccountRequest]) (*connect.Response[v1.UnlinkPlayitAccountResponse], error) {
	if err := s.manager.UnlinkPlayitAccount(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unlink playit account: %w", err))
	}

	return connect.NewResponse(&v1.UnlinkPlayitAccountResponse{
		Success: true,
	}), nil
}

func (s *TunnelService) tunnelToProto(t *storage.Tunnel) *v1.Tunnel {
	if t == nil {
		return nil
	}

	serverName := ""
	if t.Server != nil {
		serverName = t.Server.Name
	}

	protoStatus := v1.TunnelStatus_TUNNEL_STATUS_UNSPECIFIED
	switch t.Status {
	case storage.TunnelStatusStopped:
		protoStatus = v1.TunnelStatus_TUNNEL_STATUS_STOPPED
	case storage.TunnelStatusStarting:
		protoStatus = v1.TunnelStatus_TUNNEL_STATUS_STARTING
	case storage.TunnelStatusClaimPending:
		protoStatus = v1.TunnelStatus_TUNNEL_STATUS_CLAIM_PENDING
	case storage.TunnelStatusRunning:
		protoStatus = v1.TunnelStatus_TUNNEL_STATUS_RUNNING
	case storage.TunnelStatusError:
		protoStatus = v1.TunnelStatus_TUNNEL_STATUS_ERROR
	}

	protoProtocol := v1.TunnelProtocol_TUNNEL_PROTOCOL_TCP
	if t.Protocol == "udp" {
		protoProtocol = v1.TunnelProtocol_TUNNEL_PROTOCOL_UDP
	} else if t.Protocol == "both" {
		protoProtocol = v1.TunnelProtocol_TUNNEL_PROTOCOL_BOTH
	}

	return &v1.Tunnel{
		Id:                    t.ID,
		ServerId:              t.ServerID,
		ServerName:            serverName,
		Provider:              v1.TunnelProvider_TUNNEL_PROVIDER_PLAYIT,
		Name:                  t.Name,
		Protocol:              protoProtocol,
		TargetHost:            t.TargetHost,
		TargetPort:            int32(t.TargetPort),
		ContainerId:           t.ContainerID,
		Status:                protoStatus,
		ClaimUrl:              t.ClaimURL,
		ClaimCode:             t.ClaimCode,
		IsAccountLinked:       t.IsAccountLinked,
		PublicAddress:         t.PublicAddress,
		PublicPort:            int32(t.PublicPort),
		AutoStart:             t.AutoStart,
		FollowServerLifecycle: t.FollowServerLifecycle,
		CreatedAt:             timestamppb.New(t.CreatedAt),
		UpdatedAt:             timestamppb.New(t.UpdatedAt),
	}
}
