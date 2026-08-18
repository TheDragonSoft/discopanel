package terraria

import (
	"fmt"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/nickheyer/discopanel/internal/db"
)

// GetDockerImage returns the appropriate docker image for a Terraria flavor and version
func GetDockerImage(flavor db.TerrariaFlavor, version string) string {
	switch flavor {
	case db.TerrariaFlavorTShock:
		if version == "" || version == "latest" {
			return "beardedio/terraria:tshock-latest"
		}
		return fmt.Sprintf("beardedio/terraria:tshock-%s", version)
	case db.TerrariaFlavorTModLoader:
		if version == "" || version == "latest" {
			return "jacobgardner/tmodloader:latest"
		}
		return fmt.Sprintf("jacobgardner/tmodloader:%s", version)
	case db.TerrariaFlavorVanilla:
		fallthrough
	default:
		if version == "" || version == "latest" {
			return "beardedio/terraria:latest"
		}
		return fmt.Sprintf("beardedio/terraria:vanilla-%s", version)
	}
}

// BuildContainerConfig builds the Docker container configuration for a Terraria server
func BuildContainerConfig(server *db.Server, terrariaConfig *db.TerrariaConfig) (*container.Config, *container.HostConfig) {
	image := server.DockerImage
	if image == "" {
		image = GetDockerImage(server.TerrariaFlavor, server.TerrariaVersion)
	}

	env := []string{
		fmt.Sprintf("WORLD_NAME=%s", terrariaConfig.WorldName),
		fmt.Sprintf("WORLD_SIZE=%s", terrariaConfig.WorldSize),
		fmt.Sprintf("DIFFICULTY=%d", terrariaConfig.Difficulty),
		fmt.Sprintf("MAX_PLAYERS=%d", terrariaConfig.MaxPlayers),
	}

	if terrariaConfig.Password != "" {
		env = append(env, fmt.Sprintf("PASSWORD=%s", terrariaConfig.Password))
	}

	cfg := &container.Config{
		Image: image,
		Env:   env,
		Labels: map[string]string{
			"discopanel.server.id": server.ID,
		},
		Tty:       true,
		OpenStdin: true,
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: filepath.Join(server.DataPath, "config"),
				Target: "/config",
			},
		},
	}

	// Memory limits if set
	if server.Memory > 0 {
		hostConfig.Resources.Memory = int64(server.Memory) * 1024 * 1024
	}

	return cfg, hostConfig
}
