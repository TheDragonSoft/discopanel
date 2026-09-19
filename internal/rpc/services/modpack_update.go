package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/nickheyer/discopanel/internal/config"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/internal/events"
	"github.com/nickheyer/discopanel/internal/indexers"
	_ "github.com/nickheyer/discopanel/internal/indexers/fuego"
	_ "github.com/nickheyer/discopanel/internal/indexers/modrinth"
	"github.com/nickheyer/discopanel/internal/scheduler"
	"github.com/nickheyer/discopanel/internal/watchdog"
	"github.com/nickheyer/discopanel/pkg/files"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that ModpackUpdateService implements the interface
var _ discopanelv1connect.ModpackUpdateServiceHandler = (*ModpackUpdateService)(nil)

const (
	// Modpack update modes persisted in ModpackUpdateSetting.Mode.
	ModpackUpdateModeNotify = "notify"
	ModpackUpdateModeApply  = "apply"

	// DefaultModpackUpdateIntervalHours is the default period between
	// scheduled update checks for a server.
	DefaultModpackUpdateIntervalHours = 24

	// modpackUpdateCheckInterval is how often the background loop scans the
	// enabled settings and runs any checks that have come due.
	modpackUpdateCheckInterval = 15 * time.Minute

	// prefix used for the pre-update backup archives
	modpackUpdateBackupPrefix = "pre-modpack-update"
)

