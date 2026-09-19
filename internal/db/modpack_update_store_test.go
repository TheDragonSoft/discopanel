package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nickheyer/discopanel/internal/config"
)

func newModpackUpdateTestStore(t *testing.T) *Store {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "modpack_update_store_test_*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Path:        filepath.Join(tempDir, "test.db"),
			AutoMigrate: true,
		},
	}
	store, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestModpackUpdateSettingRoundTrip(t *testing.T) {
	store := newModpackUpdateTestStore(t)
	ctx := context.Background()

	// Missing row returns nil, nil (defaults apply)
	setting, err := store.GetModpackUpdateSetting(ctx, "server-1")
	if err != nil {
		t.Fatalf("expected no error for missing setting, got %v", err)
	}
	if setting != nil {
		t.Fatalf("expected nil setting for unknown server, got %+v", setting)
	}

	// Upsert
	err = store.SetModpackUpdateSetting(ctx, &ModpackUpdateSetting{
		ServerID:      "server-1",
		Enabled:       true,
		IntervalHours: 6,
		Mode:          "apply",
	})
	if err != nil {
		t.Fatalf("failed to save setting: %v", err)
	}

	setting, err = store.GetModpackUpdateSetting(ctx, "server-1")
	if err != nil || setting == nil {
		t.Fatalf("expected stored setting, got %+v err=%v", setting, err)
	}
	if !setting.Enabled || setting.IntervalHours != 6 || setting.Mode != "apply" {
		t.Fatalf("unexpected stored setting: %+v", setting)
	}

	// Interval is clamped to at least 1
	err = store.SetModpackUpdateSetting(ctx, &ModpackUpdateSetting{
		ServerID:      "server-1",
		Enabled:       true,
		IntervalHours: 0,
		Mode:          "notify",
	})
	if err != nil {
		t.Fatalf("failed to save clamped setting: %v", err)
	}
	setting, _ = store.GetModpackUpdateSetting(ctx, "server-1")
	if setting.IntervalHours != 1 {
		t.Fatalf("expected interval clamped to 1, got %d", setting.IntervalHours)
	}
}

func TestRecordModpackUpdateCheckCreatesRow(t *testing.T) {
	store := newModpackUpdateTestStore(t)
	ctx := context.Background()

	if err := store.RecordModpackUpdateCheck(ctx, "server-1", "up to date"); err != nil {
		t.Fatalf("failed to record check: %v", err)
	}
	setting, err := store.GetModpackUpdateSetting(ctx, "server-1")
	if err != nil || setting == nil {
		t.Fatalf("expected setting row created by check, got %+v err=%v", setting, err)
	}
	if setting.LastCheck == nil {
		t.Fatal("expected LastCheck to be set")
	}
	if setting.LastResult != "up to date" {
		t.Fatalf("expected LastResult %q, got %q", "up to date", setting.LastResult)
	}

	// RecordModpackUpdate keeps check bookkeeping and stores the backup name
	if err := store.RecordModpackUpdate(ctx, "server-1", "pre-modpack-update_20260101-000000.zip", "updated"); err != nil {
		t.Fatalf("failed to record update: %v", err)
	}
	setting, _ = store.GetModpackUpdateSetting(ctx, "server-1")
	if setting.LastBackupFilename != "pre-modpack-update_20260101-000000.zip" {
		t.Fatalf("unexpected LastBackupFilename: %q", setting.LastBackupFilename)
	}
	if setting.LastResult != "updated" {
		t.Fatalf("unexpected LastResult after update: %q", setting.LastResult)
	}
}

func TestListEnabledModpackUpdateSettings(t *testing.T) {
	store := newModpackUpdateTestStore(t)
	ctx := context.Background()

	if err := store.SetModpackUpdateSetting(ctx, &ModpackUpdateSetting{ServerID: "a", Enabled: true, IntervalHours: 24, Mode: "notify"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetModpackUpdateSetting(ctx, &ModpackUpdateSetting{ServerID: "b", Enabled: false, IntervalHours: 24, Mode: "notify"}); err != nil {
		t.Fatal(err)
	}

	enabled, err := store.ListEnabledModpackUpdateSettings(ctx)
	if err != nil {
		t.Fatalf("failed to list enabled settings: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ServerID != "a" {
		t.Fatalf("expected only server 'a' enabled, got %+v", enabled)
	}
}
