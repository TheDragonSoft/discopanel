package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"github.com/nickheyer/discopanel/internal/command"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/internal/minecraft"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that AdminService implements the interface
var _ discopanelv1connect.AdminServiceHandler = (*AdminService)(nil)

// whitelistNameRE matches plausible Minecraft player names. It doubles as an
// injection guard: only names matching this are ever interpolated into an
// RCON command line.
var whitelistNameRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)

// iconDataURIPrefix is stripped when a client sends the icon as a data URI.
var iconDataURIPrefix = regexp.MustCompile(`(?i)^data:image/[a-z]+;base64,`)

// ⚡ Bolt: pre-compile regex to avoid O(N) allocation bottleneck
var (
	mcColorRe   = regexp.MustCompile("§.")
	ansiColorRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// AdminService implements the Admin service (whitelist, bans, MOTD/icon).
type AdminService struct {
	store  *storage.Store
	sender *command.Sender
	docker *docker.Client
	log    *logger.Logger
}

// NewAdminService creates a new admin service
func NewAdminService(store *storage.Store, sender *command.Sender, dockerClient *docker.Client, log *logger.Logger) *AdminService {
	return &AdminService{
		store:  store,
		sender: sender,
		docker: dockerClient,
		log:    log,
	}
}

// dbWhitelistEntryToProto converts a database whitelist entry to proto
func dbWhitelistEntryToProto(entry *storage.WhitelistEntry) *v1.WhitelistEntry {
	if entry == nil {
		return nil
	}
	return &v1.WhitelistEntry{
		Id:        entry.ID,
		Name:      entry.Name,
		Note:      entry.Note,
		CreatedAt: timestamppb.New(entry.CreatedAt),
	}
}

// serverOpResult builds a per-server result skeleton
func serverOpResult(server *storage.Server) *v1.ServerOpResult {
	return &v1.ServerOpResult{
		ServerId:   server.ID,
		ServerName: server.Name,
	}
}

// ListWhitelistEntries lists whitelist entries stored in the panel
func (s *AdminService) ListWhitelistEntries(ctx context.Context, req *connect.Request[v1.ListWhitelistEntriesRequest]) (*connect.Response[v1.ListWhitelistEntriesResponse], error) {
	entries, err := s.store.ListWhitelistEntries(ctx, req.Msg.Search)
	if err != nil {
		s.log.Error("Failed to list whitelist entries: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list whitelist entries"))
	}

	protoEntries := make([]*v1.WhitelistEntry, 0, len(entries))
	for _, entry := range entries {
		protoEntries = append(protoEntries, dbWhitelistEntryToProto(entry))
	}

	return connect.NewResponse(&v1.ListWhitelistEntriesResponse{
		Entries: protoEntries,
	}), nil
}

