package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ServerTemplate CRUD operations

// CreateServerTemplate persists a new server template
func (s *Store) CreateServerTemplate(ctx context.Context, template *ServerTemplate) error {
	if err := s.db.WithContext(ctx).Create(template).Error; err != nil {
		return fmt.Errorf("failed to create server template: %w", err)
	}
	return nil
}

// GetServerTemplate fetches a single server template by ID
func (s *Store) GetServerTemplate(ctx context.Context, id string) (*ServerTemplate, error) {
	var template ServerTemplate
	err := s.db.WithContext(ctx).First(&template, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("server template not found")
		}
		return nil, err
	}
	return &template, nil
}

// ListServerTemplates lists all server templates
func (s *Store) ListServerTemplates(ctx context.Context) ([]*ServerTemplate, error) {
	var templates []*ServerTemplate
	err := s.db.WithContext(ctx).Order("created_at DESC").Find(&templates).Error
	return templates, err
}

// UpdateServerTemplate saves changes to an existing server template
func (s *Store) UpdateServerTemplate(ctx context.Context, template *ServerTemplate) error {
	if err := s.db.WithContext(ctx).Save(template).Error; err != nil {
		return fmt.Errorf("failed to update server template: %w", err)
	}
	return nil
}

// DeleteServerTemplate removes a server template by ID
func (s *Store) DeleteServerTemplate(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Delete(&ServerTemplate{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete server template: %w", err)
	}
	return nil
}
