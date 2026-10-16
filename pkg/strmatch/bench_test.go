package strmatch

import "testing"
import "fmt"

func BenchmarkBest(b *testing.B) {
	candidates := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		candidates[i] = fmt.Sprintf("some-candidate-%d", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Best("candidate-500", candidates)
	}
}
