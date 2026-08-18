package terraria_test

import (
	"testing"

	"github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/terraria"
)

func TestGetDockerImage(t *testing.T) {
	tests := []struct {
		flavor   db.TerrariaFlavor
		version  string
		expected string
	}{
		{db.TerrariaFlavorVanilla, "", "beardedio/terraria:latest"},
		{db.TerrariaFlavorVanilla, "1.4.4.9", "beardedio/terraria:vanilla-1.4.4.9"},
		{db.TerrariaFlavorTShock, "", "beardedio/terraria:tshock-latest"},
		{db.TerrariaFlavorTShock, "v5.2", "beardedio/terraria:tshock-v5.2"},
		{db.TerrariaFlavorTModLoader, "", "jacobgardner/tmodloader:latest"},
		{db.TerrariaFlavorTModLoader, "1.4", "jacobgardner/tmodloader:1.4"},
	}

	for _, tt := range tests {
		img := terraria.GetDockerImage(tt.flavor, tt.version)
		if img != tt.expected {
			t.Errorf("GetDockerImage(%s, %s) = %s, expected %s", tt.flavor, tt.version, img, tt.expected)
		}
	}
}

func TestBuildContainerConfig(t *testing.T) {
	server := &db.Server{
		ID:              "test-srv-1",
		Name:            "TerrariaWorld",
		GameType:        db.GameTypeTerraria,
		TerrariaFlavor:  db.TerrariaFlavorVanilla,
		TerrariaVersion: "1.4.4.9",
		DataPath:        "/tmp/discopanel/servers/test-srv-1",
		Memory:          2048,
	}

	cfg := &db.TerrariaConfig{
		WorldName:  "TerrariaWorld",
		WorldSize:  "small",
		Difficulty: 0,
		MaxPlayers: 8,
		Password:   "letmein",
	}

	containerCfg, hostCfg := terraria.BuildContainerConfig(server, cfg)

	if containerCfg.Image != "beardedio/terraria:vanilla-1.4.4.9" {
		t.Errorf("expected image beardedio/terraria:vanilla-1.4.4.9, got %s", containerCfg.Image)
	}
	if !containerCfg.Tty || !containerCfg.OpenStdin {
		t.Errorf("expected Tty and OpenStdin to be true for interactive console")
	}
	if hostCfg.Resources.Memory != 2048*1024*1024 {
		t.Errorf("expected 2048MB memory limit, got %d", hostCfg.Resources.Memory)
	}
	if len(hostCfg.Mounts) == 0 || hostCfg.Mounts[0].Target != "/config" {
		t.Errorf("expected mount target to be /config, got %v", hostCfg.Mounts)
	}
}