// ModpackUpdateService implements the ModpackUpdate service: in-place
// modpack updates that preserve the world, pre-update backup + rollback, and
// scheduled update checks.
//
// HOW UPDATES WORK IN DISCOMPANEL: a server's modpack lives in its container
// environment (ServerConfig env fields: CFPageURL / CFModpackZip for
// CurseForge, MODRINTH_MODPACK for Modrinth). The itzg-style container image
// downloads and applies the modpack server-pack on startup and never deletes
// the world directory, so "applying an update" means pinning the new version
// in the server config and recreating the container - the data directory
// (and therefore the world) is never touched by this pipeline. A pre-update
// backup of the world directories is taken first so the user can roll back.
type ModpackUpdateService struct {
	store     *storage.Store
	config    *config.Config
	docker    *docker.Client
	scheduler *scheduler.Scheduler
	bus       *events.Bus
	log       *logger.Logger
	watchdog  *watchdog.Watchdog

	// Guards against concurrent checks/updates of the same server.
	mu       sync.Mutex
	inFlight map[string]struct{}

	// Background scheduled-check loop state.
	loopMu   sync.Mutex
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewModpackUpdateService creates a new modpack update service
func NewModpackUpdateService(store *storage.Store, cfg *config.Config, dockerClient *docker.Client, sched *scheduler.Scheduler, bus *events.Bus, log *logger.Logger) *ModpackUpdateService {
	return &ModpackUpdateService{
		store:     store,
		config:    cfg,
		docker:    dockerClient,
		scheduler: sched,
		bus:       bus,
		log:       log,
		inFlight:  make(map[string]struct{}),
		stopChan:  make(chan struct{}),
	}
}

// SetWatchdog wires the crash watchdog so container stops performed by the
// update pipeline are not counted as crashes.
func (s *ModpackUpdateService) SetWatchdog(w *watchdog.Watchdog) {
	s.watchdog = w
}

// markIntentionalStop tells the watchdog an upcoming container exit for this
// server was requested by the panel.
func (s *ModpackUpdateService) markIntentionalStop(serverID string) {
	if s.watchdog != nil {
		s.watchdog.MarkIntentionalStop(serverID)
	}
}

// serverModpackRef is a server's resolved modpack association.
type serverModpackRef struct {
	Modpack          *storage.IndexedModpack
	CurrentVersionID string // "" = tracking latest (unpinned)
	Manual           bool   // manual uploaded modpack zip: single version
}

// parseCFPageURL splits a CurseForge modpack page URL into the modpack base
// URL and the pinned file ID ("" when the URL is not version-pinned).
func parseCFPageURL(url string) (base, fileID string) {
	base = url
	if i := strings.LastIndex(url, "/files/"); i >= 0 {
		base = url[:i]
		fileID = url[i+len("/files/"):]
	}
	return base, fileID
}

// getIndexer creates an indexer by name, looking up the fuego API key from
// settings when needed (mirrors ModpackService.getIndexer).
func (s *ModpackUpdateService) getIndexer(ctx context.Context, name string) (indexers.ModpackIndexer, error) {
	apiKey := ""
	if name == "fuego" {
		globalSettings, _, err := s.store.GetGlobalSettings(ctx)
		if err != nil || globalSettings == nil {
			return nil, fmt.Errorf("failed to get global settings")
		}
		if globalSettings.CFAPIKey != nil {
			apiKey = *globalSettings.CFAPIKey
		}
		if apiKey == "" {
			return nil, fmt.Errorf("CurseForge API key not configured")
		}
	}
	return indexers.NewIndexer(name, apiKey, s.config)
}

// resolveServerModpack determines which modpack (and pinned version, if any)
// a server is running, derived from the same ServerConfig fields the modpack
// install/creation path writes (CFPageURL / CFModpackZip / MODRINTH_MODPACK).
func (s *ModpackUpdateService) resolveServerModpack(ctx context.Context, serverID string) (*storage.Server, *serverModpackRef, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	serverConfig, err := s.store.GetServerConfig(ctx, serverID)
	if err != nil {
		return server, nil, fmt.Errorf("failed to load server configuration")
	}

	switch server.ModLoader {
	case storage.ModLoaderAutoCurseForge:
		// Manual uploaded modpacks are identified by the "manual-" slug
		// prefix set at server creation.
		if serverConfig.CFModpackZip != nil && *serverConfig.CFModpackZip != "" {
			modpackID := ""
			if serverConfig.CFSlug != nil {
				modpackID = strings.TrimPrefix(*serverConfig.CFSlug, "manual-")
			}
			if modpackID != "" {
				modpack, err := s.store.GetIndexedModpack(ctx, modpackID)
				if err != nil {
					return server, nil, fmt.Errorf("uploaded modpack not found")
				}
				return server, &serverModpackRef{Modpack: modpack, Manual: true}, nil
			}
			return server, nil, fmt.Errorf("uploaded modpack not found")
		}

		// CurseForge servers pin a version by appending /files/<fileID> to
		// the modpack page URL.
		if serverConfig.CFPageURL != nil && *serverConfig.CFPageURL != "" {
			base, currentVersionID := parseCFPageURL(*serverConfig.CFPageURL)
			modpack, err := s.store.GetModpackByWebsiteURL(ctx, strings.TrimSuffix(base, "/"))
			if err != nil || modpack == nil {
				// Fall back to the slug captured at creation time
				if serverConfig.CFSlug != nil && *serverConfig.CFSlug != "" {
					if bySlug, slugErr := s.store.GetModpackBySlug(ctx, *serverConfig.CFSlug); slugErr == nil && bySlug != nil {
						return server, &serverModpackRef{Modpack: bySlug, CurrentVersionID: currentVersionID}, nil
					}
				}
				return server, nil, fmt.Errorf("modpack not found in the index (sync it and try again)")
			}
			return server, &serverModpackRef{Modpack: modpack, CurrentVersionID: currentVersionID}, nil
		}
		return server, nil, fmt.Errorf("server has no modpack configured")

	case storage.ModLoaderModrinth:
		// Modrinth servers store "projectID" or "projectID:versionID".
		if serverConfig.ModrinthModpack != nil && *serverConfig.ModrinthModpack != "" {
			spec := *serverConfig.ModrinthModpack
			projectID, currentVersionID := spec, ""
			if i := strings.Index(spec, ":"); i >= 0 {
				projectID = spec[:i]
				currentVersionID = spec[i+1:]
			}
			modpack, err := s.store.GetIndexedModpackByIndexerID(ctx, "modrinth", projectID)
			if err != nil {
				// The stored spec may be a slug, or the modpack may have been
				// indexed under the composite "modrinth-<id>" key.
				if bySlug, slugErr := s.store.GetModpackBySlug(ctx, projectID); slugErr == nil && bySlug != nil {
					return server, &serverModpackRef{Modpack: bySlug, CurrentVersionID: currentVersionID}, nil
				}
				if byID, idErr := s.store.GetIndexedModpack(ctx, "modrinth-"+projectID); idErr == nil && byID != nil {
					return server, &serverModpackRef{Modpack: byID, CurrentVersionID: currentVersionID}, nil
				}
				return server, nil, fmt.Errorf("modpack not found in the index (sync it and try again)")
			}
			return server, &serverModpackRef{Modpack: modpack, CurrentVersionID: currentVersionID}, nil
		}
		return server, nil, fmt.Errorf("server has no modpack configured")
	}

	return server, nil, fmt.Errorf("server does not use a modpack-managed mod loader")
}

// getModpackVersions fetches the modpack's versions from its indexer, sorted
// newest first (SortIndex ascending, as produced by both indexers).
func (s *ModpackUpdateService) getModpackVersions(ctx context.Context, modpack *storage.IndexedModpack) ([]indexers.ModpackFile, error) {
	if modpack.Indexer == "manual" {
		return nil, fmt.Errorf("manual modpacks have no remote versions")
	}
	indexerClient, err := s.getIndexer(ctx, modpack.Indexer)
	if err != nil {
		return nil, err
	}
	filesList, err := indexerClient.GetModpackFiles(ctx, modpack.IndexerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(filesList, func(i, j int) bool {
		return filesList[i].SortIndex < filesList[j].SortIndex
	})
	return filesList, nil
}

// versionDisplayName returns the human-readable name for a modpack version.
func versionDisplayName(f indexers.ModpackFile) string {
	if f.DisplayName != "" {
		return f.DisplayName
	}
	if f.VersionNumber != "" {
		return f.VersionNumber
	}
	return f.FileName
}

// checkServer performs an update check for a server without touching anything.
// Returned error is an RPC-worthy failure (invalid server); every softer
// failure is reported via the check's Error field.
func (s *ModpackUpdateService) checkServer(ctx context.Context, serverID string) (*v1.ModpackUpdateCheck, error) {
	check := &v1.ModpackUpdateCheck{ServerId: serverID}

	_, ref, err := s.resolveServerModpack(ctx, serverID)
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return nil, err
		}
		check.Error = err.Error()
		return check, nil
	}
	check.ModpackName = ref.Modpack.Name
	if ref.Manual {
		check.Error = "uploaded modpacks bundle a single server-pack version and cannot be checked for updates"
		return check, nil
	}

	filesList, err := s.getModpackVersions(ctx, ref.Modpack)
	if err != nil {
		check.Error = fmt.Sprintf("failed to look up versions from %s: %v", ref.Modpack.Indexer, err)
		return check, nil
	}
	if len(filesList) == 0 {
		check.Error = "no versions found for this modpack"
		return check, nil
	}

	latest := filesList[0]
	check.LatestVersionId = latest.ID
	check.LatestVersion = versionDisplayName(latest)
	if ref.CurrentVersionID != "" {
		for _, f := range filesList {
			if f.ID == ref.CurrentVersionID {
				check.CurrentVersion = versionDisplayName(f)
				break
			}
		}
		if check.CurrentVersion == "" {
			check.CurrentVersion = ref.CurrentVersionID
		}
		check.UpdateAvailable = ref.CurrentVersionID != latest.ID
	}
	return check, nil
}

