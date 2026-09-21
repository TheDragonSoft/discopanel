package minecraft

import "testing"

func BenchmarkParseTPSFromOutput(b *testing.B) {
	output := "TPS from last 1m, 5m, 15m: 20.0, 20.0, 20.0"
	for i := 0; i < b.N; i++ {
		ParseTPSFromOutput(output)
	}
}

func BenchmarkParsePlayerListFromOutput(b *testing.B) {
	output := "There are 5 of a max 20 players online: player1, player2, player3, player4, player5"
	for i := 0; i < b.N; i++ {
		ParsePlayerListFromOutput(output)
	}
}

func BenchmarkStripMinecraftColors(b *testing.B) {
	output := "§aHello \x1b[31mWorld\x1b[0m"
	for i := 0; i < b.N; i++ {
		stripMinecraftColors(output)
	}
}
