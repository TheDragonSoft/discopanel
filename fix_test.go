package main

import (
	"regexp"
	"testing"
)

var globalRe = regexp.MustCompile(`§.`)

func withMustCompile(text string) string {
	re := regexp.MustCompile(`§.`)
	return re.ReplaceAllString(text, "")
}

func withGlobalCompile(text string) string {
	return globalRe.ReplaceAllString(text, "")
}

func BenchmarkWithMustCompile(b *testing.B) {
	output := "§aHello \x1b[31mWorld\x1b[0m"
	for i := 0; i < b.N; i++ {
		withMustCompile(output)
	}
}

func BenchmarkWithGlobalCompile(b *testing.B) {
	output := "§aHello \x1b[31mWorld\x1b[0m"
	for i := 0; i < b.N; i++ {
		withGlobalCompile(output)
	}
}
