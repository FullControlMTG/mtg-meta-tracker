package domain

import "testing"

func TestParseColorIdentity(t *testing.T) {
	ci := ParseColorIdentity([]string{"U", "R"})
	if ci != ColorIdentity(Blue|Red) {
		t.Fatalf("got %d want %d", ci, Blue|Red)
	}
	if ci.Count() != 2 {
		t.Fatalf("count = %d want 2", ci.Count())
	}
	if ci.String() != "UR" {
		t.Fatalf("string = %q want UR", ci.String())
	}
}

func TestColorlessAndOrdering(t *testing.T) {
	if ParseColorIdentity(nil).String() != "C" {
		t.Fatal("empty identity should render C")
	}
	ci := ParseColorIdentity([]string{"G", "W", "B"})
	if ci.String() != "WBG" {
		t.Fatalf("string = %q want WBG", ci.String())
	}
}

func TestInferDeckIdentity(t *testing.T) {
	deck := InferDeckIdentity([]ColorIdentity{
		ParseColorIdentity([]string{"W"}),
		ParseColorIdentity([]string{"U"}),
		ParseColorIdentity(nil),
	})
	if deck.String() != "WU" {
		t.Fatalf("deck identity = %q want WU", deck.String())
	}
}
