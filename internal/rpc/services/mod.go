package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/internal/minecraft"
	"github.com/nickheyer/discopanel/pkg/files"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"github.com/nickheyer/discopanel/pkg/upload"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that ModService implements the interface
var _ discopanelv1connect.ModServiceHandler = (*ModService)(nil)

// ModService implements the Mod service
type ModService struct {
	store         *storage.Store
	docker        *docker.Client
	log           *logger.Logger
	uploadManager *upload.Manager
}

// NewModService creates a new mod service
func NewModService(store *storage.Store, docker *docker.Client, uploadManager *upload.Manager, log *logger.Logger) *ModService {
	return &ModService{
		store:         store,
		docker:        docker,
		log:           log,
		uploadManager: uploadManager,
	}
}

// ListMods lists mods for a server
func (s *ModService) ListMods(ctx context.Context, req *connect.Request[v1.ListModsRequest]) (*connect.Response[v1.ListModsResponse], error) {
	msg := req.Msg

	// Get server to find data path and mod loader
	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	// Get the mods directory path
	modsDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if modsDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this server type does not support mods"))
	}

	// Check if mods directory exists
	mods := []*v1.Mod{}

	// Read mods from active directory
	if files, err := os.ReadDir(modsDir); err == nil {
		for _, file := range files {
			if file.IsDir() {
				continue
			}

			// Check if this is a valid mod file
			if !minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			mod := s.buildModFromFile(msg.ServerId, modsDir, file.Name(), true, info)
			mods = append(mods, mod)
		}
	}

	// Also check disabled mods directory
	disabledDir := modsDir + "_disabled"
	if files, err := os.ReadDir(disabledDir); err == nil {
		for _, file := range files {
			if file.IsDir() {
				continue
			}

			if !minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			mod := s.buildModFromFile(msg.ServerId, disabledDir, file.Name(), false, info)
			mods = append(mods, mod)
		}
	}

	return connect.NewResponse(&v1.ListModsResponse{
		Mods: mods,
	}), nil
}

// GetMod gets a specific mod
func (s *ModService) GetMod(ctx context.Context, req *connect.Request[v1.GetModRequest]) (*connect.Response[v1.GetModResponse], error) {
	msg := req.Msg

	// Get server to validate and find mod
	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	// Get the mods directory path
	modsDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if modsDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this server type does not support mods"))
	}

	// Try to find the mod file in active directory
	if files, err := os.ReadDir(modsDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
				if fileID == msg.ModId {
					info, _ := file.Info()
					mod := s.buildModFromFile(msg.ServerId, modsDir, file.Name(), true, info)
					return connect.NewResponse(&v1.GetModResponse{Mod: mod}), nil
				}
			}
		}
	}

	// Try disabled directory
	disabledDir := modsDir + "_disabled"
	if files, err := os.ReadDir(disabledDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
				if fileID == msg.ModId {
					info, _ := file.Info()
					mod := s.buildModFromFile(msg.ServerId, disabledDir, file.Name(), false, info)
					return connect.NewResponse(&v1.GetModResponse{Mod: mod}), nil
				}
			}
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, errors.New("mod not found"))
}

// ImportUploadedMod imports a mod
func (s *ModService) ImportUploadedMod(ctx context.Context, req *connect.Request[v1.ImportUploadedModRequest]) (*connect.Response[v1.ImportUploadedModResponse], error) {
	msg := req.Msg

	// Validate upload session
	if msg.UploadSessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("upload_session_id is required"))
	}

	// Get server to find data path and mod loader
	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	// Get temp file path and original filename from upload manager
	tempPath, originalFilename, err := s.uploadManager.GetTempPath(msg.UploadSessionId)
	if err != nil {
		s.log.Error("Failed to get upload session: %v", err)
		return nil, connect.NewError(connect.CodeNotFound, errors.New("upload session not found or not completed"))
	}

	// Validate file is appropriate for this mod loader
	if !minecraft.IsValidModFile(originalFilename, server.ModLoader) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid file type for this mod loader"))
	}

	// Get the correct mods directory based on mod loader
	modsDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if modsDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this server type does not support mods"))
	}

	// Create mods directory if needed
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		s.log.Error("Failed to create mods directory: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create mods directory"))
	}

	// Move file from temp location to mods dir
	modPath := filepath.Join(modsDir, originalFilename)
	if err := os.Rename(tempPath, modPath); err != nil {
		if err := files.CopyFile(tempPath, modPath); err != nil {
			s.log.Error("Failed to move mod file: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save mod"))
		}
		os.Remove(tempPath)
	}

	// Cleanup the upload session
	s.uploadManager.CleanupSession(msg.UploadSessionId)

	// Get file info for the response
	info, err := os.Stat(modPath)
	if err != nil {
		s.log.Error("Failed to stat mod file: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get mod info"))
	}

	// Create mod record
	mod := s.buildModFromFile(msg.ServerId, modsDir, originalFilename, true, info)
	if msg.DisplayName != "" {
		mod.DisplayName = msg.DisplayName
	}
	if msg.Description != "" {
		mod.Description = msg.Description
	}

	return connect.NewResponse(&v1.ImportUploadedModResponse{
		Mod:     mod,
		Message: "Mod uploaded successfully",
	}), nil
}

