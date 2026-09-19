package proxy

import (
	"io"
	"net"
	"sync"

	"github.com/nickheyer/discopanel/pkg/logger"
)

// Proxier is the interface for all proxy types (TCP, UDP, Minecraft, HTTP)
type Proxier interface {
	Start() error
	Stop() error
	AddRoute(serverID, hostname, backendHost string, backendPort int)
	RemoveRoute(hostname string)
	UpdateRoute(hostname, backendHost string, backendPort int)
	GetRoutes() map[string]*Route
	IsRunning() bool
}

// Route represents a routing rule from hostname to backend server
type Route struct {
	ServerID    string
	Hostname    string
	BackendHost string
	BackendPort int
	Active      bool
}

// Config holds proxy configuration
type Config struct {
	ListenAddr string // Address to listen on (e.g., ":25565" or ":8080")
	Logger     *logger.Logger

	// IngressProxyProtocol parses PROXY protocol v1/v2 headers on accepted
	// connections so an external edge (VPS tunnel, Pangolin, HAProxy) can
	// pass through the real client address.
	IngressProxyProtocol bool
	// TrustedProxies restricts which upstream CIDRs may send PROXY protocol
	// headers (empty = accept headers from any upstream).
	TrustedProxies []string
}

// wrapIngress applies PROXY protocol parsing to a freshly bound listener when
// configured. On wrap failure the original listener is closed and returned
// with the error so callers can treat it as a start failure.
func wrapIngress(listener net.Listener, cfg *Config) (net.Listener, error) {
	if cfg == nil || !cfg.IngressProxyProtocol {
		return listener, nil
	}
	wrapped, err := WrapIngressListener(listener, cfg.TrustedProxies)
	if err != nil {
		listener.Close()
		return nil, err
	}
	return wrapped, nil
}

var proxyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

// proxyCopy performs low-alloc streaming between reader and writer using a pooled buffer.
func proxyCopy(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := proxyBufPool.Get().(*[]byte)
	defer proxyBufPool.Put(bufPtr)
	return io.CopyBuffer(dst, src, *bufPtr)
}
