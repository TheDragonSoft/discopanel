package tunnel

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nickheyer/discopanel/internal/config"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/internal/events"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

type AccountLinkSession struct {
	SessionID   string    `json:"session_id"`
	ClaimURL    string    `json:"claim_url"`
	ClaimCode   string    `json:"claim_code"`
	ContainerID string    `json:"container_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsLinked    bool      `json:"is_linked"`
}

type Manager struct {
	store         *storage.Store
	docker        *docker.Client
	driver        *PlayitDriver
	apiClient     *PlayitAPIClient
	bus           *events.Bus
	config        *config.Config
	logger        *logger.Logger
	logStreamer   *logger.LogStreamer
	claimSessions map[string]*AccountLinkSession
	mu            sync.RWMutex
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewManager(store *storage.Store, dockerClient *docker.Client, bus *events.Bus, cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{
		store:         store,
		docker:        dockerClient,
		driver:        NewPlayitDriver(dockerClient, cfg, log),
		apiClient:     NewPlayitAPIClient(),
		bus:           bus,
		config:        cfg,
		logger:        log,
		claimSessions: make(map[string]*AccountLinkSession),
	}
}

func (m *Manager) SetLogStreamer(streamer *logger.LogStreamer) {
	m.logStreamer = streamer
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running = true

	// Subscribe to server lifecycle events if event bus is available
	if m.bus != nil {
		m.bus.Subscribe(m.handleServerEvent)
	}

	// Auto-start enabled tunnels
	go m.autoStartTunnels()

	// Launch background monitor/sniffer loop
	go m.runMonitorLoop(m.ctx)

	m.logger.Info("Tunnel manager started")
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	m.running = false
	m.logger.Info("Tunnel manager stopped")
	return nil
}

func (m *Manager) autoStartTunnels() {
	ctx := context.Background()
	tunnels, err := m.store.ListAutoStartTunnels(ctx)
	if err != nil {
		m.logger.Error("Failed to list auto-start tunnels: %v", err)
		return
	}

	for _, t := range tunnels {
		if t.Status != storage.TunnelStatusStopped {
			continue
		}
		// If server is running or not detached, check server status
		if t.FollowServerLifecycle {
			server, err := m.store.GetServer(ctx, t.ServerID)
			if err != nil || server.Status != storage.StatusRunning {
				continue
			}
		}

		m.logger.Info("Auto-starting tunnel: %s (%s)", t.Name, t.ID)
		if _, err := m.StartTunnel(ctx, t.ID); err != nil {
			m.logger.Error("Failed to auto-start tunnel %s: %v", t.ID, err)
		}
	}
}

func (m *Manager) handleServerEvent(ctx context.Context, ev events.Event) {
	if ev.ServerID == "" {
		return
	}

	switch ev.Type {
	case v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_SERVER_START:
		tunnels, err := m.store.ListTunnelsFollowingServerLifecycle(ctx, ev.ServerID)
		if err != nil {
			return
		}
		for _, t := range tunnels {
			if t.Status == storage.TunnelStatusStopped {
				m.logger.Info("Server %s started, starting tunnel %s", ev.ServerID, t.Name)
				go func(id string) {
					_, _ = m.StartTunnel(context.Background(), id)
				}(t.ID)
			}
		}

	case v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_SERVER_STOP:
		tunnels, err := m.store.ListTunnelsFollowingServerLifecycle(ctx, ev.ServerID)
		if err != nil {
			return
		}
		for _, t := range tunnels {
			if t.Status == storage.TunnelStatusRunning || t.Status == storage.TunnelStatusStarting || t.Status == storage.TunnelStatusClaimPending {
				m.logger.Info("Server %s stopped, stopping tunnel %s", ev.ServerID, t.Name)
				go func(id string) {
					_, _ = m.StopTunnel(context.Background(), id)
				}(t.ID)
			}
		}
	}
}

func (m *Manager) CreateTunnel(ctx context.Context, serverID, name string, provider v1.TunnelProvider, protocol v1.TunnelProtocol, targetPort int, targetHost string, autoStart, followLifecycle bool) (*storage.Tunnel, error) {
	server, err := m.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("server %s not found: %w", serverID, err)
	}

	if targetPort <= 0 {
		targetPort = 25565
	}
	if targetHost == "" {
		targetHost = fmt.Sprintf("discopanel-server-%s", server.ID)
	}
	if name == "" {
		name = fmt.Sprintf("%s Tunnel (%d)", server.Name, targetPort)
	}

	protoStr := "tcp"
	if protocol == v1.TunnelProtocol_TUNNEL_PROTOCOL_UDP {
		protoStr = "udp"
	} else if protocol == v1.TunnelProtocol_TUNNEL_PROTOCOL_BOTH {
		protoStr = "both"
	}

	// Check if global account secret key exists
	accountSecret, _ := m.store.GetSystemSetting(ctx, PlayitAccountSecretKey)
	isAccountLinked := accountSecret != ""

	tunnel := &storage.Tunnel{
		ID:                    uuid.New().String(),
		ServerID:              serverID,
		Provider:              "playit",
		Name:                  name,
		Protocol:              protoStr,
		TargetHost:            targetHost,
		TargetPort:            targetPort,
		Status:                storage.TunnelStatusStopped,
		IsAccountLinked:       isAccountLinked,
		AutoStart:             autoStart,
		FollowServerLifecycle: followLifecycle,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}

	// Auto-provision tunnel on Playit.gg API if account is linked
	if isAccountLinked && accountSecret != "" {
		tunnelType := ""
		if targetPort == 25565 {
			tunnelType = "minecraft-java"
		} else if targetPort == 19132 {
			tunnelType = "minecraft-bedrock"
		}

		apiDetails, apiErr := m.apiClient.CreateTunnel(ctx, accountSecret, name, tunnelType, protoStr, targetPort)
		if apiErr == nil && apiDetails != nil {
			if apiDetails.PublicAddress != "" {
				tunnel.PublicAddress = apiDetails.PublicAddress
				tunnel.PublicPort = apiDetails.PublicPort
			}
			m.logger.Info("Successfully auto-provisioned Playit tunnel via API: %s (%s:%d)", name, tunnel.PublicAddress, tunnel.PublicPort)
		} else if apiErr != nil {
			m.logger.Warn("Could not auto-provision Playit tunnel via API: %v (agent will sync upon connection)", apiErr)
		}
	}

	if err := m.store.CreateTunnel(ctx, tunnel); err != nil {
		return nil, fmt.Errorf("failed to save tunnel in database: %w", err)
	}

	// Automatically start the tunnel
	startedTunnel, err := m.StartTunnel(ctx, tunnel.ID)
	if err != nil {
		m.logger.Warn("Created tunnel %s but failed to start container immediately: %v", tunnel.ID, err)
		return tunnel, nil
	}

	return startedTunnel, nil
}

func (m *Manager) StartTunnel(ctx context.Context, tunnelID string) (*storage.Tunnel, error) {
	tunnel, err := m.store.GetTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}

	server, err := m.store.GetServer(ctx, tunnel.ServerID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}

	// Fetch global account secret key
	accountSecret, _ := m.store.GetSystemSetting(ctx, PlayitAccountSecretKey)
	if accountSecret != "" {
		tunnel.IsAccountLinked = true
	}

	secretToUse := tunnel.SecretKey
	if secretToUse == "" {
		secretToUse = accountSecret
	}

	if secretToUse == "" {
		claimSession, claimErr := m.StartAccountLinkSession(ctx)
		if claimErr == nil && claimSession != nil {
			tunnel.Status = storage.TunnelStatusClaimPending
			tunnel.ClaimURL = claimSession.ClaimURL
			tunnel.ClaimCode = claimSession.ClaimCode
			_ = m.store.UpdateTunnel(ctx, tunnel)
			m.logger.Info("Initiated Playit claim session for tunnel %s: %s", tunnel.Name, tunnel.ClaimURL)
			return tunnel, nil
		}
		return nil, fmt.Errorf("Playit secret key is required. Please link your account in Settings -> Routing")
	}

	// Auto-provision on Playit.gg API if account is linked and public address not yet known
	if secretToUse != "" && tunnel.PublicAddress == "" {
		tunnelType := ""
		if tunnel.TargetPort == 25565 {
			tunnelType = "minecraft-java"
		} else if tunnel.TargetPort == 19132 {
			tunnelType = "minecraft-bedrock"
		}
		apiDetails, apiErr := m.apiClient.CreateTunnel(ctx, secretToUse, tunnel.Name, tunnelType, tunnel.Protocol, tunnel.TargetPort)
		if apiErr == nil && apiDetails != nil && apiDetails.PublicAddress != "" {
			tunnel.PublicAddress = apiDetails.PublicAddress
			tunnel.PublicPort = apiDetails.PublicPort
			_ = m.store.UpdateTunnel(ctx, tunnel)
			m.logger.Info("Provisioned Playit tunnel on start: %s -> %s:%d", tunnel.Name, tunnel.PublicAddress, tunnel.PublicPort)
		}
	}

	// Remove old container if it exists
	if tunnel.ContainerID != "" {
		_ = m.docker.RemoveContainer(ctx, tunnel.ContainerID)
		tunnel.ContainerID = ""
	}

	// Create fresh container
	containerID, err := m.driver.CreateContainer(ctx, tunnel, server, accountSecret)
	if err != nil {
		tunnel.Status = storage.TunnelStatusError
		_ = m.store.UpdateTunnel(ctx, tunnel)
		return nil, fmt.Errorf("failed to create tunnel container: %w", err)
	}

	tunnel.ContainerID = containerID
	tunnel.Status = storage.TunnelStatusStarting

	// Start container
	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		tunnel.Status = storage.TunnelStatusError
		_ = m.store.UpdateTunnel(ctx, tunnel)
		return nil, fmt.Errorf("failed to start tunnel container: %w", err)
	}

	if err := m.store.UpdateTunnel(ctx, tunnel); err != nil {
		m.logger.Error("Failed to update tunnel status: %v", err)
	}

	return tunnel, nil
}

func (m *Manager) StopTunnel(ctx context.Context, tunnelID string) (*storage.Tunnel, error) {
	tunnel, err := m.store.GetTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}

	if tunnel.ContainerID != "" {
		_, _ = m.docker.StopContainer(ctx, tunnel.ContainerID)
	}

	tunnel.Status = storage.TunnelStatusStopped
	if err := m.store.UpdateTunnel(ctx, tunnel); err != nil {
		return nil, err
	}

	return tunnel, nil
}

func (m *Manager) RestartTunnel(ctx context.Context, tunnelID string) (*storage.Tunnel, error) {
	_, err := m.StopTunnel(ctx, tunnelID)
	if err != nil {
		m.logger.Warn("Error stopping tunnel %s during restart: %v", tunnelID, err)
	}
	return m.StartTunnel(ctx, tunnelID)
}

func (m *Manager) DeleteTunnel(ctx context.Context, tunnelID string) error {
	tunnel, err := m.store.GetTunnel(ctx, tunnelID)
	if err != nil {
		return err
	}

	if tunnel.ContainerID != "" {
		_, _ = m.docker.StopContainer(ctx, tunnel.ContainerID)
		_ = m.docker.RemoveContainer(ctx, tunnel.ContainerID)
	}

	return m.store.DeleteTunnel(ctx, tunnelID)
}

func (m *Manager) GetTunnelLogs(ctx context.Context, tunnelID string, tail int) ([]string, error) {
	tunnel, err := m.store.GetTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}

	if tunnel.ContainerID == "" {
		return []string{}, nil
	}

	if tail <= 0 {
		tail = 100
	}

	rawLogs, err := m.docker.GetContainerLogs(ctx, tunnel.ContainerID, tail)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(StripANSI(rawLogs), "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result, nil
}

func (m *Manager) GetServerTunnels(ctx context.Context, serverID string) ([]*storage.Tunnel, error) {
	return m.SyncPlayitTunnels(ctx, serverID)
}

func (m *Manager) ListTunnels(ctx context.Context) ([]*storage.Tunnel, error) {
	return m.SyncPlayitTunnels(ctx, "")
}

// SyncPlayitTunnels queries the Playit.gg API for all tunnels on the linked account,
// maps each tunnel to the appropriate server and target port (e.g. 25565, 19132, etc.),
// and automatically registers/connects them in DiscoPanel.
func (m *Manager) SyncPlayitTunnels(ctx context.Context, serverID string) ([]*storage.Tunnel, error) {
	secret, err := m.store.GetSystemSetting(ctx, PlayitAccountSecretKey)
	if err != nil || secret == "" {
		if serverID != "" {
			return m.store.GetServerTunnels(ctx, serverID)
		}
		return m.store.ListTunnels(ctx)
	}

	remoteTunnels, err := m.apiClient.ListTunnels(ctx, secret)
	if err != nil {
		m.logger.Warn("Failed to list Playit tunnels from API during sync: %v", err)
		if serverID != "" {
			return m.store.GetServerTunnels(ctx, serverID)
		}
		return m.store.ListTunnels(ctx)
	}

	// Fetch target server or all servers
	var servers []*storage.Server
	if serverID != "" {
		srv, err := m.store.GetServer(ctx, serverID)
		if err == nil && srv != nil {
			servers = []*storage.Server{srv}
		}
	} else {
		servers, _ = m.store.ListServers(ctx)
	}

	existingTunnels, _ := m.store.ListTunnels(ctx)
	existingMap := make(map[string]*storage.Tunnel) // key: serverID:port
	for _, t := range existingTunnels {
		key := fmt.Sprintf("%s:%d", t.ServerID, t.TargetPort)
		existingMap[key] = t
	}

	for _, rt := range remoteTunnels {
		port := rt.LocalPort
		if port <= 0 {
			if rt.TunnelType == "minecraft-java" {
				port = 25565
			} else if rt.TunnelType == "minecraft-bedrock" {
				port = 19132
			} else {
				port = 25565
			}
		}

		// Find which server matches this port
		for _, srv := range servers {
			isMatch := (srv.Port == port) || (srv.Port == 0 && port == 25565) || (len(servers) == 1)
			if !isMatch {
				for _, ap := range srv.AdditionalPorts {
					if ap != nil && (int(ap.HostPort) == port || int(ap.ContainerPort) == port) {
						isMatch = true
						break
					}
				}
			}

			if isMatch {
				key := fmt.Sprintf("%s:%d", srv.ID, port)
				existing, ok := existingMap[key]
				if !ok {
					// Fallback: check if server already has any tunnel without public address
					for _, ext := range existingTunnels {
						if ext.ServerID == srv.ID && (ext.PublicAddress == "" || ext.TargetPort == port) {
							existing = ext
							ok = true
							break
						}
					}
				}

				if ok && existing != nil {
					changed := false
					if rt.PublicAddress != "" && existing.PublicAddress != rt.PublicAddress {
						existing.PublicAddress = rt.PublicAddress
						changed = true
					}
					if rt.PublicPort > 0 && existing.PublicPort != rt.PublicPort {
						existing.PublicPort = rt.PublicPort
						changed = true
					}
					if !existing.IsAccountLinked {
						existing.IsAccountLinked = true
						changed = true
					}
					if changed {
						_ = m.store.UpdateTunnel(ctx, existing)
					}
					if existing.Status == storage.TunnelStatusStopped && existing.AutoStart && srv.Status == storage.StatusRunning {
						go func(id string) {
							_, _ = m.StartTunnel(context.Background(), id)
						}(existing.ID)
					}
				} else {
					tunnelName := rt.Name
					if tunnelName == "" {
						tunnelName = fmt.Sprintf("%s WAN Tunnel (%d)", srv.Name, port)
					}
					proto := "both"
					if rt.PortType != "" {
						proto = rt.PortType
					}
					newTunnel := &storage.Tunnel{
						ID:                    uuid.New().String(),
						ServerID:              srv.ID,
						Provider:              "playit",
						Name:                  tunnelName,
						Protocol:              proto,
						TargetHost:            fmt.Sprintf("discopanel-server-%s", srv.ID),
						TargetPort:            port,
						Status:                storage.TunnelStatusStopped,
						IsAccountLinked:       true,
						PublicAddress:         rt.PublicAddress,
						PublicPort:            rt.PublicPort,
						AutoStart:             true,
						FollowServerLifecycle: true,
						CreatedAt:             time.Now().UTC(),
						UpdatedAt:             time.Now().UTC(),
					}
					_ = m.store.CreateTunnel(ctx, newTunnel)
					existingMap[key] = newTunnel

					if srv.Status == storage.StatusRunning {
						go func(id string) {
							_, _ = m.StartTunnel(context.Background(), id)
						}(newTunnel.ID)
					}
				}
			}
		}
	}

	if serverID != "" {
		return m.store.GetServerTunnels(ctx, serverID)
	}
	return m.store.ListTunnels(ctx)
}

func (m *Manager) GetTunnel(ctx context.Context, id string) (*storage.Tunnel, error) {
	return m.store.GetTunnel(ctx, id)
}

func (m *Manager) GetPlayitAccountConfig(ctx context.Context) (bool, string, error) {
	secret, err := m.store.GetSystemSetting(ctx, PlayitAccountSecretKey)
	if err != nil {
		return false, "", err
	}
	if secret != "" {
		return true, "Playit.gg account is linked. All tunnels automatically inherit your account credentials.", nil
	}
	return false, "No Playit.gg account linked. Tunnels run in guest mode with claim links.", nil
}

func (m *Manager) SetPlayitAccountSecret(ctx context.Context, secretKey string) error {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return m.store.DeleteSystemSetting(ctx, PlayitAccountSecretKey)
	}
	if err := m.store.SetSystemSetting(ctx, PlayitAccountSecretKey, secretKey); err != nil {
		return err
	}

	// Trigger background sync and restart active tunnel containers to apply new credentials
	go func() {
		ctx := context.Background()
		tunnels, err := m.SyncPlayitTunnels(ctx, "")
		if err == nil {
			for _, t := range tunnels {
				if t.Status == storage.TunnelStatusRunning || t.Status == storage.TunnelStatusStarting {
					_, _ = m.StartTunnel(ctx, t.ID)
				}
			}
		}
	}()

	return nil
}

func (m *Manager) UnlinkPlayitAccount(ctx context.Context) error {
	return m.store.DeleteSystemSetting(ctx, PlayitAccountSecretKey)
}

func (m *Manager) StartAccountLinkSession(ctx context.Context) (*AccountLinkSession, error) {
	sessionID := uuid.New().String()
	code := sessionID[:8]

	if err := m.apiClient.SetupClaim(ctx, code); err != nil {
		return nil, fmt.Errorf("failed to register claim with Playit.gg: %w", err)
	}

	claimURL := fmt.Sprintf("https://playit.gg/claim/%s", code)
	session := &AccountLinkSession{
		SessionID: sessionID,
		ClaimURL:  claimURL,
		ClaimCode: code,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	m.mu.Lock()
	m.claimSessions[sessionID] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) CheckAccountLinkStatus(ctx context.Context, sessionID string) (bool, string, error) {
	m.mu.Lock()
	session, exists := m.claimSessions[sessionID]
	m.mu.Unlock()

	if !exists {
		return false, "session not found", fmt.Errorf("session not found or expired")
	}

	if session.IsLinked {
		return true, "linked", nil
	}

	if session.ClaimCode != "" {
		secretKey, err := m.apiClient.ExchangeClaim(ctx, session.ClaimCode)
		if err == nil && secretKey != "" {
			if err := m.SetPlayitAccountSecret(ctx, secretKey); err != nil {
				return false, "failed to save secret", err
			}
			m.mu.Lock()
			session.IsLinked = true
			m.mu.Unlock()
			return true, "Account successfully linked to Playit.gg!", nil
		}
	}

	return false, "waiting for user to claim in browser", nil
}

func (m *Manager) runMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollActiveTunnels(ctx)
		}
	}
}

func (m *Manager) pollActiveTunnels(ctx context.Context) {
	tunnels, err := m.store.ListTunnels(ctx)
	if err != nil {
		return
	}

	for _, t := range tunnels {
		// If tunnel is waiting for account claim, poll Playit API to check if user accepted in browser
		if t.Status == storage.TunnelStatusClaimPending && t.ClaimCode != "" {
			secretKey, err := m.apiClient.ExchangeClaim(ctx, t.ClaimCode)
			if err == nil && secretKey != "" {
				_ = m.SetPlayitAccountSecret(ctx, secretKey)
				t.IsAccountLinked = true
				t.Status = storage.TunnelStatusStarting
				_ = m.store.UpdateTunnel(ctx, t)
				m.logger.Info("Account successfully linked via claim for tunnel %s! Starting tunnel...", t.Name)
				go func(tID string) {
					_, _ = m.StartTunnel(context.Background(), tID)
				}(t.ID)
				continue
			}
		}

		if t.Status == storage.TunnelStatusStopped || t.ContainerID == "" {
			continue
		}

		// Inspect container status
		status, err := m.docker.GetContainerStatus(ctx, t.ContainerID)
		if err != nil || (status != storage.StatusRunning && status != storage.StatusStarting) {
			if t.Status != storage.TunnelStatusError && t.Status != storage.TunnelStatusStopped {
				t.Status = storage.TunnelStatusError
				_ = m.store.UpdateTunnel(ctx, t)
			}
			continue
		}

		// Read container logs to sniff claim link & public address
		rawLogs, err := m.docker.GetContainerLogs(ctx, t.ContainerID, 60)
		if err != nil {
			continue
		}

		lines := strings.Split(rawLogs, "\n")
		claimURL, claimCode, publicAddr, publicPort, isRunning := m.driver.SniffLogs(lines)

		changed := false
		if claimURL != "" && t.ClaimURL != claimURL {
			t.ClaimURL = claimURL
			t.ClaimCode = claimCode
			changed = true
		}

		if publicAddr != "" && t.PublicAddress != publicAddr {
			t.PublicAddress = publicAddr
			changed = true
		}
		if publicPort > 0 && t.PublicPort != publicPort {
			t.PublicPort = publicPort
			changed = true
		}

		// If public address is still empty and account is linked, poll Playit API list
		if t.PublicAddress == "" && t.IsAccountLinked {
			secretKey := t.SecretKey
			if secretKey == "" {
				secretKey, _ = m.store.GetSystemSetting(ctx, PlayitAccountSecretKey)
			}
			if secretKey != "" {
				apiTunnels, err := m.apiClient.ListTunnels(ctx, secretKey)
				if err == nil {
					for _, at := range apiTunnels {
						if at.LocalPort == t.TargetPort || at.Name == t.Name || len(apiTunnels) == 1 {
							if at.PublicAddress != "" {
								t.PublicAddress = at.PublicAddress
								t.PublicPort = at.PublicPort
								changed = true
								break
							}
						}
					}
				}
			}
		}

		// Update runtime status
		if publicAddr != "" || isRunning || t.PublicAddress != "" {
			if t.Status != storage.TunnelStatusRunning {
				t.Status = storage.TunnelStatusRunning
				changed = true
			}
		} else if claimURL != "" {
			if t.Status != storage.TunnelStatusClaimPending {
				t.Status = storage.TunnelStatusClaimPending
				changed = true
			}
		}

		if changed {
			_ = m.store.UpdateTunnel(ctx, t)
		}
	}
}
