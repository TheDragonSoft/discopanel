package services

import (
	"testing"

	"github.com/nickheyer/discopanel/internal/indexers"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

func TestParseCFPageURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantBase string
		wantFile string
	}{
		{
			name:     "unpinned URL",
			url:      "https://www.curseforge.com/minecraft/modpacks/all-the-mods-10",
			wantBase: "https://www.curseforge.com/minecraft/modpacks/all-the-mods-10",
			wantFile: "",
		},
		{
			name:     "pinned URL",
			url:      "https://www.curseforge.com/minecraft/modpacks/all-the-mods-10/files/5869312",
			wantBase: "https://www.curseforge.com/minecraft/modpacks/all-the-mods-10",
			wantFile: "5869312",
		},
		{
			name:     "takes the last /files/ segment",
			url:      "https://www.curseforge.com/minecraft/modpacks/atm10/files/123/files/456",
			wantBase: "https://www.curseforge.com/minecraft/modpacks/atm10/files/123",
			wantFile: "456",
		},
		{
			name:     "empty URL",
			url:      "",
			wantBase: "",
			wantFile: "",
		},
		{
			name:     "bare files path",
			url:      "/files/999",
			wantBase: "",
			wantFile: "999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, fileID := parseCFPageURL(tt.url)
			if base != tt.wantBase {
				t.Errorf("parseCFPageURL(%q) base = %q, want %q", tt.url, base, tt.wantBase)
			}
			if fileID != tt.wantFile {
				t.Errorf("parseCFPageURL(%q) fileID = %q, want %q", tt.url, fileID, tt.wantFile)
			}
		})
	}
}

func TestModpackUpdateModeToProto(t *testing.T) {
	if got := modpackUpdateModeToProto(ModpackUpdateModeApply); got != v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_APPLY {
		t.Errorf("mode %q should map to APPLY, got %v", ModpackUpdateModeApply, got)
	}
	if got := modpackUpdateModeToProto(ModpackUpdateModeNotify); got != v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_NOTIFY {
		t.Errorf("mode %q should map to NOTIFY, got %v", ModpackUpdateModeNotify, got)
	}
	// Unknown values fall back to notify
	if got := modpackUpdateModeToProto("bogus"); got != v1.ModpackUpdateMode_MODPACK_UPDATE_MODE_NOTIFY {
		t.Errorf("unknown mode should map to NOTIFY, got %v", got)
	}
}

func TestVersionDisplayNamePrecedence(t *testing.T) {
	cases := []struct{ display, version, fileName, want string }{
		{display: "Full Name", version: "1.0.0", fileName: "pack.zip", want: "Full Name"},
		{display: "", version: "1.0.0", fileName: "pack.zip", want: "1.0.0"},
		{display: "", version: "", fileName: "pack.zip", want: "pack.zip"},
	}
	for _, c := range cases {
		f := indexers.ModpackFile{DisplayName: c.display, VersionNumber: c.version, FileName: c.fileName}
		if got := versionDisplayName(f); got != c.want {
			t.Errorf("versionDisplayName(%q,%q,%q) = %q, want %q", c.display, c.version, c.fileName, got, c.want)
		}
	}
}
