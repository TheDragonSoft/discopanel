package tunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlayitAPIClient(t *testing.T) {
	t.Run("CreateTunnel success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == "/tunnels/create" {
				_, _ = w.Write([]byte(`{"status": "success", "data": {"id": "tun-123"}}`))
				return
			}
			if r.URL.Path == "/tunnels/list" {
				_, _ = w.Write([]byte(`{
					"status": "success",
					"data": {
						"tunnels": [
							{
								"id": "tun-123",
								"name": "My Java Tunnel",
								"tunnel_type": "minecraft-java",
								"port_type": "both",
								"alloc": {
									"data": {
										"assigned_domain": "reminded-known.tun.ply.gg",
										"port_start": 36104
									}
								},
								"origin": {
									"data": {
										"local_port": 25565
									}
								}
							}
						]
					}
				}`))
				return
			}
			t.Errorf("unexpected path: %s", r.URL.Path)
		}))
		defer ts.Close()

		client := &PlayitAPIClient{
			httpClient: ts.Client(),
			baseURL:    ts.URL,
		}

		res, err := client.CreateTunnel(context.Background(), "test-secret", "My Java Tunnel", "minecraft-java", "both", 25565)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.PublicAddress != "reminded-known.tun.ply.gg" {
			t.Errorf("expected public address 'reminded-known.tun.ply.gg', got '%s'", res.PublicAddress)
		}
		if res.PublicPort != 36104 {
			t.Errorf("expected public port 36104, got %d", res.PublicPort)
		}
	})

	t.Run("SetupClaim and ExchangeClaim success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/claim/setup" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"success","data":"WaitingForUserVisit"}`))
				return
			}
			if r.URL.Path == "/claim/exchange" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"success","data":{"secret_key":"secret-abc-123"}}`))
				return
			}
			t.Errorf("unexpected path: %s", r.URL.Path)
		}))
		defer ts.Close()

		client := &PlayitAPIClient{
			httpClient: ts.Client(),
			baseURL:    ts.URL,
		}

		if err := client.SetupClaim(context.Background(), "test-code"); err != nil {
			t.Fatalf("SetupClaim error: %v", err)
		}

		secretKey, err := client.ExchangeClaim(context.Background(), "test-code")
		if err != nil {
			t.Fatalf("ExchangeClaim error: %v", err)
		}
		if secretKey != "secret-abc-123" {
			t.Errorf("expected 'secret-abc-123', got '%s'", secretKey)
		}
	})
}