// ── RPC: CheckModpackUpdate ────────────────────────────────────────────────

// CheckModpackUpdate checks whether a newer version of a server's modpack is
// available. Lookup failures are reported in the check's Error field rather
// than as RPC errors so the UI can always render a result row.
func (s *ModpackUpdateService) CheckModpackUpdate(ctx context.Context, req *connect.Request[v1.CheckModpackUpdateRequest]) (*connect.Response[v1.CheckModpackUpdateResponse], error) {
	check, err := s.checkServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.CheckModpackUpdateResponse{Check: check}), nil
}

// ── RPC: ListModpackVersions ───────────────────────────────────────────────

// ListModpackVersions lists the versions available for a server's modpack
// with released_at timestamps and an is_current flag.
func (s *ModpackUpdateService) ListModpackVersions(ctx context.Context, req *connect.Request[v1.ListModpackVersionsRequest]) (*connect.Response[v1.ListModpackVersionsResponse], error) {
	_, ref, err := s.resolveServerModpack(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, err
	}
	if ref.Manual {
		return connect.NewResponse(&v1.ListModpackVersionsResponse{Versions: []*v1.ModpackVersionInfo{}}), nil
	}

	filesList, err := s.getModpackVersions(ctx, ref.Modpack)
	if err != nil {
		s.log.Error("Failed to list versions for server %s: %v", req.Msg.ServerId, err)
		return nil, mapIndexerError(err, "failed to list modpack versions")
	}

	versions := make([]*v1.ModpackVersionInfo, 0, len(filesList))
	for _, f := range filesList {
		versions = append(versions, &v1.ModpackVersionInfo{
			VersionId:   f.ID,
			VersionName: versionDisplayName(f),
			ReleasedAt:  timestamppb.New(f.FileDate),
			IsCurrent:   ref.CurrentVersionID != "" && f.ID == ref.CurrentVersionID,
		})
	}

	return connect.NewResponse(&v1.ListModpackVersionsResponse{Versions: versions}), nil
}

