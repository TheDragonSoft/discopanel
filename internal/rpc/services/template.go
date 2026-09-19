package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/nickheyer/discopanel/internal/config"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/minecraft"
	"github.com/nickheyer/discopanel/pkg/files"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that TemplateService implements the interface
var _ discopanelv1connect.TemplateServiceHandler = (*TemplateService)(nil)

// TemplateService implements the Template service
type TemplateService struct {
	store   *storage.Store
	config  *config.Config
	servers *ServerService
	log     *logger.Logger
}

// NewTemplateService creates a new template service
func NewTemplateService(store *storage.Store, config *config.Config, servers *ServerService, log *logger.Logger) *TemplateService {
	return &TemplateService{
		store:   store,
		config:  config,
		servers: servers,
		log:     log,
	}
}

// templateModsDir returns the directory holding a template's captured mods.
// Layout: <storage.data_dir>/templates/<template_id>/mods
func (s *TemplateService) templateModsDir(templateID string) string {
	return filepath.Join(s.config.Storage.DataDir, "templates", templateID, "mods")
}

// dbTemplateToProto converts a database server template to proto
func dbTemplateToProto(t *storage.ServerTemplate) *v1.ServerTemplate {
	if t == nil {
		return nil
	}
	return &v1.ServerTemplate{
		Id:               t.ID,
		Name:             t.Name,
		Description:      t.Description,
		ModLoader:        dbModLoaderToProto(storage.ModLoader(t.ModLoader)),
		McVersion:        t.MCVersion,
		Memory:           int32(t.Memory),
		MaxPlayers:       int32(t.MaxPlayers),
		JavaVersion:      t.JavaVersion,
		DockerImage:      t.DockerImage,
		AdditionalPorts:  t.AdditionalPorts,
		DockerOverrides:  t.DockerOverrides,
		ConfigJson:       t.ConfigJSON,
		HasMods:          t.HasMods,
		ModsSizeBytes:    t.ModsSizeBytes,
		SourceServerName: t.SourceServerName,
		CreatedAt:        timestamppb.New(t.CreatedAt),
		UpdatedAt:        timestamppb.New(t.UpdatedAt),
	}
}

// ListServerTemplates lists all server templates
func (s *TemplateService) ListServerTemplates(ctx context.Context, req *connect.Request[v1.ListServerTemplatesRequest]) (*connect.Response[v1.ListServerTemplatesResponse], error) {
	templates, err := s.store.ListServerTemplates(ctx)
	if err != nil {
		s.log.Error("Failed to list server templates: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list server templates"))
	}

	protos := make([]*v1.ServerTemplate, len(templates))
	for i, t := range templates {
		protos[i] = dbTemplateToProto(t)
	}

	return connect.NewResponse(&v1.ListServerTemplatesResponse{
		Templates: protos,
	}), nil
}

// CreateServerTemplate captures a template from an existing server
// (config + optional mods).
func (s *TemplateService) CreateServerTemplate(ctx context.Context, req *connect.Request[v1.CreateServerTemplateRequest]) (*connect.Response[v1.CreateServerTemplateResponse], error) {
	msg := req.Msg

	if msg.ServerId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server_id is required"))
	}
	if msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	template := &storage.ServerTemplate{
		ID:               uuid.New().String(),
		Name:             msg.Name,
		Description:      msg.Description,
		ModLoader:        string(server.ModLoader),
		MCVersion:        server.MCVersion,
		Memory:           server.Memory,
		MaxPlayers:       server.MaxPlayers,
		JavaVersion:      server.JavaVersion,
		DockerImage:      server.DockerImage,
		AdditionalPorts:  server.AdditionalPorts,
		DockerOverrides:  server.DockerOverrides,
		SourceServerName: server.Name,
	}

	// Snapshot the server's config (JVM flags, env vars, etc.) as JSON
	serverConfig, err := s.store.GetServerConfig(ctx, server.ID)
	if err != nil {
		s.log.Warn("Failed to load config for server %s, template will use defaults: %v", server.ID, err)
	} else {
		configJSON, err := json.Marshal(serverConfig)
		if err != nil {
			s.log.Error("Failed to serialize server config: %v", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to serialize server config"))
		}
		template.ConfigJSON = string(configJSON)
	}

	// Optionally capture the server's mods directory
	if msg.IncludeMods {
		if err := s.captureMods(server, template); err != nil {
			s.log.Error("Failed to capture mods for template %s: %v", template.ID, err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to capture server mods"))
		}
	}

	if err := s.store.CreateServerTemplate(ctx, template); err != nil {
		s.log.Error("Failed to create server template: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create server template"))
	}

	s.log.Info("Created server template %s (%s) from server %s (mods: %v)",
		template.Name, template.ID, server.Name, template.HasMods)

	return connect.NewResponse(&v1.CreateServerTemplateResponse{
		Template: dbTemplateToProto(template),
	}), nil
}

