package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickheyer/discopanel/internal/config"
)

func TestNewSQLiteStore_WALAndPragmas(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "db_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Path:           dbPath,
			MaxConnections: 10,
			MaxIdleConns:   5,
			AutoMigrate:    true,
		},
	}

	store, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()

	// Verify journal_mode is WAL
	var journalMode string
	err = store.db.Raw("PRAGMA journal_mode;").Scan(&journalMode).Error
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("expected journal_mode 'wal', got '%s'", journalMode)
	}

	// Verify foreign_keys is ON
	var foreignKeys int
	err = store.db.Raw("PRAGMA foreign_keys;").Scan(&foreignKeys).Error
	if err != nil {
		t.Fatalf("failed to query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign_keys 1, got %d", foreignKeys)
	}

	// Verify busy_timeout is 5000
	var busyTimeout int
	err = store.db.Raw("PRAGMA busy_timeout;").Scan(&busyTimeout).Error
	if err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout 5000, got %d", busyTimeout)
	}

	// Verify Server CRUD operations
	ctx := context.Background()
	srv := &Server{
		ID:          "test-srv-1",
		Name:        "Test Server",
		ModLoader:   ModLoaderPaper,
		MCVersion:   "1.21.1",
		Status:      StatusStopped,
		Port:        25565,
		DataPath:    tempDir,
	}

	err = store.CreateServer(ctx, srv)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	fetched, err := store.GetServer(ctx, "test-srv-1")
	if err != nil {
		t.Fatalf("failed to get server: %v", err)
	}
	if fetched.Name != "Test Server" {
		t.Fatalf("expected name 'Test Server', got '%s'", fetched.Name)
	}
}
