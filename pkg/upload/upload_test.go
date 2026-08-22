package upload

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/nickheyer/discopanel/pkg/logger"
)

func TestUploadManager_SequentialWriteStream(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "upload_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	log := logger.New()
	mgr := NewManager(tempDir, 1*time.Hour, 100*1024*1024, log)
	defer mgr.Stop()

	data := []byte("hello contiguous upload stream world!")
	totalSize := int64(len(data))

	session, err := mgr.InitSession("stream.txt", totalSize, 64*1024)
	if err != nil {
		t.Fatalf("failed to init session: %v", err)
	}

	// First stream write: first 10 bytes
	part1 := bytes.NewReader(data[:10])
	written1, completed1, err := mgr.WriteStream(session.ID, part1, 0)
	if err != nil {
		t.Fatalf("failed part1 write: %v", err)
	}
	if written1 != 10 || completed1 {
		t.Fatalf("expected written=10, completed=false; got written=%d, completed=%v", written1, completed1)
	}

	// Out-of-order write should be rejected (offset 20 instead of 10)
	partInvalid := bytes.NewReader(data[20:])
	_, _, err = mgr.WriteStream(session.ID, partInvalid, 20)
	if err == nil {
		t.Fatal("expected error on non-contiguous offset jump, got nil")
	}

	// Second stream write: remaining bytes at offset 10
	part2 := bytes.NewReader(data[10:])
	written2, completed2, err := mgr.WriteStream(session.ID, part2, 10)
	if err != nil {
		t.Fatalf("failed part2 write: %v", err)
	}
	if written2 != int64(len(data)-10) || !completed2 {
		t.Fatalf("expected written=%d, completed=true; got written=%d, completed=%v", len(data)-10, written2, completed2)
	}

	tempPath, origName, err := mgr.GetTempPath(session.ID)
	if err != nil {
		t.Fatalf("failed to get temp path: %v", err)
	}
	if origName != "stream.txt" {
		t.Fatalf("expected original name 'stream.txt', got '%s'", origName)
	}

	savedData, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("failed to read uploaded file: %v", err)
	}
	if !bytes.Equal(savedData, data) {
		t.Fatalf("uploaded content mismatch: expected '%s', got '%s'", string(data), string(savedData))
	}
}

func TestUploadManager_WriteChunkAndCompletion(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "upload_chunk_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	log := logger.New()
	mgr := NewManager(tempDir, 1*time.Hour, 100*1024*1024, log)
	defer mgr.Stop()

	chunk1 := []byte("chunk number one ")
	chunk2 := []byte("chunk number two")
	fullData := append(chunk1, chunk2...)
	chunkSize := int32(len(chunk1))

	session, err := mgr.InitSession("chunked.txt", int64(len(fullData)), chunkSize)
	if err != nil {
		t.Fatalf("failed to init session: %v", err)
	}

	// Write chunk 0
	comp1, err := mgr.WriteChunk(session.ID, 0, chunk1)
	if err != nil {
		t.Fatalf("failed to write chunk 0: %v", err)
	}
	if comp1 {
		t.Fatal("expected session not completed after chunk 0")
	}

	// Write chunk 1
	comp2, err := mgr.WriteChunk(session.ID, 1, chunk2)
	if err != nil {
		t.Fatalf("failed to write chunk 1: %v", err)
	}
	if !comp2 {
		t.Fatal("expected session completed after chunk 1")
	}

	// Cleanup session
	mgr.CleanupSession(session.ID)
	_, _, err = mgr.GetTempPath(session.ID)
	if err == nil {
		t.Fatal("expected error getting path for cleaned-up session, got nil")
	}
}