// AddWhitelistEntry adds a player to the panel-side whitelist, updating the
// note when the name already exists.
func (s *AdminService) AddWhitelistEntry(ctx context.Context, req *connect.Request[v1.AddWhitelistEntryRequest]) (*connect.Response[v1.AddWhitelistEntryResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	if !whitelistNameRE.MatchString(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid player name"))
	}

	entry, err := s.store.UpsertWhitelistEntry(ctx, name, req.Msg.Note)
	if err != nil {
		s.log.Error("Failed to upsert whitelist entry %s: %v", name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save whitelist entry"))
	}

	return connect.NewResponse(&v1.AddWhitelistEntryResponse{
		Entry: dbWhitelistEntryToProto(entry),
	}), nil
}

// RemoveWhitelistEntry removes a player from the panel-side whitelist
func (s *AdminService) RemoveWhitelistEntry(ctx context.Context, req *connect.Request[v1.RemoveWhitelistEntryRequest]) (*connect.Response[v1.RemoveWhitelistEntryResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	if err := s.store.DeleteWhitelistEntry(ctx, req.Msg.Id); err != nil {
		if err.Error() == "whitelist entry not found" {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("whitelist entry not found"))
		}
		s.log.Error("Failed to delete whitelist entry %s: %v", req.Msg.Id, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete whitelist entry"))
	}

	return connect.NewResponse(&v1.RemoveWhitelistEntryResponse{}), nil
}

// targetServers resolves the servers an operation applies to: the listed IDs,
// or every server when the list is empty.
func (s *AdminService) targetServers(ctx context.Context, serverIDs []string) ([]*storage.Server, error) {
	if len(serverIDs) == 0 {
		return s.store.ListServers(ctx)
	}
	servers := make([]*storage.Server, 0, len(serverIDs))
	for _, id := range serverIDs {
		server, err := s.store.GetServer(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("server %s not found", id)
		}
		servers = append(servers, server)
	}
	return servers, nil
}

// isServerRunning reports whether the server's container is currently running.
func (s *AdminService) isServerRunning(ctx context.Context, server *storage.Server) bool {
	if server.ContainerID == "" || s.docker == nil {
		return false
	}
	status, err := s.docker.GetContainerStatus(ctx, server.ContainerID)
	return err == nil && status == storage.StatusRunning
}

// ApplyWhitelist converges the selected servers to the panel-side whitelist.
// Per-server failures are reported in the results and never fail the whole RPC.
func (s *AdminService) ApplyWhitelist(ctx context.Context, req *connect.Request[v1.ApplyWhitelistRequest]) (*connect.Response[v1.ApplyWhitelistResponse], error) {
	names, err := s.store.ListWhitelistNames(ctx)
	if err != nil {
		s.log.Error("Failed to list whitelist names: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list whitelist names"))
	}
	desired := make(map[string]bool, len(names))
	for _, name := range names {
		desired[name] = true
	}

	servers, err := s.targetServers(ctx, req.Msg.ServerIds)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	results := make([]*v1.ServerOpResult, 0, len(servers))
	for _, server := range servers {
		results = append(results, s.applyWhitelistToServer(ctx, server, names, desired))
	}

	return connect.NewResponse(&v1.ApplyWhitelistResponse{
		Results: results,
	}), nil
}

// applyWhitelistToServer diffs one server's current whitelist against the
// panel list and issues the add/remove/reload RCON commands.
func (s *AdminService) applyWhitelistToServer(ctx context.Context, server *storage.Server, names []string, desired map[string]bool) *v1.ServerOpResult {
	result := serverOpResult(server)

	if !s.isServerRunning(ctx, server) {
		result.Success = false
		result.Message = "server not running"
		return result
	}

	// Read the server's current whitelist.
	output, err := s.sender.SendCommand(ctx, server.ID, "whitelist list")
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("failed to read whitelist: %v", err)
		return result
	}
	current, parsed := parseWhitelistListOutput(output)

	// When the output cannot be parsed, only add missing names and leave
	// existing ones alone rather than risk removing everyone.
	currentSet := make(map[string]bool, len(current))
	for _, name := range current {
		currentSet[name] = true
	}

	var added, removed []string
	var failures []string

	run := func(command string) bool {
		if _, err := s.sender.SendCommand(ctx, server.ID, command); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", command, err))
			return false
		}
		return true
	}

	for _, name := range names {
		if !currentSet[name] {
			if run("whitelist add " + name) {
				added = append(added, name)
			}
		}
	}
	if parsed {
		for _, name := range current {
			if !desired[name] {
				if run("whitelist remove " + name) {
					removed = append(removed, name)
				}
			}
		}
	}

	// Reload so the server applies the updated whitelist file.
	run("whitelist reload")

	if len(failures) > 0 {
		result.Success = false
		result.Message = strings.Join(failures, "; ")
	} else {
		result.Success = true
		result.Message = fmt.Sprintf("added %d, removed %d", len(added), len(removed))
	}
	result.Added = added
	result.Removed = removed
	// The converged state is the panel list; when the parse failed or any
	// command failed, report what the server actually has (best effort).
	if result.Success {
		result.Current = names
	} else {
		effective := make([]string, 0, len(current))
		for _, name := range current {
			if !desired[name] {
				effective = append(effective, name)
			}
		}
		effective = append(effective, names...)
		result.Current = effective
	}
	return result
}

// PullWhitelist reads the whitelist of one server via RCON and imports every
// name into the panel whitelist.
func (s *AdminService) PullWhitelist(ctx context.Context, req *connect.Request[v1.PullWhitelistRequest]) (*connect.Response[v1.PullWhitelistResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}
	if !s.isServerRunning(ctx, server) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("server not running"))
	}

	output, err := s.sender.SendCommand(ctx, server.ID, "whitelist list")
	if err != nil {
		s.log.Error("Failed to read whitelist from %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read whitelist"))
	}
	names, ok := parseWhitelistListOutput(output)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("could not parse whitelist list output"))
	}

	imported := make([]string, 0, len(names))
	for _, name := range names {
		if !whitelistNameRE.MatchString(name) {
			continue
		}
		if _, err := s.store.UpsertWhitelistEntry(ctx, name, ""); err != nil {
			s.log.Error("Failed to import whitelist entry %s: %v", name, err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to import whitelist entries"))
		}
		imported = append(imported, name)
	}

	return connect.NewResponse(&v1.PullWhitelistResponse{
		Imported: imported,
	}), nil
}

