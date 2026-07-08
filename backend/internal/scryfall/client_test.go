package scryfall

import "testing"

func TestFrontFace(t *testing.T) {
	cases := map[string]string{
		"Sol Ring": "Sol Ring",
		"Fable of the Mirror-Breaker / Reflection of Kiki-Jiki":  "Fable of the Mirror-Breaker",
		"Fable of the Mirror-Breaker // Reflection of Kiki-Jiki": "Fable of the Mirror-Breaker",
		"Life // Death":                                  "Life",
		"Bonecrusher Giant//Stomp":                       "Bonecrusher Giant",
		"Aang, Swift Savior / Aang and La, Ocean's Fury": "Aang, Swift Savior",
	}
	for in, want := range cases {
		if got := frontFace(in); got != want {
			t.Errorf("frontFace(%q) = %q, want %q", in, got, want)
		}
	}
}
