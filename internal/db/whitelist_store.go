package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Whitelist operations. The panel keeps its own desired-state list; servers
// are converged to it over RCON by the admin service.

// ListWhitelistEntries returns entries ordered by name, optionally filtered by
// a case-insensitive name substring.
func (s *Store) ListWhitelistEntries(ctx context.Context, search string) ([]*WhitelistEntry, error) {
	query := s.db.WithContext(ctx).Model(&WhitelistEntry{})
	if search != "" {
		query = query.Where("name LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	var entries []*WhitelistEntry
	err := query.Order("name ASC").Find(&entries).Error
	return entries, err
}

// UpsertWhitelistEntry creates the entry for name, or updates the note of the
// existing one. Returns the stored entry.
func (s *Store) UpsertWhitelistEntry(ctx context.Context, name, note string) (*WhitelistEntry, error) {
	var existing WhitelistEntry
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	if err == nil {
		if err := s.db.WithContext(ctx).Model(&WhitelistEntry{}).Where("id = ?", existing.ID).
			Update("note", note).Error; err != nil {
			return nil, fmt.Errorf("failed to update whitelist entry: %w", err)
		}
		existing.Note = note
		return &existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	entry := &WhitelistEntry{
		ID:   uuid.New().String(),
		Name: name,
		Note: note,
	}
	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		// A concurrent create for the same name may have won the unique
		// index; retry as an update.
		var raced WhitelistEntry
		if findErr := s.db.WithContext(ctx).Where("name = ?", name).First(&raced).Error; findErr == nil {
			if updErr := s.db.WithContext(ctx).Model(&WhitelistEntry{}).Where("id = ?", raced.ID).
				Update("note", note).Error; updErr != nil {
				return nil, fmt.Errorf("failed to update whitelist entry: %w", updErr)
			}
			raced.Note = note
			return &raced, nil
		}
		return nil, fmt.Errorf("failed to create whitelist entry: %w", err)
	}
	return entry, nil
}

// DeleteWhitelistEntry removes an entry by ID. Returns an error when the
// entry does not exist.
func (s *Store) DeleteWhitelistEntry(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Where("id = ?", id).Delete(&WhitelistEntry{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("whitelist entry not found")
	}
	return nil
}

// ListWhitelistNames returns just the desired names, for apply diffing.
func (s *Store) ListWhitelistNames(ctx context.Context) ([]string, error) {
	var names []string
	err := s.db.WithContext(ctx).Model(&WhitelistEntry{}).Order("name ASC").Pluck("name", &names).Error
	return names, err
}
