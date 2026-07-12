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

// nonland/land build a DeckColorCard from the color letters of a card's cost.
func nonland(qty int, cost ...string) DeckColorCard {
	return DeckColorCard{Colors: ParseColorIdentity(cost), IsLand: false, Quantity: qty}
}

func land(qty int, produces ...string) DeckColorCard {
	// A land's own cost is colorless; the colors it taps for are deliberately not
	// its `colors`, which is exactly the point of the land flag.
	_ = produces
	return DeckColorCard{Colors: 0, IsLand: true, Quantity: qty}
}

func TestInferDeckColors(t *testing.T) {
	cases := []struct {
		name       string
		cards      []DeckColorCard
		wantMain   string
		wantSplash string
	}{{
		// The reported bug: a Mox Sapphire taps for blue and a Hallowed Fountain is
		// blue in identity, but the deck never casts a blue card.
		name: "mana that produces blue does not make a deck blue",
		cards: []DeckColorCard{
			nonland(10, "G"), nonland(10, "W"), nonland(2), // 2 colorless artifacts
			nonland(1),        // Mox Sapphire: colorless cost
			land(4), land(13), // Hallowed Fountain, basics
		},
		wantMain: "WG", wantSplash: "C",
	}, {
		name: "a color under 10% of the nonlands is a splash",
		cards: []DeckColorCard{
			nonland(13, "G"), nonland(10, "W"), nonland(2, "R"), land(17),
		},
		wantMain: "WG", wantSplash: "R",
	}, {
		name: "a color at exactly 10% of the nonlands is not a splash",
		cards: []DeckColorCard{
			nonland(18, "U"), nonland(2, "B"), land(17),
		},
		wantMain: "UB", wantSplash: "C",
	}, {
		// Copies count, so 4 Lightning Bolts are 4 red cards, not one.
		name: "quantity is weighted",
		cards: []DeckColorCard{
			nonland(30, "G"), nonland(4, "R"), land(17),
		},
		wantMain: "RG", wantSplash: "C",
	}, {
		name: "a gold card counts for each of its colors",
		cards: []DeckColorCard{
			nonland(10, "W", "U"), nonland(10, "U"), land(17),
		},
		wantMain: "WU", wantSplash: "C",
	}, {
		// Nothing clears 10% of a 40-card artifact deck, but it is not a colorless
		// deck: its best-represented color is promoted rather than splashed away.
		name: "the top color is promoted when nothing clears the bar",
		cards: []DeckColorCard{
			nonland(36), nonland(3, "W"), nonland(1, "U"), land(17),
		},
		wantMain: "W", wantSplash: "U",
	}, {
		name: "a tie promotes both colors",
		cards: []DeckColorCard{
			nonland(36), nonland(2, "W"), nonland(2, "R"),
		},
		wantMain: "WR", wantSplash: "C",
	}, {
		name:     "a deck of nothing but lands is colorless",
		cards:    []DeckColorCard{land(40)},
		wantMain: "C", wantSplash: "C",
	}, {
		name:     "no cards at all",
		cards:    nil,
		wantMain: "C", wantSplash: "C",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferDeckColors(tc.cards)
			if got.Main.String() != tc.wantMain || got.Splash.String() != tc.wantSplash {
				t.Fatalf("main/splash = %q/%q want %q/%q",
					got.Main.String(), got.Splash.String(), tc.wantMain, tc.wantSplash)
			}
			if got.Main&got.Splash != 0 {
				t.Fatalf("main %q and splash %q overlap", got.Main, got.Splash)
			}
		})
	}
}
