package docker

import "testing"

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		host     string
		want     string
	}{
		{"docker default", "docker", "unix:///var/run/docker.sock", "unix:///var/run/docker.sock"},
		{"docker explicit", "docker", "tcp://127.0.0.1:2375", "tcp://127.0.0.1:2375"},
		{"podman explicit wins", "podman", "unix:///custom/podman.sock", "unix:///custom/podman.sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveHost(tt.provider, tt.host); got != tt.want {
				t.Errorf("resolveHost(%q, %q) = %q, want %q", tt.provider, tt.host, got, tt.want)
			}
		})
	}
}

func TestResolveHostPodmanDefault(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/user-runtime")
	if got := resolveHost("podman", ""); got != "unix:///tmp/user-runtime/podman/podman.sock" {
		t.Errorf("podman rootless default = %q, want unix:///tmp/user-runtime/podman/podman.sock", got)
	}
	if got := resolveHost("podman", "unix:///var/run/docker.sock"); got != "unix:///tmp/user-runtime/podman/podman.sock" {
		t.Errorf("podman default should override docker default host, got %q", got)
	}
}

func TestResolveHostPodmanFallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := resolveHost("podman", ""); got != "unix:///run/podman/podman.sock" {
		t.Errorf("podman rootful fallback = %q, want unix:///run/podman/podman.sock", got)
	}
}
