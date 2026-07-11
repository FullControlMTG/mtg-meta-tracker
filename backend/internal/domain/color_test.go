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

func TestGroupColors(t *testing.T) {
	cases := []struct {
		name     string
		typeLine string
		oracle   string
		colors   []string
		identity []string
		produced []string
		want     string
	}{
		{
			name: "Lightning Bolt", typeLine: "Instant",
			colors: []string{"R"}, identity: []string{"R"}, want: "R",
		},
		{
			// The point of grouping by cost: a mana rock's colored symbols live in
			// an activated ability, so its identity is WU but it is an artifact.
			name: "Azorius Signet", typeLine: "Artifact",
			oracle:   "{T}: Add {C}. {1}, {T}: Add {W}{U}.",
			identity: []string{"W", "U"}, produced: []string{"C", "W", "U"}, want: "C",
		},
		{
			name: "Sol Ring", typeLine: "Artifact",
			oracle: "{T}: Add {C}{C}.", produced: []string{"C"}, want: "C",
		},
		{
			// A fetch land has no color identity at all; its colors are the basic
			// land types it searches for.
			name: "Flooded Strand", typeLine: "Land",
			oracle: "{T}, Pay 1 life, Sacrifice Flooded Strand: Search your library " +
				"for a Plains or Island card, put it onto the battlefield, then shuffle.",
			want: "WU",
		},
		{
			name: "Prismatic Vista", typeLine: "Land",
			oracle: "{T}, Pay 1 life, Sacrifice Prismatic Vista: Search your library " +
				"for a basic land card, put it onto the battlefield, then shuffle.",
			want: "WUBRG",
		},
		{
			// "Basic Land" in the type line must not read as the "basic land card" a
			// fetch land searches for — a Plains is a white card, not a five-color one.
			name: "Plains", typeLine: "Basic Land — Plains",
			oracle: "({T}: Add {W}.)", produced: []string{"W"}, want: "W",
		},
		{
			name: "Sacred Foundry", typeLine: "Land — Mountain Plains",
			oracle:   "As Sacred Foundry enters, you may pay 2 life. If you don't, it enters tapped.",
			identity: []string{"R", "W"}, produced: []string{"R", "W"}, want: "WR",
		},
		{
			// Taps for any color, but Scryfall gives it no identity — produced_mana
			// is the only thing that knows it belongs with the gold lands.
			name: "City of Brass", typeLine: "Land",
			oracle:   "Whenever City of Brass becomes tapped, it deals 1 damage to you.\n{T}: Add one mana of any color.",
			produced: []string{"W", "U", "B", "R", "G"}, want: "WUBRG",
		},
		{
			name: "Celestial Colonnade", typeLine: "Land",
			oracle:   "{T}: Add {W} or {U}.",
			identity: []string{"W", "U"}, produced: []string{"W", "U"}, want: "WU",
		},
		{
			name: "Wasteland", typeLine: "Land",
			oracle:   "{T}: Add {C}.\n{T}, Sacrifice Wasteland: Destroy target nonbasic land.",
			produced: []string{"C"}, want: "C",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupColors(tc.typeLine, tc.oracle,
				ParseColorIdentity(tc.colors), ParseColorIdentity(tc.identity), tc.produced)
			if got.String() != tc.want {
				t.Fatalf("group colors = %q want %q", got.String(), tc.want)
			}
		})
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