// BanPlayer bans a player on the selected servers via RCON. The panel
// whitelist is intentionally left untouched.
func (s *AdminService) BanPlayer(ctx context.Context, req *connect.Request[v1.BanPlayerRequest]) (*connect.Response[v1.BanPlayerResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	if !whitelistNameRE.MatchString(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid player name"))
	}
	command := "ban " + name
	if strings.TrimSpace(req.Msg.Reason) != "" {
		command += " " + strings.TrimSpace(req.Msg.Reason)
	}

	servers, err := s.targetServers(ctx, req.Msg.ServerIds)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&v1.BanPlayerResponse{
		Results: s.runOnServers(ctx, servers, command),
	}), nil
}

// UnbanPlayer unbans a player on the selected servers via RCON
func (s *AdminService) UnbanPlayer(ctx context.Context, req *connect.Request[v1.UnbanPlayerRequest]) (*connect.Response[v1.UnbanPlayerResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	if !whitelistNameRE.MatchString(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid player name"))
	}

	servers, err := s.targetServers(ctx, req.Msg.ServerIds)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&v1.UnbanPlayerResponse{
		Results: s.runOnServers(ctx, servers, "unban "+name),
	}), nil
}

// runOnServers runs a single RCON command on each target server and reports
// one result per server, never failing the whole RPC on per-server errors.
func (s *AdminService) runOnServers(ctx context.Context, servers []*storage.Server, command string) []*v1.ServerOpResult {
	results := make([]*v1.ServerOpResult, 0, len(servers))
	for _, server := range servers {
		result := serverOpResult(server)
		if !s.isServerRunning(ctx, server) {
			result.Success = false
			result.Message = "server not running"
			results = append(results, result)
			continue
		}
		if _, err := s.sender.SendCommand(ctx, server.ID, command); err != nil {
			result.Success = false
			result.Message = err.Error()
		} else {
			result.Success = true
		}
		results = append(results, result)
	}
	return results
}