// ── RPC: UpdateServerModpack ───────────────────────────────────────────────

// UpdateServerModpack runs the update pipeline synchronously:
//  1. resolve the target version (latest when empty)
//  2. optionally take a pre-update backup (world directories)
//  3. pin the new version in the server config (the same env fields the
//     original install path uses) and recreate the container - the container
//     image then applies the new server-pack on start without touching the
//     world directory
//  4. restart the server when it was running before
//
// On failure after the backup the response carries status "failed"; the user
// rolls back explicitly via RollbackModpackUpdate.
func (s *ModpackUpdateService) UpdateServerModpack(ctx context.Context, req *connect.Request[v1.UpdateServerModpackRequest]) (*connect.Response[v1.UpdateServerModpackResponse], error) {
	if !s.begin(req.Msg.ServerId) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("an update or check is already in progress for this server"))
	}
	defer s.done(req.Msg.ServerId)

	resp, err := s.applyUpdate(ctx, req.Msg.ServerId, req.Msg.TargetVersionId, req.Msg.CreateBackup)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// applyUpdate is the pipeline shared by the RPC and the scheduled loop. A
// non-nil error means nothing was changed (invalid request / backup failure);
// a pipeline failure after the backup is reported via the response's status.
func (s *ModpackUpdateService) applyUpdate(ctx context.Context, serverID, targetVersionID string, createBackup bool) (*v1.UpdateServerModpackResponse, error) {
	resp := &v1.UpdateServerModpackResponse{}

	server, ref, err := s.resolveServerModpack(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if ref.Manual {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("uploaded modpacks cannot be updated in place"))
	}

	filesList, err := s.getModpackVersions(ctx, ref.Modpack)
	if err != nil {
		s.log.Error("Failed to resolve versions for server %s: %v", serverID, err)
		return nil, mapIndexerError(err, "failed to resolve modpack versions")
	}
	if len(filesList) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no versions found for this modpack"))
	}

	// Resolve the target version (latest when empty)
	var target indexers.ModpackFile
	if targetVersionID == "" {
		target = filesList[0]
	} else {
		found := false
		for _, f := range filesList {
			if f.ID == targetVersionID {
				target = f
				found = true
				break
			}
		}
		if !found {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target version %s not found for this modpack", targetVersionID))
		}
	}

	if ref.CurrentVersionID != "" && ref.CurrentVersionID == target.ID {
		resp.Status = "up_to_date"
		resp.Message = fmt.Sprintf("%s is already on version %s", ref.Modpack.Name, versionDisplayName(target))
		return resp, nil
	}

	// Pre-update backup (world directories, save pausing included) - taken
	// with the same machinery the scheduled backup task uses.
	backupFilename := ""
	if createBackup {
		s.log.Info("Taking pre-update backup of server %s before modpack update", server.Name)
		backupFilename, err = s.scheduler.CreateServerBackup(ctx, server, modpackUpdateBackupPrefix)
		if err != nil {
			s.log.Error("Pre-update backup failed for server %s: %v", server.Name, err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("pre-update backup failed: %w", err))
		}
		resp.BackupFilename = backupFilename
	}

	// Pin the new version using the same config fields the install path writes
	serverConfig, err := s.store.GetServerConfig(ctx, serverID)
	if err != nil {
		serverConfig = s.store.CreateDefaultServerConfig(serverID)
	}
	switch ref.Modpack.Indexer {
	case "fuego":
		versionedURL := fmt.Sprintf("%s/files/%s", strings.TrimSuffix(ref.Modpack.WebsiteURL, "/"), target.ID)
		serverConfig.CFPageURL = &versionedURL
	case "modrinth":
		projectSpec := fmt.Sprintf("%s:%s", ref.Modpack.IndexerID, target.ID)
		serverConfig.ModrinthModpack = &projectSpec
		downloadDeps := "required"
		serverConfig.ModrinthDownloadDependencies = &downloadDeps
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported modpack indexer %q", ref.Modpack.Indexer))
	}
	if err := s.store.UpdateServerConfig(ctx, serverConfig); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist new modpack version: %w", err))
	}

	updateMsg := fmt.Sprintf("updated %s to version %s (%s)", ref.Modpack.Name, target.ID, versionDisplayName(target))

	// Recreate the container so the new modpack is applied on start. The data
	// directory (and the world inside it) is untouched - only container env
	// changes - and the image syncs the modpack server-pack itself.
	if server.ContainerID != "" {
		s.markIntentionalStop(server.ID)
		result, err := s.docker.RecreateContainer(ctx, server.ContainerID, server, serverConfig)
		if err != nil {
			s.log.Error("Container recreation failed during modpack update for %s: %v", server.Name, err)
			if result != nil && result.NewContainerID != "" {
				server.ContainerID = result.NewContainerID
			}
			server.Status = storage.StatusError
			if updErr := s.store.UpdateServer(ctx, server); updErr != nil {
				s.log.Error("Failed to persist error state for %s: %v", server.Name, updErr)
			}
			resp.Status = "failed"
			resp.Message = fmt.Sprintf("modpack config updated to %s but the container could not be recreated: %v", target.ID, err)
			if recordErr := s.store.RecordModpackUpdate(ctx, serverID, backupFilename, "failed: "+resp.Message); recordErr != nil {
				s.log.Error("Failed to record failed update for %s: %v", server.Name, recordErr)
			}
			return resp, nil
		}
		server.ContainerID = result.NewContainerID
		if result.WasRunning {
			server.Status = storage.StatusRunning
		} else {
			server.Status = storage.StatusStopped
		}
		if err := s.store.UpdateServer(ctx, server); err != nil {
			s.log.Error("Failed to update server after modpack update: %v", err)
		}
	} else {
		// No container yet: the pinned version is applied on the next start.
		resp.Message = "modpack version pinned; the new version will be applied when the server starts. "
	}

	resp.Status = "updated"
	if resp.Message == "" {
		resp.Message = updateMsg
	} else {
		resp.Message += updateMsg
	}

	if recordErr := s.store.RecordModpackUpdate(ctx, serverID, backupFilename, resp.Message); recordErr != nil {
		s.log.Error("Failed to record modpack update for %s: %v", server.Name, recordErr)
	}

	s.log.Info("Modpack update complete for server %s: %s (backup: %s)", server.Name, updateMsg, backupFilename)
	return resp, nil
}

