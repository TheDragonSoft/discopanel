package proxy

import (
	"bytes"
	"testing"
)

// encodeMCPacket builds a framed Minecraft packet: varint length followed by
// the packet payload (varint packet ID + fields).
func encodeMCPacket(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteVarInt(&buf, VarInt(len(payload))); err != nil {
		t.Fatalf("failed to encode packet length: %v", err)
	}
	buf.Write(payload)
	return buf.Bytes()
}

// encodeMCString encodes a Minecraft varint-prefixed string.
func encodeMCString(s string) []byte {
	var buf bytes.Buffer
	if err := WriteVarInt(&buf, VarInt(len(s))); err != nil {
		return nil
	}
	buf.WriteString(s)
	return buf.Bytes()
}

func TestReadLoginStartPacket(t *testing.T) {
	// Packet ID 0x00 (login start) + username
	payload := append([]byte{0x00}, encodeMCString("Steve")...)
	raw := encodeMCPacket(t, payload)

	pkt, got, err := ReadLoginStartPacket(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadLoginStartPacket failed: %v", err)
	}
	if pkt.Username != "Steve" {
		t.Errorf("expected username Steve, got %q", pkt.Username)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("raw bytes mismatch: got %v, want %v", got, raw)
	}
}

func TestReadLoginStartPacket_ExtraFields(t *testing.T) {
	// 1.20.2+ appends a profile UUID (16 bytes) after the username; the parser
	// must still extract the username and consume the full packet.
	uuidBytes := make([]byte, 16)
	for i := range uuidBytes {
		uuidBytes[i] = byte(i)
	}
	payload := append(append([]byte{0x00}, encodeMCString("Alex")...), uuidBytes...)
	raw := encodeMCPacket(t, payload)

	pkt, got, err := ReadLoginStartPacket(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadLoginStartPacket failed: %v", err)
	}
	if pkt.Username != "Alex" {
		t.Errorf("expected username Alex, got %q", pkt.Username)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("raw bytes mismatch: got %v, want %v", got, raw)
	}
}

func TestReadLoginStartPacket_WrongPacketID(t *testing.T) {
	// Encryption response (0x01) is not a login start: the parser must return
	// an error but still hand back the fully consumed raw bytes for forwarding.
	payload := append([]byte{0x01}, bytes.Repeat([]byte{0xAB}, 8)...)
	raw := encodeMCPacket(t, payload)

	pkt, got, err := ReadLoginStartPacket(bytes.NewReader(raw))
	if err == nil {
		t.Fatalf("expected error for non-login-start packet")
	}
	if pkt != nil {
		t.Errorf("expected nil packet, got %+v", pkt)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("raw bytes mismatch: got %v, want %v", got, raw)
	}
}

func TestReadLoginStartPacket_Truncated(t *testing.T) {
	// An incomplete packet is an IO error: no raw bytes are returned since the
	// connection cannot be resynchronized.
	raw := encodeMCPacket(t, append([]byte{0x00}, encodeMCString("Notch")...))
	raw = raw[:len(raw)-3]

	if _, got, err := ReadLoginStartPacket(bytes.NewReader(raw)); err == nil {
		t.Fatalf("expected error for truncated packet")
	} else if got != nil {
		t.Errorf("expected nil raw bytes on IO error, got %v", got)
	}
}
