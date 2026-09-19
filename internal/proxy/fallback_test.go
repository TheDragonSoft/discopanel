package proxy

import (
	"bytes"
	"testing"
)

func TestShouldRouteToFallback(t *testing.T) {
	tests := []struct {
		name            string
		fallbackID      string
		originalID      string
		containerID     string
		ip              string
		want            bool
	}{
		{"no fallback configured", "", "srv-1", "abc", "10.0.0.2", false},
		{"fallback configured and running", "srv-lobby", "srv-1", "abc", "10.0.0.2", true},
		{"original route is the fallback", "srv-lobby", "srv-lobby", "abc", "10.0.0.2", false},
		{"fallback has no container", "srv-lobby", "srv-1", "", "10.0.0.2", false},
		{"fallback container unreachable", "srv-lobby", "srv-1", "abc", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRouteToFallback(tt.fallbackID, tt.originalID, tt.containerID, tt.ip)
			if got != tt.want {
				t.Errorf("shouldRouteToFallback(%q, %q, %q, %q) = %v, want %v",
					tt.fallbackID, tt.originalID, tt.containerID, tt.ip, got, tt.want)
			}
		})
	}
}

func TestConnLimiterAcquireRelease(t *testing.T) {
	l := newConnLimiter()

	// Unlimited (limit 0): many acquires all succeed.
	for i := 0; i < 100; i++ {
		if !l.acquire("srv-a", 0) {
			t.Fatalf("acquire with unlimited limit rejected connection #%d", i+1)
		}
	}
	if got := l.count("srv-a"); got != 100 {
		t.Errorf("count = %d, want 100", got)
	}

	// Release everything; count must drop back to zero (and stay tracked).
	for i := 0; i < 100; i++ {
		l.release("srv-a")
	}
	if got := l.count("srv-a"); got != 0 {
		t.Errorf("count after releasing all = %d, want 0", got)
	}

	// Over-release must not go negative or resurrect counts.
	l.release("srv-a")
	if got := l.count("srv-a"); got != 0 {
		t.Errorf("count after over-release = %d, want 0", got)
	}
}

func TestConnLimiterEnforcesLimit(t *testing.T) {
	l := newConnLimiter()

	for i := 0; i < 3; i++ {
		if !l.acquire("srv-b", 3) {
			t.Fatalf("acquire #%d rejected, want accepted (limit 3)", i+1)
		}
	}
	if got := l.count("srv-b"); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
	if l.acquire("srv-b", 3) {
		t.Error("acquire beyond limit accepted, want rejected")
	}
	// A rejected acquire must not have consumed a slot.
	if got := l.count("srv-b"); got != 3 {
		t.Errorf("count after rejected acquire = %d, want 3", got)
	}

	// One release frees exactly one slot.
	l.release("srv-b")
	if !l.acquire("srv-b", 3) {
		t.Error("acquire after release rejected, want accepted")
	}

	// Other servers are unaffected by srv-b's limit.
	if !l.acquire("srv-c", 3) {
		t.Error("acquire for unrelated server rejected, want accepted")
	}
}

func TestConnLimiterNilAndEmptyServer(t *testing.T) {
	var l *connLimiter
	if !l.acquire("srv-x", 1) {
		t.Error("nil limiter must allow connections")
	}
	if got := l.count("srv-x"); got != 0 {
		t.Errorf("nil limiter count = %d, want 0", got)
	}
	l.release("srv-x")

	real := newConnLimiter()
	if !real.acquire("", 1) {
		t.Error("empty server ID must be allowed")
	}
}

func TestWriteLoginDisconnectPacket(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteLoginDisconnectPacket(&buf, "full"); err != nil {
		t.Fatalf("WriteLoginDisconnectPacket returned error: %v", err)
	}

	// Payload: packet ID 0x00 + varint length + `{"text":"full"}` (15 bytes).
	payload := []byte{0x00, 0x0f}
	payload = append(payload, []byte(`{"text":"full"}`)...)
	// Framed: varint(len(payload)) = 0x11 + payload.
	want := append([]byte{0x11}, payload...)

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("packet = %x, want %x", buf.Bytes(), want)
	}
}