// captureMods copies the source server's mods directory into template
// storage and updates the template's mods metadata. Missing/empty mods
// directories are skipped (HasMods stays false).
func (s *TemplateService) captureMods(server *storage.Server, template *storage.ServerTemplate) error {
	srcDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if srcDir == "" {
		// Loader has no dedicated mods directory; fall back to the
		// conventional layout so captured mods are not silently dropped.
		srcDir = filepath.Join(server.DataPath, "mods")
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil || len(entries) == 0 {
		// Missing or empty: nothing to capture
		template.HasMods = false
		template.ModsSizeBytes = 0
		return nil
	}

	dstDir := s.templateModsDir(template.ID)
	// Start from a clean slate so re-captures never mix old and new files
	if err := os.RemoveAll(filepath.Dir(dstDir)); err != nil {
		return fmt.Errorf("failed to clean template directory: %w", err)
	}

	start := time.Now()
	if err := files.CopyDir(srcDir, dstDir); err != nil {
		return fmt.Errorf("failed to copy mods: %w", err)
	}

	size, err := files.CalculateDirSize(dstDir)
	if err != nil {
		return fmt.Errorf("failed to size captured mods: %w", err)
	}

	template.HasMods = true
	template.ModsSizeBytes = size

	s.log.Info("Captured %d mod files (%d bytes) from %s into template %s in %s",
		len(entries), size, server.Name, template.ID, time.Since(start).Round(time.Millisecond))
	return nil
}

// UpdateServerTemplate updates a template's metadata (name/description only)
func (s *TemplateService) UpdateServerTemplate(ctx context.Context, req *connect.Request[v1.UpdateServerTemplateRequest]) (*connect.Response[v1.UpdateServerTemplateResponse], error) {
	template, err := s.store.GetServerTemplate(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server template not found"))
	}

	if req.Msg.Name != nil && *req.Msg.Name != "" {
		template.Name = *req.Msg.Name
	}
	if req.Msg.Description != nil && *req.Msg.Description != "" {
		template.Description = *req.Msg.Description
	}

	if err := s.store.UpdateServerTemplate(ctx, template); err != nil {
		s.log.Error("Failed to update server template: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update server template"))
	}

	return connect.NewResponse(&v1.UpdateServerTemplateResponse{
		Template: dbTemplateToProto(template),
	}), nil
}

// DeleteServerTemplate removes a template row and its captured mods
func (s *TemplateService) DeleteServerTemplate(ctx context.Context, req *connect.Request[v1.DeleteServerTemplateRequest]) (*connect.Response[v1.DeleteServerTemplateResponse], error) {
	template, err := s.store.GetServerTemplate(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server template not found"))
	}

	if err := s.store.DeleteServerTemplate(ctx, template.ID); err != nil {
		s.log.Error("Failed to delete server template: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete server template"))
	}

	// Remove the template's captured files (best effort)
	templateRoot := filepath.Dir(s.templateModsDir(template.ID))
	if err := os.RemoveAll(templateRoot); err != nil {
		s.log.Error("Failed to remove template directory %s: %v", templateRoot, err)
	}

	return connect.NewResponse(&v1.DeleteServerTemplateResponse{}), nil
}

// DeployServerTemplate creates a new server from a template, reusing the
// CreateServer internals so ports/hostname/proxy wiring behave identically.
func (s *TemplateService) DeployServerTemplate(ctx context.Context, req *connect.Request[v1.DeployServerTemplateRequest]) (*connect.Response[v1.DeployServerTemplateResponse], error) {
	template, err := s.store.GetServerTemplate(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server template not found"))
	}

	name := req.Msg.Name
	if name == "" {
		name = template.Name
	}

	protoTemplate := dbTemplateToProto(template)

	// For non-proxy servers allocate the next available host port; proxied
	// servers get their port from the listener inside CreateServer.
	port := int32(0)
	if req.Msg.ProxyHostname == "" {
		var err error
		port, _, err = s.servers.nextAvailablePort(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
	}

	createReq := &v1.CreateServerRequest{
		Name:             name,
		Description:      template.Description,
		ModLoader:        protoTemplate.ModLoader,
		McVersion:        template.MCVersion,
		Port:             port,
		ProxyHostname:    req.Msg.ProxyHostname,
		MaxPlayers:       int32(template.MaxPlayers),
		Memory:           int32(template.Memory),
		DockerImage:      template.DockerImage,
		AutoStart:        req.Msg.AutoStart,
		StartImmediately: req.Msg.AutoStart,
		AdditionalPorts:  template.AdditionalPorts,
		DockerOverrides:  template.DockerOverrides,
	}

	// postCreate runs synchronously before the container exists: apply the
	// captured config and copy the captured mods into the new server's data
	// directory so an auto-started container sees them immediately.
	postCreate := func(server *storage.Server) error {
		if template.ConfigJSON != "" {
			var captured storage.ServerConfig
			if err := json.Unmarshal([]byte(template.ConfigJSON), &captured); err != nil {
				return fmt.Errorf("failed to deserialize template config: %w", err)
			}
			captured.ID = server.ID + "-config"
			captured.ServerID = server.ID
			// Keep system fields in sync with the NEW server, not the source
			typeStr := string(server.ModLoader)
			versionStr := server.MCVersion
			portInt := server.Port
			maxPlayersInt := server.MaxPlayers
			captured.Type = &typeStr
			captured.Version = &versionStr
			captured.ServerPort = &portInt
			captured.MaxPlayers = &maxPlayersInt
			if err := s.store.SaveServerConfig(ctx, &captured); err != nil {
				return fmt.Errorf("failed to apply template config: %w", err)
			}
		}

		if template.HasMods {
			start := time.Now()
			dstDir := minecraft.GetModsPath(server.DataPath, storage.ModLoader(template.ModLoader))
			if dstDir == "" {
				dstDir = filepath.Join(server.DataPath, "mods")
			}
			if err := files.CopyDir(s.templateModsDir(template.ID), dstDir); err != nil {
				return fmt.Errorf("failed to copy template mods: %w", err)
			}
			s.log.Info("Copied template %s mods (%d bytes) into server %s in %s",
				template.ID, template.ModsSizeBytes, server.ID, time.Since(start).Round(time.Millisecond))
		}
		return nil
	}

	server, err := s.servers.createServerInternal(ctx, createReq, postCreate)
	if err != nil {
		return nil, err
	}

	s.log.Info("Deployed server %s (%s) from template %s", server.Name, server.ID, template.ID)

	return connect.NewResponse(&v1.DeployServerTemplateResponse{
		Server: dbServerToProto(server),
	}), nil
}
