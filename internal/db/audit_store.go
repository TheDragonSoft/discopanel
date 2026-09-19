package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditRetentionDays is how long audit entries are kept; entries older than
// this are pruned once at startup.
const AuditRetentionDays = 7

// AuditEntry operations.

// InsertAuditEntry stores one audit record.
func (s *Store) InsertAuditEntry(ctx context.Context, entry *AuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	return s.db.WithContext(ctx).Create(entry).Error
}

// ListAuditEntries returns entries newest first with optional
// username/resource/action filters, plus the total count matching the filters.
func (s *Store) ListAuditEntries(ctx context.Context, username, resource, action string, limit, offset int) ([]*AuditEntry, int64, error) {
	conds := "1 = 1"
	var args []any
	if username != "" {
		conds += " AND username = ?"
		args = append(args, username)
	}
	if resource != "" {
		conds += " AND resource = ?"
		args = append(args, resource)
	}
	if action != "" {
		conds += " AND action = ?"
		args = append(args, action)
	}

	var total int64
	if err := s.db.WithContext(ctx).Model(&AuditEntry{}).Where(conds, args...).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 100
	}

	var entries []*AuditEntry
	err := s.db.WithContext(ctx).
		Where(conds, args...).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// ClearAuditEntries deletes entries created before before (all entries when
// before is nil) and returns the number of deleted rows.
func (s *Store) ClearAuditEntries(ctx context.Context, before *time.Time) (int64, error) {
	query := s.db.WithContext(ctx)
	if before != nil {
		query = query.Where("created_at < ?", *before)
	} else {
		query = query.Where("1 = 1")
	}
	result := query.Delete(&AuditEntry{})
	return result.RowsAffected, result.Error
}

// PruneAuditEntries deletes entries older than the given time and returns the
// number of deleted rows. Used for the startup retention prune (see
// AuditRetentionDays).
func (s *Store) PruneAuditEntries(ctx context.Context, olderThan time.Time) (int64, error) {
	result := s.db.WithContext(ctx).Where("created_at < ?", olderThan).Delete(&AuditEntry{})
	return result.RowsAffected, result.Error
}
