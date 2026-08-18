package tunnel

import (
	"testing"
)

func TestPlayitDriverSniffLogs(t *testing.T) {
	driver := &PlayitDriver{}

	t.Run("Sniff claim URL and claim code", func(t *testing.T) {
		logs := []string{
			"playit (0.15.26): starting up",
			"claim url: https://playit.gg/claim/abc-123-xyz",
			"waiting for user confirmation",
		}

		claimURL, claimCode, publicAddr, publicPort, isRunning := driver.SniffLogs(logs)
		if claimURL != "https://playit.gg/claim/abc-123-xyz" {
			t.Errorf("expected claim URL 'https://playit.gg/claim/abc-123-xyz', got '%s'", claimURL)
		}
		if claimCode != "abc-123-xyz" {
			t.Errorf("expected claim code 'abc-123-xyz', got '%s'", claimCode)
		}
		if isRunning {
			t.Errorf("expected isRunning to be false")
		}
		if publicAddr != "" || publicPort != 0 {
			t.Errorf("expected no public address or port, got %s:%d", publicAddr, publicPort)
		}
	})

	t.Run("Sniff joinmc public domain", func(t *testing.T) {
		logs := []string{
			"tunnel registered successfully",
			"server online at my-server.gl.joinmc.link",
			"ready to accept connections",
		}

		claimURL, _, publicAddr, _, isRunning := driver.SniffLogs(logs)
		if claimURL != "" {
			t.Errorf("expected empty claim URL, got '%s'", claimURL)
		}
		if publicAddr != "my-server.gl.joinmc.link" {
			t.Errorf("expected public address 'my-server.gl.joinmc.link', got '%s'", publicAddr)
		}
		if !isRunning {
			t.Errorf("expected isRunning to be true")
		}
	})

	t.Run("Sniff tun.ply.gg domain from client connection logs", func(t *testing.T) {
		logs := []string{
			"INFO playit_agent_core::network::tcp::tcp_clients: New TCP Client details=NewClient { connect_addr: 147.185.221.22:36104, domain: reminded-known.tun.ply.gg }",
		}

		_, _, publicAddr, _, isRunning := driver.SniffLogs(logs)
		if publicAddr != "reminded-known.tun.ply.gg" {
			t.Errorf("expected public address 'reminded-known.tun.ply.gg', got '%s'", publicAddr)
		}
		if !isRunning {
			t.Errorf("expected isRunning to be true")
		}
	})
}
