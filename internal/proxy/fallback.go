package proxy

import (
	"context"
)

// fallbackPort is the internal Minecraft port of the fallback (lobby) backend.
// Server routes always target container port 25565.
const fallbackPort = 25565

// shouldRouteToFallback is the pure decision core of Manager.fallbackResolve.
// A fallback applies only when one is configured, it is not the server the
// connection was originally routed to (avoids routing loops), and the
// fallback server actually has a running container reachable on the panel
// network (container ID and a resolved IP). When it returns false the proxy
// behaves as before: the connection is closed.
func shouldRouteToFallback(fallbackServerID, originalServerID, fallbackContainerID, fallbackIP string) bool {
	if fallbackServerID == "" {
		return false
	}
	// Never route to the fallback if the original route WAS the fallback
	// server; that would loop forever on a dead lobby.
	if fallbackServerID == originalServerID {
		return false
	}
	if fallbackContainerID == "" || fallbackIP == "" {
		return false
	}
	return true
}

// fallbackResolve returns the fallback (lobby) backend for a connection whose
// original route target is unknown or offline. It is consulted by server
// listener Minecraft proxies (via fallbackResolver) only after wake-on-connect
// had its chance: if the target server was woken up successfully the proxy
// continues to the target, and fallback applies only when waking is disabled
// or failed. ok=false means no usable fallback is configured and the caller
// should behave as today (close the connection).
func (m *Manager) fallbackResolve(hostname, originalServerID string) (serverID string, backendHost string, backendPort int, ok bool) {
	ctx := context.Background()

	proxyCfg, _, err := m.store.GetProxyConfig(ctx)
	if err != nil || proxyCfg == nil {
		return "", "", 0, false
	}

	fallbackID := proxyCfg.FallbackServerID
	if fallbackID == "" {
		return "", "", 0, false
	}

	server, err := m.store.GetServer(ctx, fallbackID)
	if err != nil {
		m.logger.Debug("Fallback routing: configured fallback server %s not found", fallbackID)
		return "", "", 0, false
	}

	// The fallback server must be running to accept lobby traffic; if it is
	// not up we do not wake it and behave as if no fallback was configured.
	ip := ""
	if server.ContainerID != "" {
		if resolved, err := GetContainerIP(server.ContainerID, m.networkName); err == nil {
			ip = resolved
		}
	}

	if !shouldRouteToFallback(fallbackID, originalServerID, server.ContainerID, ip) {
		return "", "", 0, false
	}

	m.logger.Info("routing %s to fallback %s", hostname, server.Name)
	return fallbackID, ip, fallbackPort, true
}

// serverConnectionLimit returns the configured connection limit for a server
// (0 = unlimited). Unknown servers report unlimited; the limiter rejects
// nothing it cannot attribute.
func (m *Manager) serverConnectionLimit(serverID string) int {
	server, err := m.store.GetServer(context.Background(), serverID)
	if err != nil {
		return 0
	}
	return server.ConnectionLimit
}