// ── RPC: RollbackModpackUpdate ─────────────────────────────────────────────

// RollbackModpackUpdate restores the pre-update backup recorded by the last
// modpack update. The server must be stopped so live files aren't clobbered.
func (s *ModpackUpdateService) RollbackModpackUpdate(ctx context.Context, req *connect.Request[v1.RollbackModpackUpdateRequest]) (*connect.Response[v1.RollbackModpackUpdateResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	setting, err := s.store.GetModpackUpdateSetting(ctx, server.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load update settings"))
	}
	if setting == nil || setting.LastBackupFilename == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no pre-update backup recorded for this server"))
	}
	filename := setting.LastBackupFilename
	if err := validateBackupFilename(filename); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	if s.config == nil || s.config.Storage.BackupDir == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("backup directory is not configured"))
	}
	archivePath := filepath.Join(s.config.Storage.BackupDir, filepath.Base(server.DataPath), filename)
	if _, err := os.Stat(archivePath); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backup archive not found"))
	}

	if server.ContainerID != "" {
		if status, err := s.docker.GetContainerStatus(ctx, server.ContainerID); err == nil && status == storage.StatusRunning {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("stop the server before rolling back its modpack update"))
		}
	}

	s.log.Info("Rolling back modpack update for server %s using backup %s", server.Name, filename)
	if _, err := files.ExtractArchive(ctx, archivePath, server.DataPath, nil); err != nil {
		s.log.Error("Failed to roll back server %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to restore pre-update backup"))
	}

	message := fmt.Sprintf("restored pre-update backup %s; start the server to re-apply the previous modpack files", filename)
	return connect.NewResponse(&v1.RollbackModpackUpdateResponse{
		Status:  "restored",
		Message: message,
	}), nil
}

