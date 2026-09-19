package services

import (
	"reflect"
	"testing"
)

func TestParseWhitelistListOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []string
		ok       bool
	}{
		{
			name:     "vanilla empty whitelist",
			output:   "There are 0 whitelisted players",
			expected: nil,
			ok:       true,
		},
		{
			name:     "vanilla with names",
			output:   "There are 2 whitelisted players: Steve, Alex",
			expected: []string{"Steve", "Alex"},
			ok:       true,
		},
		{
			name:     "single name",
			output:   "There are 1 whitelisted players: Notch",
			expected: []string{"Notch"},
			ok:       true,
		},
		{
			name:     "alternative format",
			output:   "Whitelisted players: Steve",
			expected: []string{"Steve"},
			ok:       true,
		},
		{
			name:     "color codes stripped",
			output:   "§aThere are 2 whitelisted players: §bSteve§r, Alex",
			expected: []string{"Steve", "Alex"},
			ok:       true,
		},
		{
			name:     "none placeholder filtered",
			output:   "Whitelisted players: None",
			expected: []string{},
			ok:       true,
		},
		{
			name:     "unrelated output unparseable",
			output:   "Unknown command. Type \"/help\" for help.",
			expected: nil,
			ok:       false,
		},
		{
			name:     "whitelist mention without colon or count unparseable",
			output:   "Whitelist is turned off",
			expected: nil,
			ok:       false,
		},
		{
			name:     "malformed token makes output unparseable",
			output:   "There are 2 whitelisted players: Steve (offline), Alex",
			expected: nil,
			ok:       false,
		},
		{
			name:     "ansi escape codes stripped",
			output:   "\x1b[32mThere are 1 whitelisted players:\x1b[0m Steve",
			expected: []string{"Steve"},
			ok:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, ok := parseWhitelistListOutput(tt.output)
			if ok != tt.ok {
				t.Fatalf("parseWhitelistListOutput(%q) ok = %v, want %v", tt.output, ok, tt.ok)
			}
			if !reflect.DeepEqual(names, tt.expected) {
				t.Errorf("parseWhitelistListOutput(%q) names = %v, want %v", tt.output, names, tt.expected)
			}
		})
	}
}
