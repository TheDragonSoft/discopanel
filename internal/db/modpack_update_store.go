package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Modpack update settings operations. One row per server (ServerID is the
// primary key); a missing row means "use defaults" (disabled, 24h, notify).

// GetModpackUpdateSetting returns the stored setting for a server, or
// (nil, nil) when the server has no settings row yet.
func (s *Store) GetModpackUpdateSetting(ctx context.Context, serverID string) (*ModpackUpdateSetting, error) {
	var setting ModpackUpdateSetting
	err := s.db.WithContext(ctx).First(&setting, "server_id = ?", serverID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get modpack update setting: %w", err)
	}
	return &setting, nil
}

// SetModpackUpdateSetting upserts the settings row for a server.
func (s *Store) SetModpackUpdateSetting(ctx context.Context, setting *ModpackUpdateSetting) error {
	if setting == nil {
		return fmt.Errorf("setting is required")
	}
	if setting.ServerID == "" {
		return fmt.Errorf("server_id is required")
	}
	// Clamp a non-positive interval so a bad value can never disable the loop.
	if setting.IntervalHours < 1 {
		setting.IntervalHours = 1
	}
	err := s.db.WithContext(ctx).Save(setting).Error
	if err != nil {
		return fmt.Errorf("failed to save modpack update setting: %w", err)
	}
	return nil
}

// ListEnabledModpackUpdateSettings returns settings rows for servers with
// scheduled update checks enabled.
func (s *Store) ListEnabledModpackUpdateSettings(ctx context.Context) ([]*ModpackUpdateSetting, error) {
	var settings []*ModpackUpdateSetting
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&settings).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled modpack update settings: %w", err)
	}
	return settings, nil
}

// RecordModpackUpdateCheck stores the outcome of an update check: it refreshes
// LastCheck and LastResult. A missing row is created with defaults so the
// check is remembered even when the feature has not been configured yet.
func (s *Store) RecordModpackUpdateCheck(ctx context.Context, serverID string, result string) error {
	return s.recordModpackUpdateResult(ctx, serverID, func(setting *ModpackUpdateSetting) {
		now := time.Now()
		setting.LastCheck = &now
		setting.LastResult = result
	})
}

// RecordModpackUpdate stores the outcome of an applied modpack update: the
// pre-update backup filename (for rollback) and the result message.
func (s *Store) RecordModpackUpdate(ctx context.Context, serverID string, backupFilename string, result string) error {
	return s.recordModpackUpdateResult(ctx, serverID, func(setting *ModpackUpdateSetting) {
		setting.LastBackupFilename = backupFilename
		setting.LastResult = result
	})
}

// recordModpackUpdateResult loads (or creates) the settings row and applies
// the mutation inside one transaction.
func (s *Store) recordModpackUpdateResult(ctx context.Context, serverID string, mutate func(*ModpackUpdateSetting)) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var setting ModpackUpdateSetting
		err := tx.First(&setting, "server_id = ?", serverID).Error
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("failed to load modpack update setting: %w", err)
			}
			setting = ModpackUpdateSetting{
				ServerID:      serverID,
				IntervalHours: 24,
				Mode:          "notify",
			}
		}
		mutate(&setting)
		if err := tx.Save(&setting).Error; err != nil {
			return fmt.Errorf("failed to save modpack update setting: %w", err)
		}
		return nil
	})
}

// GetIndexedModpackByIndexerID looks up an indexed modpack by its original
// indexer-scoped ID (e.g. Modrinth project ID used in MODRINTH_MODPACK).
func (s *Store) GetIndexedModpackByIndexerID(ctx context.Context, indexer, indexerID string) (*IndexedModpack, error) {
	var modpack IndexedModpack
	err := s.db.WithContext(ctx).First(&modpack, "indexer = ? AND indexer_id = ?", indexer, indexerID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("modpack not found")
		}
		return nil, err
	}
	return &modpack, nil
}