// GetServerMotd reads the MOTD, icon and related settings from a stopped or
// running server's server.properties / icon.png.
func (s *AdminService) GetServerMotd(ctx context.Context, req *connect.Request[v1.GetServerMotdRequest]) (*connect.Response[v1.GetServerMotdResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	props, err := minecraft.LoadServerProperties(server.DataPath)
	if err != nil {
		// Missing server.properties is fine for a fresh server; other read
		// failures are real errors.
		if !errors.Is(err, os.ErrNotExist) {
			s.log.Error("Failed to load server.properties for %s: %v", server.Name, err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read server.properties"))
		}
		props = minecraft.ServerProperties{}
	}

	response := &v1.GetServerMotdResponse{
		Motd:             props.GetString("motd", ""),
		WhitelistEnabled: props.GetBool("white-list", false),
		OnlineMode:       props.GetBool("online-mode", true),
		MaxPlayers:       int32(props.GetInt("max-players", server.MaxPlayers)),
	}

	if iconBytes, err := os.ReadFile(filepath.Join(server.DataPath, "icon.png")); err == nil && len(iconBytes) > 0 {
		response.Icon = base64.StdEncoding.EncodeToString(iconBytes)
	}

	return connect.NewResponse(response), nil
}

// UpdateServerMotd writes the MOTD, icon and related settings. Properties are
// only written while the server is stopped (otherwise they would be
// overwritten on restart and the change would not take effect anyway), so a
// running server gets FailedPrecondition unless only the whitelist toggle is
// requested - that one can be applied live via RCON.
func (s *AdminService) UpdateServerMotd(ctx context.Context, req *connect.Request[v1.UpdateServerMotdRequest]) (*connect.Response[v1.UpdateServerMotdResponse], error) {
	msg := req.Msg

	server, err := s.store.GetServer(ctx, msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	running := s.isServerRunning(ctx, server)
	liveWhitelistToggle := running && msg.WhitelistEnabled != nil

	needsProps := msg.Motd != nil || msg.Icon != nil || msg.WhitelistEnabled != nil ||
		msg.OnlineMode != nil || msg.MaxPlayers != nil
	if needsProps && running {
		if liveWhitelistToggle && msg.Motd == nil && msg.Icon == nil && msg.OnlineMode == nil && msg.MaxPlayers == nil {
			// Whitelist toggle only - apply via RCON below.
			if err := s.toggleWhitelist(ctx, server, *msg.WhitelistEnabled); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return connect.NewResponse(&v1.UpdateServerMotdResponse{Status: "updated"}), nil
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("server must be stopped to update server.properties"))
	}

	// Load current properties, apply the requested fields and save.
	props, err := minecraft.LoadServerProperties(server.DataPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Error("Failed to load server.properties for %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read server.properties"))
	}

	if msg.Motd != nil {
		props.SetString("motd", *msg.Motd)
	}
	if msg.WhitelistEnabled != nil {
		props.SetBool("white-list", *msg.WhitelistEnabled)
	}
	if msg.OnlineMode != nil {
		props.SetBool("online-mode", *msg.OnlineMode)
	}
	if msg.MaxPlayers != nil {
		props.SetInt("max-players", int(*msg.MaxPlayers))
	}
	if err := minecraft.SaveServerProperties(server.DataPath, props); err != nil {
		s.log.Error("Failed to save server.properties for %s: %v", server.Name, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write server.properties"))
	}

	// Icon: empty string means remove icon.png, otherwise decode and write.
	if msg.Icon != nil {
		iconPath := filepath.Join(server.DataPath, "icon.png")
		icon := iconDataURIPrefix.ReplaceAllString(*msg.Icon, "")
		if icon == "" {
			if err := os.Remove(iconPath); err != nil && !os.IsNotExist(err) {
				s.log.Error("Failed to remove icon.png for %s: %v", server.Name, err)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to remove icon.png"))
			}
		} else {
			iconBytes, err := base64.StdEncoding.DecodeString(icon)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("icon is not valid base64 PNG data"))
			}
			if err := os.WriteFile(iconPath, iconBytes, 0644); err != nil {
				s.log.Error("Failed to write icon.png for %s: %v", server.Name, err)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write icon.png"))
			}
		}
	}

	// When the server is up (should not normally happen after the guard
	// above), apply the whitelist toggle live so it takes effect immediately.
	if liveWhitelistToggle {
		if err := s.toggleWhitelist(ctx, server, *msg.WhitelistEnabled); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	return connect.NewResponse(&v1.UpdateServerMotdResponse{Status: "updated"}), nil
}

// toggleWhitelist turns the whitelist on or off on a running server and
// reloads it.
func (s *AdminService) toggleWhitelist(ctx context.Context, server *storage.Server, enabled bool) error {
	command := "whitelist off"
	if enabled {
		command = "whitelist on"
	}
	for _, cmd := range []string{command, "whitelist reload"} {
		if _, err := s.sender.SendCommand(ctx, server.ID, cmd); err != nil {
			return fmt.Errorf("failed to %s whitelist: %v", cmd, err)
		}
	}
	return nil
}

// stripChatFormatting removes Minecraft section-sign color codes and ANSI
// escape sequences from server console output.
func stripChatFormatting(text string) string {
	text = mcColorRe.ReplaceAllString(text, "")
	text = ansiColorRe.ReplaceAllString(text, "")
	return text
}

// parseWhitelistListOutput parses the output of `whitelist list`. Returns the
// parsed names and whether the output was recognized; when it was not, the
// caller should treat the server list as unknown and add only.
func parseWhitelistListOutput(output string) ([]string, bool) {
	stripped := stripChatFormatting(output)
	lower := strings.ToLower(stripped)
	if !strings.Contains(lower, "whitelist") {
		return nil, false
	}

	// Names, when present, follow the last colon: e.g.
	//   "There are 2 whitelisted players: Steve, Alex"
	//   "Whitelisted players: Steve"
	if idx := strings.LastIndex(stripped, ":"); idx != -1 {
		names := make([]string, 0, 8)
		for _, part := range strings.Split(stripped[idx+1:], ",") {
			name := strings.TrimSpace(part)
			if name == "" || strings.EqualFold(name, "none") {
				continue
			}
			if !whitelistNameRE.MatchString(name) {
				// One malformed token makes the whole output untrusted.
				return nil, false
			}
			names = append(names, name)
		}
		return names, true
	}

	// No colon: "There are 0 whitelisted players" means an empty whitelist.
	if strings.Contains(lower, "0") && strings.Contains(lower, "player") {
		return nil, true
	}
	return nil, false
}
