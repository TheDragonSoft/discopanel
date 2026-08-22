package proxy

import (
	"io"
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
