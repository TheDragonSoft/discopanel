package terraria

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nickheyer/discopanel/internal/db"
)

// GenerateServerConfig generates the serverconfig.txt contents from a TerrariaConfig
func GenerateServerConfig(config *db.TerrariaConfig) string {
	var sb strings.Builder

	if config.WorldName != "" {
		sb.WriteString(fmt.Sprintf("worldname=%s\n", config.WorldName))
		sb.WriteString(fmt.Sprintf("world=/config/Worlds/%s.wld\n", config.WorldName))
	}

	switch config.WorldSize {
	case "small":
		sb.WriteString("autocreate=1\n")
	case "medium":
		sb.WriteString("autocreate=2\n")
	case "large":
		sb.WriteString("autocreate=3\n")
	default:
		if config.AutoCreate {
			sb.WriteString("autocreate=1\n")
		}
	}

	sb.WriteString(fmt.Sprintf("difficulty=%d\n", config.Difficulty))
	sb.WriteString(fmt.Sprintf("maxplayers=%d\n", config.MaxPlayers))

	if config.Seed != "" {
		sb.WriteString(fmt.Sprintf("seed=%s\n", config.Seed))
	}
	if config.Password != "" {
		sb.WriteString(fmt.Sprintf("password=%s\n", config.Password))
	}
	if config.MOTD != "" {
		sb.WriteString(fmt.Sprintf("motd=%s\n", config.MOTD))
	}
	if config.BanListPath != "" {
		sb.WriteString(fmt.Sprintf("banlist=%s\n", config.BanListPath))
	}
	if config.Secure {
		sb.WriteString("secure=1\n")
	} else {
		sb.WriteString("secure=0\n")
	}
	if config.Language != "" {
		sb.WriteString(fmt.Sprintf("language=%s\n", config.Language))
	}

	if config.CustomConfig != "" {
		sb.WriteString(config.CustomConfig)
		if !strings.HasSuffix(config.CustomConfig, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// ParseServerConfig parses a serverconfig.txt into a TerrariaConfig
func ParseServerConfig(content string, config *db.TerrariaConfig) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "worldname":
			config.WorldName = val
		case "autocreate":
			if val == "1" {
				config.WorldSize = "small"
			} else if val == "2" {
				config.WorldSize = "medium"
			} else if val == "3" {
				config.WorldSize = "large"
			}
			config.AutoCreate = true
		case "difficulty":
			if d, err := strconv.Atoi(val); err == nil {
				config.Difficulty = d
			}
		case "maxplayers":
			if mp, err := strconv.Atoi(val); err == nil {
				config.MaxPlayers = mp
			}
		case "seed":
			config.Seed = val
		case "password":
			config.Password = val
		case "motd":
			config.MOTD = val
		case "banlist":
			config.BanListPath = val
		case "secure":
			config.Secure = val == "1"
		case "language":
			config.Language = val
		}
	}
}
