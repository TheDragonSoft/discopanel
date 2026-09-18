package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/nickheyer/discopanel/internal/docker"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/minecraft"
	"github.com/nickheyer/discopanel/pkg/files"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

// ImportServer imports an existing server (e.g. migrated from another panel
// such as Crafty) from a ZIP archive of its server directory. The archive is
// uploaded through the chunked upload endpoint and referenced by session ID.
// The imported server has no container yet; it is created on first start.
func (s *ServerService) ImportServer(ctx context.Context, req *connect.Request[v1.ImportServerRequest]) (*connect.Response[v1.ImportServerResponse], error) {
	msg := req.Msg

	if msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	if msg.UploadSessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("upload_session_id is required"))
	}

	tempPath, originalFilename, err := s.uploadManager.GetTempPath(msg.UploadSessionId)
	if err != nil {
		s.log.Error("Failed to get upload session %s: %v", msg.UploadSessionId, err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("upload session not found or not completed"))
	}

	cleanup := func() {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			s.log.Error("Failed to remove temp file %s: %v", tempPath, err)
		}
		s.uploadManager.Cancel(msg.UploadSessionId)
	}

	if !strings.HasSuffix(strings.ToLower(originalFilename), ".zip") {
		cleanup()
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("server import requires a ZIP archive"))
	}

	memory := int(msg.Memory)
	if memory == 0 {
		memory = 4096
	}
	maxPlayers := int(msg.MaxPlayers)
	if maxPlayers == 0 {
		maxPlayers = 20
	}

	// Global memory quota admission control
	if err := s.checkMemoryQuota(ctx, memory, ""); err != nil {
		cleanup()
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	// Resolve optional metadata
	modLoader := storage.ModLoaderVanilla
	if msg.ModLoader != "" {
		modLoader = storage.ModLoader(msg.ModLoader)
	}
	mcVersion := msg.McVersion
	if mcVersion == "" {
		mcVersion = minecraft.GetLatestVersion()
	}

	// Allocate the next free port
	portResp, err := s.GetNextAvailablePort(ctx, connect.NewRequest(&v1.GetNextAvailablePortRequest{}))
	if err != nil {
		cleanup()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to allocate port"))
	}
	port := int(portResp.Msg.GetPort())

	// Create the server record
	serverUUID := uuid.New().String()
	serverDataDir := fmt.Sprintf("%s_%s", files.SanitizePathName(msg.Name), serverUUID)
	serverDataPath := filepath.Join(s.config.Storage.DataDir, "servers", serverDataDir)

	server := &storage.Server{
		ID:          serverUUID,
		Name:        msg.Name,
		Description: msg.Description,
		ModLoader:   modLoader,
		MCVersion:   mcVersion,
		Status:      storage.StatusStopped,
		Port:        port,
		MaxPlayers:  maxPlayers,
		Memory:      memory,
		DataPath:    serverDataPath,
		JavaVersion: docker.GetRequiredJavaVersion(mcVersion, modLoader),
		DockerImage: docker.GetOptimalDockerTag(mcVersion, modLoader, false),
		TPSCommand:  minecraft.GetTPSCommand(modLoader),
	}

	if err := os.MkdirAll(server.DataPath, 0755); err != nil {
		s.log.Error("Failed to create data directory: %v", err)
		cleanup()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create server directory"))
	}

	if err := s.store.CreateServer(ctx, server); err != nil {
		s.log.Error("Failed to save imported server: %v", err)
		cleanup()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create server"))
	}

	// Extract the archive into the server's data directory
	count, err := files.ExtractArchive(ctx, tempPath, server.DataPath, nil)
	if err != nil {
		s.log.Error("Failed to extract server archive for %s: %v", server.Name, err)
		cleanup()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to extract server archive: %w", err))
	}

	// Seed the server config with memory settings derived from the allocation
	serverConfig, err := s.store.GetServerConfig(ctx, server.ID)
	if err != nil {
		serverConfig = s.store.CreateDefaultServerConfig(server.ID)
	}
	strMax := fmt.Sprintf("%dM", int(float64(memory)*0.75))
	strMin := fmt.Sprintf("%dM", int(float64(memory)*0.45))
	serverConfig.MaxMemory = &strMax
	serverConfig.InitMemory = &strMin
	if err := s.store.UpdateServerConfig(ctx, serverConfig); err != nil {
		s.log.Error("Failed to update server config: %v", err)
	}

	// The temp archive is fully consumed now
	cleanup()

	s.log.Info("Imported server %s from %s (%d files extracted)", server.Name, originalFilename, count)
	return connect.NewResponse(&v1.ImportServerResponse{
		Server:        dbServerToProto(server),
		ImportedFiles: fmt.Sprintf("%d", count),
	}), nil
}
