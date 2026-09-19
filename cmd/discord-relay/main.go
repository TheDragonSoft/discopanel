// discord-relay is a DiscoPanel module that bridges Minecraft chat to Discord
// and (optionally) back. MC -> Discord works via a Discord webhook fed by the
// server log; Discord -> MC requires a bot token and posts messages into the
// game via RCON `say`.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jltobler/go-rcon"
)

const (
	pollInterval = 1 * time.Second
	// Maximum webhook posts per second; anything beyond this is dropped.
	webhookRateLimit = 5
	// Longest "say" text we forward to the game.
	maxSayLength = 256
)

type config struct {
	logPath        string
	webhookURL     string
	serverName     string
	relayChat      bool
	relayJoinLeave bool
	botToken       string
	channelID      string
	rconHost       string
	rconPort       int
	rconPassword   string
	port           int
}

func loadConfig() config {
	return config{
		logPath:        env("LOG_PATH", "/logs/latest.log"),
		webhookURL:     os.Getenv("DISCORD_WEBHOOK_URL"),
		serverName:     env("SERVER_NAME", "Minecraft"),
		relayChat:      envBool("RELAY_CHAT", true),
		relayJoinLeave: envBool("RELAY_JOIN_LEAVE", true),
		botToken:       os.Getenv("DISCORD_BOT_TOKEN"),
		channelID:      os.Getenv("DISCORD_CHANNEL_ID"),
		rconHost:       env("RCON_HOST", "127.0.0.1"),
		rconPort:       envInt("RCON_PORT", 25575),
		rconPassword:   os.Getenv("RCON_PASSWORD"),
		port:           envInt("PORT", 8301),
	}
}

func main() {
	cfg := loadConfig()

	fmt.Printf("Discord Relay: log=%s server=%s chat=%v joinleave=%v webhook=%v port=%d\n",
		cfg.logPath, cfg.serverName, cfg.relayChat, cfg.relayJoinLeave, cfg.webhookURL != "", cfg.port)

	relay := &relay{cfg: cfg}
	relay.webhook = newRateLimiter(webhookRateLimit)

	if cfg.webhookURL != "" {
		fmt.Println("MC -> Discord: webhook configured")
	} else {
		fmt.Println("MC -> Discord: no DISCORD_WEBHOOK_URL set, disabled")
	}

	// Discord -> MC only when both a bot token and a target channel are set.
	if cfg.botToken != "" {
		if cfg.channelID == "" {
			fmt.Println("Discord -> MC: DISCORD_CHANNEL_ID not set, disabled")
		} else if cfg.rconPassword == "" {
			fmt.Println("Discord -> MC: RCON_PASSWORD not set, disabled")
		} else {
			go relay.runDiscordBot()
			relay.discordEnabled = true
		}
	} else {
		fmt.Println("Discord -> MC: no DISCORD_BOT_TOKEN set, disabled")
	}

	// Health endpoint.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Start tailing the log in the background.
	go relay.tailLoop()

	fmt.Printf("Listening :%d\n", cfg.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.port), nil); err != nil {
		fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
		os.Exit(1)
	}
}

type relay struct {
	cfg            config
	webhook        *rateLimiter
	discordEnabled bool
	session        *discordgo.Session
}

// ---------------------------------------------------------------------------
// Log tailing
// ---------------------------------------------------------------------------

// tailLoop polls the log file, forwarding new lines to the parser. It handles
// truncation/rotation (log reset or latest.log pointing to a new file) by
// restarting at offset 0 whenever the file shrinks.
func (r *relay) tailLoop() {
	var (
		offset  int64
		partial strings.Builder
	)

	for {
		f, err := os.Open(r.cfg.logPath)
		if err != nil {
			// Log file may not exist yet (server still starting); retry.
			time.Sleep(pollInterval)
			continue
		}

		info, err := f.Stat()
		if err != nil {
			f.Close()
			time.Sleep(pollInterval)
			continue
		}

		// Rotated or truncated: start over.
		if info.Size() < offset {
			offset = 0
			partial.Reset()
		}

		if _, err := f.Seek(offset, 0); err != nil {
			f.Close()
			time.Sleep(pollInterval)
			continue
		}

		buf := make([]byte, 64*1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				offset += int64(n)

				data := chunk
				if partial.Len() > 0 {
					partial.Write(chunk)
					data = []byte(partial.String())
				}

				// If the chunk does not end with a newline, keep the trailing
				// partial line for the next poll.
				lastNL := bytes.LastIndexByte(data, '\n')
				if lastNL < 0 {
					partial.Reset()
					partial.Write(data)
					continue
				}
				complete := data[:lastNL]
				remainder := data[lastNL+1:]

				for _, line := range strings.Split(string(complete), "\n") {
					r.handleLine(strings.TrimRight(line, "\r"))
				}

				partial.Reset()
				partial.Write(remainder)
			}
			if err != nil {
				break // EOF or read error; resume on next poll
			}
		}

		f.Close()
		time.Sleep(pollInterval)
	}
}

// handleLine parses one log line and dispatches chat/join/leave events.
func (r *relay) handleLine(line string) {
	content, ok := stripLogPrefix(line)
	if !ok {
		return
	}

	if r.cfg.relayChat {
		if m := chatRe.FindStringSubmatch(content); m != nil {
			player := sanitizeMCName(m[1])
			message := strings.TrimSpace(m[2])
			if player != "" && message != "" {
				r.sendWebhook(fmt.Sprintf("**%s**: %s", player, message))
			}
			return
		}
	}

	if r.cfg.relayJoinLeave {
		if m := joinRe.FindStringSubmatch(content); m != nil {
			r.sendWebhook(fmt.Sprintf("➡ **%s** joined the game", sanitizeMCName(m[1])))
			return
		}
		if m := leaveRe.FindStringSubmatch(content); m != nil {
			r.sendWebhook(fmt.Sprintf("⬅ **%s** left the game", sanitizeMCName(m[1])))
			return
		}
	}
}