// ── RPC: Get/SetModpackUpdateSettings ──────────────────────────────────────

func modpackUpdateModeToProto(mode string) v1.ModpackUpdateMode {
	if mode == ModpackUpdateModeApply {
		return v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_APPLY
	}
	return v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_NOTIFY
}

func dbModpackUpdateSettingToProto(setting *storage.ModpackUpdateSetting) *v1.ModpackUpdateSettings {
	proto := &v1.ModpackUpdateSettings{
		ServerId:      setting.ServerID,
		Enabled:       setting.Enabled,
		IntervalHours: int32(setting.IntervalHours),
		Mode:          modpackUpdateModeToProto(setting.Mode),
		LastResult:    setting.LastResult,
	}
	if setting.LastCheck != nil {
		proto.LastCheck = timestamppb.New(*setting.LastCheck)
	}
	return proto
}

// GetModpackUpdateSettings returns the scheduled-update settings for a
// server; servers without stored settings get the defaults.
func (s *ModpackUpdateService) GetModpackUpdateSettings(ctx context.Context, req *connect.Request[v1.GetModpackUpdateSettingsRequest]) (*connect.Response[v1.GetModpackUpdateSettingsResponse], error) {
	if _, err := s.store.GetServer(ctx, req.Msg.ServerId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	setting, err := s.store.GetModpackUpdateSetting(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load update settings"))
	}
	if setting == nil {
		setting = &storage.ModpackUpdateSetting{
			ServerID:      req.Msg.ServerId,
			IntervalHours: DefaultModpackUpdateIntervalHours,
			Mode:          ModpackUpdateModeNotify,
		}
	}

	return connect.NewResponse(&v1.GetModpackUpdateSettingsResponse{
		Settings: dbModpackUpdateSettingToProto(setting),
	}), nil
}

// SetModpackUpdateSettings upserts the scheduled-update settings for a
// server. interval_hours is clamped to >= 1 and the mode enum is validated
// (APPLY persists as "apply", anything else as "notify").
func (s *ModpackUpdateService) SetModpackUpdateSettings(ctx context.Context, req *connect.Request[v1.SetModpackUpdateSettingsRequest]) (*connect.Response[v1.SetModpackUpdateSettingsResponse], error) {
	if _, err := s.store.GetServer(ctx, req.Msg.ServerId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	mode := ModpackUpdateModeNotify
	if req.Msg.Mode == v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_APPLY {
		mode = ModpackUpdateModeApply
	}

	interval := int(req.Msg.IntervalHours)
	if interval < 1 {
		interval = 1
	}

	// Preserve check/update bookkeeping across settings updates.
	setting, err := s.store.GetModpackUpdateSetting(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load update settings"))
	}
	if setting == nil {
		setting = &storage.ModpackUpdateSetting{ServerID: req.Msg.ServerId}
	}
	setting.Enabled = req.Msg.Enabled
	setting.IntervalHours = interval
	setting.Mode = mode

	if err := s.store.SetModpackUpdateSetting(ctx, setting); err != nil {
		s.log.Error("Failed to save modpack update settings for %s: %v", req.Msg.ServerId, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save update settings"))
	}

	return connect.NewResponse(&v1.SetModpackUpdateSettingsResponse{
		Settings: dbModpackUpdateSettingToProto(setting),
	}), nil
}

// ── Scheduled checks loop ──────────────────────────────────────────────────

// Start launches the background loop that periodically checks enabled
// servers for modpack updates (following the watchdog/collector pattern).
func (s *ModpackUpdateService) Start() error {
	s.loopMu.Lock()
	defer s.loopMu.Unlock()
	if s.running {
		return fmt.Errorf("modpack update loop already running")
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.wg.Add(1)
	go s.loop()
	s.log.Info("Modpack update checker started (scan interval: %v)", modpackUpdateCheckInterval)
	return nil
}

// Stop gracefully stops the background loop.
func (s *ModpackUpdateService) Stop() {
	s.loopMu.Lock()
	if !s.running {
		s.loopMu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.loopMu.Unlock()
	s.wg.Wait()
}

func (s *ModpackUpdateService) loop() {
	defer s.wg.Done()

	s.runCheckPass(context.Background())

	ticker := time.NewTicker(modpackUpdateCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runCheckPass(context.Background())
		case <-s.stopChan:
			return
		}
	}
}

// runCheckPass scans every enabled setting whose LastCheck is older than its
// interval and runs a check. Depending on the configured mode it either emits
// a MODPACK_UPDATE_AVAILABLE event or runs the full update pipeline.
func (s *ModpackUpdateService) runCheckPass(ctx context.Context) {
	settings, err := s.store.ListEnabledModpackUpdateSettings(ctx)
	if err != nil {
		s.log.Error("Modpack update checker: failed to list enabled settings: %v", err)
		return
	}

	for _, setting := range settings {
		if ctx.Err() != nil {
			return
		}

		interval := time.Duration(setting.IntervalHours) * time.Hour
		if setting.LastCheck != nil && time.Since(*setting.LastCheck) < interval {
			continue
		}

		// Skip servers with a check/update already in flight
		if !s.begin(setting.ServerID) {
			continue
		}

		check, err := s.checkServer(ctx, setting.ServerID)
		if err != nil {
			// Server was deleted or is otherwise unreachable
			s.log.Debug("Modpack update checker: skipping server %s: %v", setting.ServerID, err)
			s.done(setting.ServerID)
			continue
		}

		if check.Error != "" {
			if recordErr := s.store.RecordModpackUpdateCheck(ctx, setting.ServerID, "error: "+check.Error); recordErr != nil {
				s.log.Error("Modpack update checker: failed to record check for %s: %v", setting.ServerID, recordErr)
			}
			s.done(setting.ServerID)
			continue
		}

		result := "up to date"
		if check.UpdateAvailable {
			result = fmt.Sprintf("update available: %s -> %s", check.CurrentVersion, check.LatestVersion)
		}
		if recordErr := s.store.RecordModpackUpdateCheck(ctx, setting.ServerID, result); recordErr != nil {
			s.log.Error("Modpack update checker: failed to record check for %s: %v", setting.ServerID, recordErr)
		}

		if check.UpdateAvailable {
			if setting.Mode == ModpackUpdateModeApply {
				s.log.Info("Modpack update checker: applying update for server %s (%s -> %s)", setting.ServerID, check.CurrentVersion, check.LatestVersion)
				resp, applyErr := s.applyUpdate(ctx, setting.ServerID, "", true)
				if applyErr != nil {
					s.log.Error("Modpack update checker: scheduled update for %s failed: %v", setting.ServerID, applyErr)
				} else if resp.Status != "updated" {
					s.log.Warn("Modpack update checker: scheduled update for %s finished with status %q: %s", setting.ServerID, resp.Status, resp.Message)
				}
			} else {
				s.bus.Emit(ctx, events.Event{
					Type:     v1.TriggeredEventType_TRIGGERED_EVENT_TYPE_MODPACK_UPDATE_AVAILABLE,
					ServerID: setting.ServerID,
					Data: map[string]any{
						"modpack_name":    check.ModpackName,
						"current_version": check.CurrentVersion,
						"latest_version":  check.LatestVersion,
					},
				})
			}
		}

		s.done(setting.ServerID)
	}
}

// begin marks a server as having a check/update in flight; returns false when
// one is already running.
func (s *ModpackUpdateService) begin(serverID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inFlight[serverID]; ok {
		return false
	}
	s.inFlight[serverID] = struct{}{}
	return true
}

// done clears the in-flight marker for a server.
func (s *ModpackUpdateService) done(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, serverID)
}
