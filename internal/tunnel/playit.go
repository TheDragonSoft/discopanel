package tunnel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/nickheyer/discopanel/internal/config"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/docker"
	"github.com/nickheyer/discopanel/pkg/logger"
)

const (
	PlayitDockerImage        = "ghcr.io/playit-cloud/playit-agent:latest"
	PlayitAccountSecretKey   = "playit_account_secret_key"
	PlayitAccountSessionKey  = "playit_account_session"
)

var (
	claimURLRegex     = regexp.MustCompile(`https?://playit\.gg/claim/([a-zA-Z0-9_\-]+)`)
	publicAddrRegex   = regexp.MustCompile(`([a-zA-Z0-9_\-]+\.(?:gl\.joinmc\.link|craft\.ply\.gg|ply\.gg|auto\.playit\.gg))(?::(\d+))?`)
	tunnelActiveRegex = regexp.MustCompile(`(?i)(tunnel active|tunnel running|connected to server|established connection|registered tunnel|agent registered)`)
)

type PlayitDriver struct {
	docker *docker.Client
	config *config.Config
	log    *logger.Logger
}

func NewPlayitDriver(dockerClient *docker.Client, cfg *config.Config, log *logger.Logger) *PlayitDriver {
	return &PlayitDriver{
		docker: dockerClient,
		config: cfg,
		log:    log,
	}
}

// EnsureImage ensures the Playit Docker image is downloaded locally
func (d *PlayitDriver) EnsureImage(ctx context.Context) error {
	d.log.Info("Ensuring playit docker image %s is available...", PlayitDockerImage)
	if err := d.docker.PullImage(ctx, PlayitDockerImage); err != nil {
		d.log.Warn("Failed to pull image %s: %v, checking for local cached image", PlayitDockerImage, err)
		if _, _, inspectErr := d.docker.GetDockerClient().ImageInspectWithRaw(ctx, PlayitDockerImage); inspectErr != nil {
			return fmt.Errorf("failed to pull required playit image %s: %w", PlayitDockerImage, err)
		}
		d.log.Info("Using locally cached image %s", PlayitDockerImage)
	}
	return nil
}

// CreateContainer creates a Playit container for a tunnel
func (d *PlayitDriver) CreateContainer(ctx context.Context, tunnel *storage.Tunnel, server *storage.Server, accountSecretKey string) (string, error) {
	// Ensure image is downloaded
	if err := d.EnsureImage(ctx); err != nil {
		return "", err
	}

	// Prepare persistent config dir
	dataDir := d.config.Storage.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	tunnelDataPath := filepath.Join(dataDir, "tunnels", tunnel.ID)
	if err := os.MkdirAll(tunnelDataPath, 0755); err != nil {
		d.log.Warn("Failed to create tunnel data dir %s: %v", tunnelDataPath, err)
	}

	targetHost := tunnel.TargetHost
	if targetHost == "" {
		targetHost = fmt.Sprintf("discopanel-server-%s", tunnel.ServerID)
	}

	env := []string{
		fmt.Sprintf("DISCOPANEL_TUNNEL_ID=%s", tunnel.ID),
		fmt.Sprintf("DISCOPANEL_SERVER_ID=%s", tunnel.ServerID),
		fmt.Sprintf("DISCOPANEL_TARGET_HOST=%s", targetHost),
		fmt.Sprintf("DISCOPANEL_TARGET_PORT=%d", tunnel.TargetPort),
	}

	// Secret key precedence: tunnel-specific secret > global account secret
	secretKey := tunnel.SecretKey
	if secretKey == "" && accountSecretKey != "" {
		secretKey = accountSecretKey
	}
	if secretKey == "" {
		return "", fmt.Errorf("Playit secret key is required. Please link your account in Settings -> Routing or enter a secret key for this tunnel")
	}

	env = append(env,
		fmt.Sprintf("SECRET_KEY=%s", secretKey),
		fmt.Sprintf("PLAYIT_SECRET_KEY=%s", secretKey),
	)

	// Volume mount for persistent playit.toml / configuration
	hostDataPath := docker.TranslateToHostPath(tunnelDataPath)
	mounts := []mount.Mount{
		{
			Type:        mount.TypeBind,
			Source:      hostDataPath,
			Target:      "/etc/playit",
			ReadOnly:    false,
			BindOptions: &mount.BindOptions{CreateMountpoint: true},
		},
	}

	containerConfig := &container.Config{
		Image:        PlayitDockerImage,
		Env:          env,
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
		Labels: map[string]string{
			"discopanel.tunnel.id":        tunnel.ID,
			"discopanel.tunnel.server_id": tunnel.ServerID,
			"discopanel.tunnel.provider":  "playit",
			"discopanel.managed":          "true",
		},
	}

	hostConfig := &container.HostConfig{
		Mounts: mounts,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		Resources: container.Resources{
			Memory: 256 * 1024 * 1024, // 256MB limit
		},
		LogConfig: container.LogConfig{
			Type:   "json-file",
			Config: map[string]string{"max-size": "10m", "max-file": "2"},
		},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
	}

	networkConfig := &network.NetworkingConfig{}
	if d.config.Docker.NetworkName != "" {
		networkConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			d.config.Docker.NetworkName: {},
		}
	}

	containerName := fmt.Sprintf("discopanel-tunnel-%s", tunnel.ID)
	resp, err := d.docker.GetDockerClient().ContainerCreate(
		ctx, containerConfig, hostConfig, networkConfig, nil, containerName,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create playit container: %w", err)
	}

	return resp.ID, nil
}

// ReadSecretFromToml attempts to read and extract the generated Playit secret key from the mounted directory
func ReadSecretFromToml(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "playit.toml"),
		filepath.Join(dir, "playit.secret"),
	}
	for _, f := range candidates {
		data, err := os.ReadFile(f)
		if err == nil {
			re := regexp.MustCompile(`(?i)(?:secret|secret_key)\s*=\s*["']?([a-zA-Z0-9_\-]+)["']?`)
			if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
				return matches[1], nil
			}
			trimmed := strings.TrimSpace(string(data))
			if len(trimmed) > 10 && !strings.Contains(trimmed, "\n") {
				return trimmed, nil
			}
		}
	}
	return "", fmt.Errorf("secret not found in %s", dir)
}

// SniffLogs analyzes recent log output to extract claim URL, claim code, or public address
func (d *PlayitDriver) SniffLogs(logs []string) (claimURL, claimCode, publicAddr string, publicPort int, isRunning bool) {
	for _, line := range logs {
		// Sniff Claim URL
		if matches := claimURLRegex.FindStringSubmatch(line); len(matches) > 0 {
			claimURL = matches[0]
			if len(matches) > 1 {
				claimCode = matches[1]
			}
		}

		// Sniff Public Address & Port
		if matches := publicAddrRegex.FindStringSubmatch(line); len(matches) > 0 {
			publicAddr = matches[1]
			if len(matches) > 2 && matches[2] != "" {
				fmt.Sscanf(matches[2], "%d", &publicPort)
			}
		}

		// Sniff Tunnel Active state
		if tunnelActiveRegex.MatchString(line) || strings.Contains(line, "gl.joinmc.link") || strings.Contains(line, "ply.gg") {
			isRunning = true
		}
	}

	return claimURL, claimCode, publicAddr, publicPort, isRunning
}