// stripLogPrefix removes the leading timestamp + logger prefix of the common
// Minecraft server log formats, e.g.:
//
//	[12:34:56] [Server thread/INFO]: <Player> hi            (vanilla)
//	[12:34:56 INFO]: <Player> hi                            (paper/spigot)
//	[12:34:56] [Server thread/INFO] [net.minecraft.server.dedicated.DedicatedServer/]: <Player> hi
//	[12:34:56] [main/INFO]: ...                             (fabric)
func stripLogPrefix(line string) (string, bool) {
	rest := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(rest, "[") {
		return "", false
	}

	// Skip all leading bracket groups.
	for strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return "", false
		}
		rest = rest[end+1:]
	}
	rest = strings.TrimLeft(rest, " ")

	// The separator between prefix and message.
	if strings.HasPrefix(rest, ": ") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, ":") {
		rest = rest[1:]
	} else {
		return "", false
	}

	return strings.TrimLeft(rest, " "), true
}

// Chat lines look like "<Player> message" once the prefix is stripped.
var chatRe = regexp.MustCompile(`^<([^>]+)> (.+)$`)

// "Player joined the game" and "Player[/1.2.3.4:5] logged in with entity id N".
var joinRe = regexp.MustCompile(`^([A-Za-z0-9_]{1,16})(?:\[/[^\]]*\])? (?:joined the game|logged in with entity id)`)

// "Player left the game" and "Player lost connection: ...".
var leaveRe = regexp.MustCompile(`^([A-Za-z0-9_]{1,16}) (?:left the game|lost connection)`)

func sanitizeMCName(name string) string {
	return strings.TrimSpace(name)
}

// ---------------------------------------------------------------------------
// Webhook (MC -> Discord)
// ---------------------------------------------------------------------------

// rateLimiter is a simple sliding-window limiter allowing at most `limit`
// sends per second; excess sends are reported as dropped.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	times   []time.Time
	dropped int64
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit}
}

func (l *rateLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	kept := l.times[:0]
	for _, t := range l.times {
		if now.Sub(t) < time.Second {
			kept = append(kept, t)
		}
	}
	l.times = kept

	if len(l.times) >= l.limit {
		l.dropped++
		if l.dropped == 1 || l.dropped%10 == 0 {
			fmt.Printf("webhook rate limited: dropped %d message(s) so far\n", l.dropped)
		}
		return false
	}
	l.times = append(l.times, now)
	return true
}

// sendWebhook POSTs {"content": ...} to the configured Discord webhook.
func (r *relay) sendWebhook(content string) {
	if r.cfg.webhookURL == "" || content == "" {
		return
	}
	if !r.webhook.allow() {
		return
	}

	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		fmt.Printf("webhook marshal error: %v\n", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, r.cfg.webhookURL, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("webhook request error: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("webhook post error: %v\n", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		fmt.Printf("webhook post failed: status %d\n", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Discord bot (Discord -> MC via RCON)
// ---------------------------------------------------------------------------

func (r *relay) runDiscordBot() {
	for {
		if err := r.runDiscordOnce(); err != nil {
			fmt.Printf("discord bot error: %v\n", err)
		}
		// Back off and retry so the bot survives transient network issues.
		time.Sleep(10 * time.Second)
	}
}

func (r *relay) runDiscordOnce() error {
	s, err := discordgo.New("Bot " + r.cfg.botToken)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	s.AddHandler(r.handleMessage)
	s.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		fmt.Printf("discord bot connected as %s (channel %s)\n", ready.User.Username, r.cfg.channelID)
	})

	r.session = s
	if err := s.Open(); err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer s.Close()

	// Block until the websocket dies; runDiscordBot reconnects.
	select {}
}

// handleMessage forwards Discord messages from the configured channel into
// the game via RCON `say <Player>: <message>`.
func (r *relay) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore our own messages and other bots.
	if m.Author == nil || m.Author.ID == s.State.User.ID || m.Author.Bot {
		return
	}
	if m.ChannelID != r.cfg.channelID {
		return
	}

	content := strings.Join(strings.Fields(m.Content), " ")
	if content == "" {
		return
	}
	if len(content) > maxSayLength {
		content = content[:maxSayLength]
	}

	// Strip markdown/backticks that would look odd in-game.
	content = strings.ReplaceAll(content, "`", "'")
	command := fmt.Sprintf("say %s: %s", m.Author.Username, content)

	output, err := r.sendRCON(command)
	if err != nil {
		fmt.Printf("rcon say error: %v\n", err)
		return
	}
	fmt.Printf("rcon say: %s -> %q\n", m.Author.Username, strings.TrimSpace(output))
}

// sendRCON connects to the server's RCON port and runs one command with a
// timeout, mirroring internal/rcon.SendCommand.
func (r *relay) sendRCON(command string) (string, error) {
	client := rcon.NewClient(fmt.Sprintf("rcon://%s:%d", r.cfg.rconHost, r.cfg.rconPort), r.cfg.rconPassword)

	type result struct {
		output string
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		out, sendErr := client.Send(command)
		resultCh <- result{output: out, err: sendErr}
	}()

	select {
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("rcon command timed out")
	case res := <-resultCh:
		return res.output, res.err
	}
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

func envBool(k string, d bool) bool {
	v := strings.ToLower(os.Getenv(k))
	if v == "" {
		return d
	}
	return v == "true" || v == "1" || v == "yes"
}
