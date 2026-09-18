package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"connectrpc.com/connect"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/pkg/files"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// backupTimestampPattern matches the "_20060102-150405" suffix appended by the
// backup task when naming archives.
var backupTimestampPattern = regexp.MustCompile(`_(\d{8}-\d{6})$`)

// serverBackupDir returns the backup directory holding a server's archives.
// Mirrors the layout written by the scheduler's backup task.
func (s *TaskService) serverBackupDir(serverDataPath string) (string, error) {
	if s.config == nil || s.config.Storage.BackupDir == "" {
		return "", fmt.Errorf("backup directory is not configured")
	}
	if serverDataPath == "" {
		return "", fmt.Errorf("server has no data directory")
	}
	return filepath.Join(s.config.Storage.BackupDir, filepath.Base(serverDataPath)), nil
}

// validateBackupFilename rejects path separators and traversal in a
// user-supplied archive filename.
func validateBackupFilename(filename string) error {
	if filename == "" || filepath.Base(filename) != filename {
		return fmt.Errorf("invalid backup filename")
	}
	if !strings.HasSuffix(filename, ".zip") {
		return fmt.Errorf("backup filename must end with .zip")
	}
	return nil
}

// ListServerBackups lists the backup archives stored for a server.
func (s *TaskService) ListServerBackups(ctx context.Context, req *connect.Request[v1.ListServerBackupsRequest]) (*connect.Response[v1.ListServerBackupsResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	backupDir, err := s.serverBackupDir(server.DataPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return connect.NewResponse(&v1.ListServerBackupsResponse{}), nil
		}
		s.log.Error("Failed to read backup directory for %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read backup directory"))
	}

	backups := make([]*v1.ServerBackup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		base := strings.TrimSuffix(entry.Name(), ".zip")
		name := base
		if loc := backupTimestampPattern.FindStringSubmatchIndex(base); loc != nil {
			name = base[:loc[0]]
		}

		backups = append(backups, &v1.ServerBackup{
			ServerId:  server.ID,
			Filename:  entry.Name(),
			Name:      name,
			SizeBytes: info.Size(),
			CreatedAt: timestamppb.New(info.ModTime()),
		})
	}

	// Newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.AsTime().After(backups[j].CreatedAt.AsTime())
	})

	return connect.NewResponse(&v1.ListServerBackupsResponse{Backups: backups}), nil
}

// RestoreServerBackup extracts a backup archive over the server's data
// directory. The server must be stopped so live files aren't clobbered.
func (s *TaskService) RestoreServerBackup(ctx context.Context, req *connect.Request[v1.RestoreServerBackupRequest]) (*connect.Response[v1.RestoreServerBackupResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	if err := validateBackupFilename(req.Msg.Filename); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	backupDir, err := s.serverBackupDir(server.DataPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	archivePath := filepath.Join(backupDir, req.Msg.Filename)
	if _, err := os.Stat(archivePath); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backup archive not found"))
	}

	if server.ContainerID != "" {
		if status, err := s.docker.GetContainerStatus(ctx, server.ContainerID); err == nil && status == storage.StatusRunning {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("stop the server before restoring a backup"))
		}
	}

	s.log.Info("Restoring backup %s for server %s", req.Msg.Filename, server.Name)
	if _, err := files.ExtractArchive(ctx, archivePath, server.DataPath, nil); err != nil {
		s.log.Error("Failed to restore backup for %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restore backup"))
	}

	return connect.NewResponse(&v1.RestoreServerBackupResponse{Status: "restored"}), nil
}

// DeleteServerBackup removes a backup archive from disk.
func (s *TaskService) DeleteServerBackup(ctx context.Context, req *connect.Request[v1.DeleteServerBackupRequest]) (*connect.Response[v1.DeleteServerBackupResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	if err := validateBackupFilename(req.Msg.Filename); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	backupDir, err := s.serverBackupDir(server.DataPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if err := os.Remove(filepath.Join(backupDir, req.Msg.Filename)); err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backup archive not found"))
		}
		s.log.Error("Failed to delete backup %s for %s: %v", req.Msg.Filename, server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete backup"))
	}

	return connect.NewResponse(&v1.DeleteServerBackupResponse{}), nil
}
