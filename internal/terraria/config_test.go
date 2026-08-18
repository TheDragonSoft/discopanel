package terraria_test

import (
	"strings"
	"testing"

	"github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/terraria"
)

func TestGenerateServerConfig(t *testing.T) {
	cfg := &db.TerrariaConfig{
		WorldName:    "TestTerrariaWorld",
		WorldSize:    "medium",
		Difficulty:   1, // Expert
		MaxPlayers:   16,
		Seed:         "SpecialSeed123",
		Password:     "secret123",
		MOTD:         "Welcome to DiscoPanel Terraria!",
		Secure:       true,
		Language:     "en-US",
		CustomConfig: "npcstream=60\n",
	}

	content := terraria.GenerateServerConfig(cfg)

	if !strings.Contains(content, "worldname=TestTerrariaWorld") {
		t.Errorf("expected worldname=TestTerrariaWorld in output, got:\n%s", content)
	}
	if !strings.Contains(content, "autocreate=2") {
		t.Errorf("expected autocreate=2 (medium) in output, got:\n%s", content)
	}
	if !strings.Contains(content, "difficulty=1") {
		t.Errorf("expected difficulty=1 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "maxplayers=16") {
		t.Errorf("expected maxplayers=16 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "seed=SpecialSeed123") {
		t.Errorf("expected seed=SpecialSeed123 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "password=secret123") {
		t.Errorf("expected password=secret123 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "motd=Welcome to DiscoPanel Terraria!") {
		t.Errorf("expected motd in output, got:\n%s", content)
	}
	if !strings.Contains(content, "secure=1") {
		t.Errorf("expected secure=1 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "npcstream=60") {
		t.Errorf("expected custom config in output, got:\n%s", content)
	}
}

func TestParseServerConfig(t *testing.T) {
	rawConfig := `
# Terraria Server Config
worldname=AdventureWorld
world=/config/Worlds/AdventureWorld.wld
autocreate=3
difficulty=2
maxplayers=24
password=mypass
motd=Awesome Terraria Server
secure=1
language=en-US
`
	var cfg db.TerrariaConfig
	terraria.ParseServerConfig(rawConfig, &cfg)

	if cfg.WorldName != "AdventureWorld" {
		t.Errorf("expected WorldName to be AdventureWorld, got %s", cfg.WorldName)
	}
	if cfg.WorldSize != "large" {
		t.Errorf("expected WorldSize to be large, got %s", cfg.WorldSize)
	}
	if cfg.Difficulty != 2 {
		t.Errorf("expected Difficulty to be 2 (Master), got %d", cfg.Difficulty)
	}
	if cfg.MaxPlayers != 24 {
		t.Errorf("expected MaxPlayers to be 24, got %d", cfg.MaxPlayers)
	}
	if cfg.Password != "mypass" {
		t.Errorf("expected Password to be mypass, got %s", cfg.Password)
	}
	if cfg.MOTD != "Awesome Terraria Server" {
		t.Errorf("expected MOTD to be Awesome Terraria Server, got %s", cfg.MOTD)
	}
	if !cfg.Secure {
		t.Errorf("expected Secure to be true, got %v", cfg.Secure)
	}
}
