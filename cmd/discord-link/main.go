// discord-link is a DiscoPanel module that exposes a Discord slash command to
// manage the Minecraft whitelist through the DiscoPanel AdminService API.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	healthPortDefault = 8302
	// Minecraft player name rules.
	mcNamePattern = `^[A-Za-z0-9_]{1,16}$`
	// HTTP client timeout for panel RPC calls.
	panelTimeout = 15 * time.Second
)

var mcNameRe = regexp.MustCompile(mcNamePattern)

type config struct {
	botToken      string
	guildID       string
	panelURL      string
	apiToken      string
	serverIDs     []string
	allowedRoleID string
	port          int
}

func loadConfig() config {
	var serverIDs []string
	for _, s := range strings.Split(os.Getenv("SERVER_IDS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			serverIDs = append(serverIDs, s)
		}
	}

	return config{
		botToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		guildID:       os.Getenv("DISCORD_GUILD_ID"),
		panelURL:      env("DISCOPANEL_URL", "http://host.docker.internal:8080"),
		apiToken:      os.Getenv("DISCOPANEL_API_TOKEN"),
		serverIDs:     serverIDs,
		allowedRoleID: os.Getenv("ALLOWED_ROLE_ID"),
		port:          envInt("PORT", healthPortDefault),
	}
}

func main() {
	cfg := loadConfig()

	if cfg.botToken == "" {
		fmt.Fprintln(os.Stderr, "DISCORD_BOT_TOKEN required")
		os.Exit(1)
	}
	if cfg.guildID == "" {
		fmt.Fprintln(os.Stderr, "DISCORD_GUILD_ID required")
		os.Exit(1)
	}
	if cfg.apiToken == "" {
		fmt.Fprintln(os.Stderr, "DISCOPANEL_API_TOKEN required (create a panel API token with players create/update permissions)")
		os.Exit(1)
	}

	fmt.Printf("Discord Whitelist Bot: guild=%s panel=%s servers=%d port=%d roleFilter=%v\n",
		cfg.guildID, cfg.panelURL, len(cfg.serverIDs), cfg.port, cfg.allowedRoleID != "")

	bot := &bot{cfg: cfg, http: &http.Client{Timeout: panelTimeout}}

	s, err := discordgo.New("Bot " + cfg.botToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create discord session: %v\n", err)
		os.Exit(1)
	}
	s.AddHandler(bot.onReady)
	s.AddHandler(bot.onInteraction)
	s.AddHandler(func(_ *discordgo.Session, _ *discordgo.Disconnect) {
		fmt.Println("discord disconnected, reconnecting...")
	})

	if err := s.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "open discord session: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// Health endpoint.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	fmt.Printf("Listening :%d\n", cfg.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.port), nil); err != nil {
		fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
		os.Exit(1)
	}
}

type bot struct {
	cfg  config
	http *http.Client
}

// onReady registers the guild slash commands.
func (b *bot) onReady(s *discordgo.Session, ready *discordgo.Ready) {
	fmt.Printf("bot connected as %s\n", ready.User.Username)

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "whitelist",
			Description: "Add a player to the Minecraft whitelist",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "minecraft_name",
					Description: "Minecraft player name (letters, digits, underscores; max 16 chars)",
					Required:    true,
				},
			},
		},
		{
			Name:        "unwhitelist",
			Description: "Remove a player from the Minecraft whitelist",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "minecraft_name",
					Description: "Minecraft player name to remove",
					Required:    true,
				},
			},
		},
	}

	for _, cmd := range commands {
		if _, err := s.ApplicationCommandCreate(s.State.User.ID, b.cfg.guildID, cmd); err != nil {
			fmt.Printf("failed to register /%s: %v\n", cmd.Name, err)
		} else {
			fmt.Printf("registered /%s in guild %s\n", cmd.Name, b.cfg.guildID)
		}
	}
}