// UpdateMod updates a mod
func (s *ModService) UpdateMod(ctx context.Context, req *connect.Request[v1.UpdateModRequest]) (*connect.Response[v1.UpdateModResponse], error) {
	msg := req.Msg

	// Get server to find mod path
	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	modsDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if modsDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this server type does not support mods"))
	}

	disabledDir := modsDir + "_disabled"

	// Find the mod file
	var modFileName string
	var currentlyEnabled bool
	var modInfo os.FileInfo

	// First, scan the mods directory
	if files, err := os.ReadDir(modsDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				// Generate the same ID as in ListMods to match
				fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
				if fileID == msg.ModId {
					modFileName = file.Name()
					currentlyEnabled = true
					modInfo, _ = file.Info()
					break
				}
			}
		}
	}

	// If not found, check disabled directory
	if modFileName == "" {
		if files, err := os.ReadDir(disabledDir); err == nil {
			for _, file := range files {
				if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
					fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
					if fileID == msg.ModId {
						modFileName = file.Name()
						currentlyEnabled = false
						modInfo, _ = file.Info()
						break
					}
				}
			}
		}
	}

	if modFileName == "" {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("mod not found"))
	}

	// Handle enabling/disabling
	finalEnabled := currentlyEnabled
	if msg.Enabled != nil && *msg.Enabled != currentlyEnabled {
		if *msg.Enabled {
			// Move from disabled to mods directory
			oldPath := filepath.Join(disabledDir, modFileName)
			newPath := filepath.Join(modsDir, modFileName)
			if err := os.Rename(oldPath, newPath); err != nil {
				s.log.Error("Failed to enable mod: %v", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enable mod"))
			}
			finalEnabled = true
		} else {
			// Move from mods to disabled directory
			os.MkdirAll(disabledDir, 0755)
			oldPath := filepath.Join(modsDir, modFileName)
			newPath := filepath.Join(disabledDir, modFileName)
			if err := os.Rename(oldPath, newPath); err != nil {
				s.log.Error("Failed to disable mod: %v", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to disable mod"))
			}
			finalEnabled = false
		}
	}

	targetDir := modsDir
	if !finalEnabled {
		targetDir = disabledDir
	}

	// Build response with metadata & icon
	mod := s.buildModFromFile(msg.ServerId, targetDir, modFileName, finalEnabled, modInfo)
	mod.UpdatedAt = timestamppb.Now()

	// Use provided display name if given
	if msg.DisplayName != nil && *msg.DisplayName != "" {
		mod.DisplayName = *msg.DisplayName
	}

	if msg.Description != nil {
		mod.Description = *msg.Description
	}

	return connect.NewResponse(&v1.UpdateModResponse{
		Mod: mod,
	}), nil
}

// DeleteMod deletes a mod
func (s *ModService) DeleteMod(ctx context.Context, req *connect.Request[v1.DeleteModRequest]) (*connect.Response[v1.DeleteModResponse], error) {
	msg := req.Msg

	// Get server to find file path
	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	modsDir := minecraft.GetModsPath(server.DataPath, server.ModLoader)
	if modsDir == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this server type does not support mods"))
	}

	// Try to find and delete the mod file
	deleted := false

	// Check active directory
	if files, err := os.ReadDir(modsDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
				fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
				if fileID == msg.ModId {
					modPath := filepath.Join(modsDir, file.Name())
					if err := os.Remove(modPath); err != nil {
						s.log.Error("Failed to delete mod file: %v", err)
						return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete mod file"))
					}
					deleted = true
					break
				}
			}
		}
	}

	// Check disabled directory
	if !deleted {
		disabledDir := modsDir + "_disabled"
		if files, err := os.ReadDir(disabledDir); err == nil {
			for _, file := range files {
				if !file.IsDir() && minecraft.IsValidModFile(file.Name(), server.ModLoader) {
					fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ServerId+file.Name())).String()
					if fileID == msg.ModId {
						modPath := filepath.Join(disabledDir, file.Name())
						if err := os.Remove(modPath); err != nil {
							s.log.Error("Failed to delete mod file: %v", err)
							return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete mod file"))
						}
						deleted = true
						break
					}
				}
			}
		}
	}

	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("mod not found"))
	}

	return connect.NewResponse(&v1.DeleteModResponse{
		Message: "Mod deleted successfully",
	}), nil
}

// buildModFromFile constructs a Mod protobuf object with extracted metadata and icon
func (s *ModService) buildModFromFile(serverId string, dir string, fileName string, enabled bool, info os.FileInfo) *v1.Mod {
	filePath := filepath.Join(dir, fileName)
	displayName := fileName
	if ext := filepath.Ext(displayName); ext != "" {
		displayName = displayName[:len(displayName)-len(ext)]
	}

	var fileSize int64
	var modTime timestamppb.Timestamp
	if info != nil {
		fileSize = info.Size()
		modTime = *timestamppb.New(info.ModTime())
	}

	mod := &v1.Mod{
		Id:          uuid.NewSHA1(uuid.NameSpaceURL, []byte(serverId+fileName)).String(),
		ServerId:    serverId,
		FileName:    fileName,
		DisplayName: displayName,
		Enabled:     enabled,
		FileSize:    fileSize,
		UploadedAt:  &modTime,
	}

	if meta, err := minecraft.ExtractModMetadata(filePath); err == nil && meta != nil {
		if meta.DisplayName != "" {
			mod.DisplayName = meta.DisplayName
		}
		if meta.Version != "" {
			mod.Version = meta.Version
		}
		if meta.Description != "" {
			mod.Description = meta.Description
		}
		if meta.Author != "" {
			mod.Author = meta.Author
		}
		if meta.Website != "" {
			mod.Website = meta.Website
		}
		if meta.ModID != "" {
			mod.ModId = meta.ModID
		}
		if meta.IconDataURL != "" {
			mod.IconUrl = meta.IconDataURL
		}
	}

	return mod
}
