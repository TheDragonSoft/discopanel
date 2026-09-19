package module

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nickheyer/discopanel/internal/alias"
	storage "github.com/nickheyer/discopanel/internal/db"
)

// PlayitTemplateID is the builtin module template that runs the playit.gg agent
const PlayitTemplateID = "builtin-playit"

// Module metadata keys populated by the playit watcher
const (
	MetaPublicAddress = "public_address"
	MetaPublicPort    = "public_port"
	MetaSyncedAt      = "playit_synced_at"
)

// playitClient talks to the playit.gg agent API. The base URL is configurable
// so integrations can be tested against a mock server.
type playitClient struct {
	httpClient *http.Client
	baseURL    string
}

func newPlayitClient(baseURL string) *playitClient {
	if baseURL == "" {
		baseURL = "https://api.playit.gg"
	}
	return &playitClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

type playitTunnel struct {
	ID            string
	Name          string
	TunnelType    string
	PortType      string
	PublicAddress string
	PublicPort    int
	LocalPort     int
}

// listTunnels lists all tunnels registered to the given playit agent secret key
func (c *playitClient) listTunnels(ctx context.Context, secretKey string) ([]*playitTunnel, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tunnels/list", strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("playit api error (HTTP %d)", resp.StatusCode)
	}

	var res struct {
		Data struct {
			Tunnels []map[string]any `json:"tunnels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var results []*playitTunnel
	for _, raw := range res.Data.Tunnels {
		item := &playitTunnel{}
		item.ID, _ = raw["id"].(string)
		item.Name, _ = raw["name"].(string)
		item.TunnelType, _ = raw["tunnel_type"].(string)
		item.PortType, _ = raw["port_type"].(string)

		// Public address can appear under several keys depending on allocation state
		if v, ok := raw["display_address"].(string); ok && v != "" {
			item.PublicAddress = v
		}
		if domain, ok := raw["domain"].(map[string]any); ok {
			if v, ok := domain["name"].(string); ok && v != "" && item.PublicAddress == "" {
				item.PublicAddress = v
			}
			if v, ok := domain["domain"].(string); ok && v != "" && item.PublicAddress == "" {
				item.PublicAddress = v
			}
		}
		if alloc, ok := raw["alloc"].(map[string]any); ok {
			if allocData, ok := alloc["data"].(map[string]any); ok {
				if v, ok := allocData["assigned_domain"].(string); ok && v != "" && item.PublicAddress == "" {
					item.PublicAddress = v
				}
				if v, ok := allocData["ip_hostname"].(string); ok && v != "" && item.PublicAddress == "" {
					item.PublicAddress = v
				}
				if v, ok := allocData["assigned_srv"].(string); ok && v != "" && item.PublicAddress == "" {
					item.PublicAddress = v
				}
				item.PublicPort = firstPositiveInt(allocData["port_start"], allocData["port"])
			}
		}
		if origin, ok := raw["origin"].(map[string]any); ok {
			if originData, ok := origin["data"].(map[string]any); ok {
				item.LocalPort = firstPositiveInt(originData["local_port"], originData["port"])
			}
		}
		if agentConfig, ok := raw["agent_config"].(map[string]any); ok {
			if fields, ok := agentConfig["fields"].([]any); ok {
				for _, f := range fields {
					fmap, ok := f.(map[string]any)
					if !ok || fmap["name"] != "local_port" {
						continue
					}
					switch v := fmap["value"].(type) {
					case string:
						if p, err := strconv.Atoi(v); err == nil && p > 0 {
							item.LocalPort = p
						}
					case float64:
						if v > 0 {
							item.LocalPort = int(v)
						}
					}
				}
			}
		}
		if item.LocalPort <= 0 {
			if item.TunnelType == "minecraft-bedrock" {
				item.LocalPort = 19132
			} else {
				item.LocalPort = 25565
			}
		}
		if item.ID == "" && item.PublicAddress == "" {
			continue
		}
		results = append(results, item)
	}

	return results, nil
}

type playitCreateReq struct {
	Name       string       `json:"name,omitempty"`
	TunnelType string       `json:"tunnel_type,omitempty"`
	PortType   string       `json:"port_type"`
	PortCount  int          `json:"port_count"`
	Enabled    bool         `json:"enabled"`
	Origin     playitOrigin `json:"origin"`
}

type playitOrigin struct {
	Type string            `json:"type"`
	Data playitOriginData  `json:"data"`
}

type playitOriginData struct {
	LocalIP   string `json:"local_ip"`
	LocalPort int    `json:"local_port,omitempty"`
}

// createTunnel provisions a tunnel on the linked playit.gg account
func (c *playitClient) createTunnel(ctx context.Context, secretKey, name, tunnelType, portType string, localPort int) (*playitTunnel, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("secret key is required")
	}
	if portType == "" {
		portType = "tcp"
	}
	if portType == "both" && tunnelType == "minecraft-bedrock" {
		portType = "udp"
	}

	body, err := json.Marshal(playitCreateReq{
		Name:       name,
		TunnelType: tunnelType,
		PortType:   portType,
		PortCount:  1,
		Enabled:    true,
		Origin: playitOrigin{
			Type: "default",
			Data: playitOriginData{LocalIP: "127.0.0.1", LocalPort: localPort},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tunnels/create", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("playit api error (HTTP %d)", resp.StatusCode)
	}

	// Allocation details arrive asynchronously; list to pick up the full record
	tunnels, err := c.listTunnels(ctx, secretKey)
	if err != nil {
		return nil, err
	}
	for _, t := range tunnels {
		if t.LocalPort == localPort {
			return t, nil
		}
	}
	return nil, fmt.Errorf("tunnel created but allocation not visible yet")
}

func (c *playitClient) setHeaders(req *http.Request, secretKey string) {
	req.Header.Set("Content-Type", "application/json")
	cleanKey := strings.TrimSpace(secretKey)
	if strings.HasPrefix(cleanKey, "agent-key ") {
		req.Header.Set("Authorization", cleanKey)
	} else if strings.HasPrefix(cleanKey, "AgentKey ") {
		req.Header.Set("Authorization", "agent-key "+strings.TrimPrefix(cleanKey, "AgentKey "))
	} else {
		req.Header.Set("Authorization", "agent-key "+cleanKey)
	}
}

func firstPositiveInt(values ...any) int {
	for _, v := range values {
		if f, ok := v.(float64); ok && f > 0 {
			return int(f)
		}
	}
	return 0
}

// startPlayitWatcher runs a background loop that syncs playit tunnels for all
// running playit modules: it captures each tunnel's assigned public address
// into module metadata and auto-creates missing tunnels on the linked account.
func (m *Manager) startPlayitWatcher(ctx context.Context) {
	interval := m.config.Module.PlayitPollSeconds
	if interval <= 0 {
		interval = 15
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	m.logger.Info("Playit module watcher started (poll interval: %ds)", interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncPlayitModules(ctx)
		}
	}
}

// syncPlayitModules syncs every playit module; non-running modules get any
// previously discovered address cleared so stale entries are never shown.
func (m *Manager) syncPlayitModules(ctx context.Context) {
	modules, err := m.store.ListModules(ctx)
	if err != nil {
		m.logger.Error("Playit watcher: failed to list modules: %v", err)
		return
	}

	for _, mod := range modules {
		if mod.TemplateID != PlayitTemplateID {
			continue
		}
		if mod.Status != storage.ModuleStatusRunning {
			m.clearPlayitAddress(ctx, mod)
			continue
		}
		if err := m.syncPlayitModule(ctx, mod); err != nil {
			m.logger.Warn("Playit watcher: failed to sync module %s: %v", mod.Name, err)
		}
	}
}

// syncPlayitModule resolves the module's secret key and listen port from its
// environment, finds the matching playit tunnel, and records its public address.
func (m *Manager) syncPlayitModule(ctx context.Context, mod *storage.Module) error {
	env, err := m.resolvePlayitEnv(ctx, mod)
	if err != nil {
		return err
	}

	secretKey := strings.TrimSpace(env["SECRET_KEY"])
	if secretKey == "" {
		return fmt.Errorf("no SECRET_KEY configured")
	}

	listenPort := 25565
	if p, err := strconv.Atoi(strings.TrimSpace(env["LISTEN_PORT"])); err == nil && p > 0 {
		listenPort = p
	}

	client := newPlayitClient(m.config.Module.PlayitAPIURL)
	tunnels, err := client.listTunnels(ctx, secretKey)
	if err != nil {
		return fmt.Errorf("failed to list tunnels: %w", err)
	}

	var match *playitTunnel
	for _, t := range tunnels {
		if t.LocalPort == listenPort && t.PublicAddress != "" {
			match = t
			break
		}
	}

	// Auto-provision the tunnel on the linked account if it doesn't exist yet
	if match == nil && m.config.Module.PlayitAutoCreate {
		tunnelType := "minecraft-java"
		if listenPort == 19132 {
			tunnelType = "minecraft-bedrock"
		}
		created, err := client.createTunnel(ctx, secretKey, mod.Name, tunnelType, "tcp", listenPort)
		if err != nil {
			m.logger.Warn("Playit watcher: could not auto-create tunnel for module %s: %v", mod.Name, err)
		} else {
			m.logger.Info("Playit watcher: auto-created tunnel %q for module %s", created.Name, mod.Name)
			match = created
		}
	}

	if match == nil || match.PublicAddress == "" {
		return fmt.Errorf("no allocated tunnel found for port %d yet", listenPort)
	}

	return m.setPlayitAddress(ctx, mod, match.PublicAddress, match.PublicPort)
}

// resolvePlayitEnv merges template default env with module overrides and
// substitutes aliases the same way the container env is built.
func (m *Manager) resolvePlayitEnv(ctx context.Context, mod *storage.Module) (map[string]string, error) {
	env := map[string]string{}

	template, err := m.store.GetModuleTemplate(ctx, mod.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get module template: %w", err)
	}
	if template.DefaultEnv != "" {
		var defaults map[string]string
		if err := json.Unmarshal([]byte(template.DefaultEnv), &defaults); err == nil {
			for k, v := range defaults {
				env[k] = v
			}
		}
	}
	if mod.EnvOverrides != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(mod.EnvOverrides), &overrides); err == nil {
			for k, v := range overrides {
				env[k] = v
			}
		}
	}

	server, err := m.store.GetServer(ctx, mod.ServerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}
	serverConfig, _ := m.store.GetServerConfig(ctx, mod.ServerID)
	aliasCtx := &alias.Context{
		Server:       server,
		ServerConfig: serverConfig,
		Module:       mod,
		Config:       m.config,
	}
	for k, v := range env {
		env[k] = alias.Substitute(v, aliasCtx)
	}

	return env, nil
}

// setPlayitAddress records the discovered address in module metadata,
// writing to the database only when something actually changed.
func (m *Manager) setPlayitAddress(ctx context.Context, mod *storage.Module, address string, port int) error {
	if mod.Metadata == nil {
		mod.Metadata = map[string]string{}
	}
	portStr := strconv.Itoa(port)
	if mod.Metadata[MetaPublicAddress] == address && mod.Metadata[MetaPublicPort] == portStr {
		return nil
	}

	m.logger.Info("Playit watcher: module %s public address: %s:%d", mod.Name, address, port)
	mod.Metadata[MetaPublicAddress] = address
	mod.Metadata[MetaPublicPort] = portStr
	mod.Metadata[MetaSyncedAt] = time.Now().UTC().Format(time.RFC3339)
	if err := m.store.UpdateModule(ctx, mod); err != nil {
		return fmt.Errorf("failed to update module metadata: %w", err)
	}
	return nil
}

// clearPlayitAddress removes a previously discovered address when the module
// is no longer running
func (m *Manager) clearPlayitAddress(ctx context.Context, mod *storage.Module) {
	if mod.Metadata == nil || mod.Metadata[MetaPublicAddress] == "" {
		return
	}
	delete(mod.Metadata, MetaPublicAddress)
	delete(mod.Metadata, MetaPublicPort)
	delete(mod.Metadata, MetaSyncedAt)
	if err := m.store.UpdateModule(ctx, mod); err != nil {
		m.logger.Error("Playit watcher: failed to clear address for module %s: %v", mod.Name, err)
	}
}