// onInteraction handles /whitelist and /unwhitelist.
func (b *bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.ApplicationCommandData().Name != "whitelist" && i.ApplicationCommandData().Name != "unwhitelist" {
		return
	}
	// Slash commands are guild-scoped here, but double-check when a guild is set.
	if b.cfg.guildID != "" && i.GuildID != "" && i.GuildID != b.cfg.guildID {
		return
	}

	// Always respond ephemerally.
	respond := func(content string) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	// Role gate.
	if b.cfg.allowedRoleID != "" && !b.memberHasRole(i.Member, b.cfg.allowedRoleID) {
		respond("You do not have permission to use this command.")
		return
	}

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		respond("Missing `minecraft_name` option.")
		return
	}
	name := strings.TrimSpace(opts[0].StringValue())

	if !mcNameRe.MatchString(name) {
		respond(fmt.Sprintf("`%s` is not a valid Minecraft name (1-16 characters, A-Z a-z 0-9 _).", name))
		return
	}

	add := i.ApplicationCommandData().Name == "whitelist"
	if err := b.callPanel(name, add, i); err != nil {
		fmt.Printf("/%s %s failed: %v\n", i.ApplicationCommandData().Name, name, err)
		respond(fmt.Sprintf("Failed to %s whitelist for `%s`: %s", verb(add), name, err.Error()))
		return
	}

	respond(fmt.Sprintf("Success: %s has been %s the whitelist.", name, pastTense(add)))
}

func verb(add bool) string {
	if add {
		return "add"
	}
	return "remove"
}

func pastTense(add bool) string {
	if add {
		return "added to"
	}
	return "removed from"
}

func (b *bot) memberHasRole(member *discordgo.Member, roleID string) bool {
	if member == nil {
		return false
	}
	for _, r := range member.Roles {
		if r == roleID {
			return true
		}
	}
	return false
}

// callPanel POSTs the whitelist RPCs to the panel via the Connect HTTP/JSON
// protocol: /discopanel.v1.AdminService/<Method> with a JSON body.
func (b *bot) callPanel(name string, add bool, i *discordgo.InteractionCreate) error {
	if add {
		if err := b.postRPC("AddWhitelistEntry", map[string]string{
			"name": name,
			"note": fmt.Sprintf("discord:%s@%s", i.Member.User.ID, i.Member.User.Username),
		}); err != nil {
			return err
		}
	} else {
		// RemoveWhitelistEntry takes the entry's id, so look the name up first.
		id, err := b.lookupEntryID(name)
		if err != nil {
			return err
		}
		if err := b.postRPC("RemoveWhitelistEntry", map[string]string{"id": id}); err != nil {
			return err
		}
	}

	// Empty serverIds means "apply to all servers".
	serverIDs := b.cfg.serverIDs
	if serverIDs == nil {
		serverIDs = []string{}
	}
	return b.postRPC("ApplyWhitelist", map[string]any{"serverIds": serverIDs})
}

// lookupEntryID finds the whitelist entry id for an exact player name via
// ListWhitelistEntries (which searches by substring) and an exact match.
func (b *bot) lookupEntryID(name string) (string, error) {
	var resp struct {
		Entries []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := b.postRPCDecode("ListWhitelistEntries", map[string]string{"search": name}, &resp); err != nil {
		return "", err
	}
	for _, e := range resp.Entries {
		if strings.EqualFold(e.Name, name) {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("whitelist entry %q not found", name)
}

// postRPCDecode sends one Connect JSON request and decodes a successful
// response body into `out`; errors use the Connect error shape.
func (b *bot) postRPCDecode(method string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(b.cfg.panelURL, "/") + "/discopanel.v1.AdminService/" + method
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.apiToken)

	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("panel unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		msg := connectErrorMessage(data)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("%s", msg)
	}
	return json.Unmarshal(data, out)
}

// postRPC sends one Connect JSON request and turns the Connect error shape
// ({"code": ..., "message": ...}) into a Go error.
func (b *bot) postRPC(method string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(b.cfg.panelURL, "/") + "/discopanel.v1.AdminService/" + method
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.apiToken)

	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("panel unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		msg := connectErrorMessage(data)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// connectErrorMessage extracts the message from the Connect JSON error body,
// e.g. {"code":"already_exists","message":"whitelist entry already exists"}.
func connectErrorMessage(data []byte) string {
	var errShape struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &errShape) == nil && errShape.Message != "" {
		return errShape.Message
	}
	return strings.TrimSpace(string(data))
}

// ---------------------------------------------------------------------------
// Env helpers
// ---------------------------------------------------------------------------

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
