package strmatch

import (
	"testing"
)

func TestScore(t *testing.T) {
	s := Score("hello", "hello world")
	if s <= 0 {
		t.Errorf("Expected score > 0")
	}
}

func TestBest(t *testing.T) {
	m, ok := Best("forge", []string{"neoforge", "forge", "fabric"})
	if !ok || m.Value != "forge" {
		t.Errorf("Expected exact match 'forge', got %v", m)
	}
}
