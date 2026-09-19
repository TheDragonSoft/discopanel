package alias

import (
	"strings"
	"testing"

	models "github.com/nickheyer/discopanel/internal/db"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
)

func TestSubstituteDepsReferences(t *testing.T) {
	mariadb := &models.Module{
		ID:   "mod-123",
		Name: "Database",
		Ports: []*v1.ModulePort{
			{Name: "DB", ContainerPort: 3306, Protocol: "tcp"},
			{Name: "Admin", ContainerPort: 8080, Protocol: "http"},
		},
	}

	tests := []struct {
		name  string
		input string
		deps  map[string]*models.Module
		want  string
	}{
		{
			name:  "host resolves to container name",
			input: "jdbc:mariadb://{{deps.mariadb.host}}:3306/minecraft",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "jdbc:mariadb://discopanel-module-mod-123:3306/minecraft",
		},
		{
			name:  "container matches host",
			input: "{{deps.mariadb.container}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "discopanel-module-mod-123",
		},
		{
			name:  "id resolves to module id",
			input: "{{deps.mariadb.id}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "mod-123",
		},
		{
			name:  "name resolves to module name",
			input: "{{deps.mariadb.name}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "Database",
		},
		{
			name:  "named port resolves to container port",
			input: "{{deps.mariadb.port_DB}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "3306",
		},
		{
			name:  "second named port resolves",
			input: "{{deps.mariadb.port_Admin}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "8080",
		},
		{
			name:  "unknown named port strips alias",
			input: "port={{deps.mariadb.port_MISSING}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "port=",
		},
		{
			name:  "unknown capability strips alias",
			input: "{{deps.redis.host}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "",
		},
		{
			name:  "nil deps map strips alias",
			input: "{{deps.mariadb.host}}",
			deps:  nil,
			want:  "",
		},
		{
			name:  "malformed alias strips",
			input: "{{deps.mariadb}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "",
		},
		{
			name:  "multiple aliases in one string",
			input: "{{deps.mariadb.host}}:{{deps.mariadb.port_DB}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "discopanel-module-mod-123:3306",
		},
		{
			name:  "non-deps aliases untouched",
			input: "{{server.id}} and {{modules.x.host}}",
			deps:  map[string]*models.Module{"mariadb": mariadb},
			want:  "{{server.id}} and {{modules.x.host}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext()
			ctx.Deps = tt.deps
			got := Substitute(tt.input, ctx)
			if got != tt.want {
				t.Errorf("Substitute(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSubstituteDepsDoesNotLoopOnNestedBraces(t *testing.T) {
	// A dep value containing braces must not re-trigger substitution
	module := &models.Module{
		ID:       "mod-x",
		Name:     "weird {{name}}",
		ServerID: "srv-1",
	}
	ctx := NewContext()
	ctx.Deps = map[string]*models.Module{"cap": module}

	got := Substitute("{{deps.cap.name}}", ctx)
	if strings.Contains(got, "{{deps.") {
		t.Errorf("substitution left a deps alias in place: %q", got)
	}
	if !strings.Contains(got, "{{name}}") {
		t.Errorf("expected injected value preserved verbatim, got %q", got)
	}
}
