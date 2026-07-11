package decklist

import "testing"

func find(cards []ParsedCard, name string) (ParsedCard, bool) {
	for _, c := range cards {
		if c.Name == name {
			return c, true
		}
	}
	return ParsedCard{}, false
}

func TestParseList(t *testing.T) {
	raw := `2 Lightning Bolt
1x Sol Ring
Black Lotus
// a comment
Mountain (NEO) 280

Sideboard
1 Pyroblast
SB: 2 Red Elemental Blast`

	cards := ParseList(raw)

	if c, ok := find(cards, "Lightning Bolt"); !ok || c.Quantity != 2 || c.Board != "main" {
		t.Fatalf("Lightning Bolt: %+v ok=%v", c, ok)
	}
	if c, ok := find(cards, "Sol Ring"); !ok || c.Quantity != 1 {
		t.Fatalf("Sol Ring quantity: %+v", c)
	}
	if c, ok := find(cards, "Black Lotus"); !ok || c.Quantity != 1 {
		t.Fatalf("bare name should be qty 1: %+v", c)
	}
	if c, ok := find(cards, "Mountain"); !ok || c.Board != "main" {
		t.Fatalf("set annotation should be stripped: %+v ok=%v", c, ok)
	}
	if c, ok := find(cards, "Pyroblast"); !ok || c.Board != "side" {
		t.Fatalf("Pyroblast should be sideboard: %+v", c)
	}
	if c, ok := find(cards, "Red Elemental Blast"); !ok || c.Board != "side" || c.Quantity != 2 {
		t.Fatalf("SB: prefix: %+v", c)
	}
}

func TestParseListMergesDuplicates(t *testing.T) {
	cards := ParseList("1 Forest\n1 Forest\n2 Forest")
	c, ok := find(cards, "Forest")
	if !ok || c.Quantity != 4 {
		t.Fatalf("duplicate names should merge quantities: %+v", c)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 merged entry, got %d", len(cards))
	}
}

// The printing annotation is captured, not discarded. Every shape here is taken
// verbatim from a real 540-card cube export; the collector numbers are the whole
// reason the pattern cannot be [\w-]+.
func TestSplitAnnotation(t *testing.T) {
	cases := []struct {
		in                       string
		name, setCode, collector string
	}{
		{"Balance (PLST) EMA-2", "Balance", "PLST", "EMA-2"},
		{"Adeline, Resplendent Cathar (PMID) 1p", "Adeline, Resplendent Cathar", "PMID", "1p"},
		{"Jace, Wielder of Mysteries (WAR) 54★", "Jace, Wielder of Mysteries", "WAR", "54★"},
		{"Hymn to Tourach (FEM) 38a", "Hymn to Tourach", "FEM", "38a"},
		{"Black Lotus (2ED) 233", "Black Lotus", "2ED", "233"},
		{"Bonecrusher Giant / Stomp (ELD) 115", "Bonecrusher Giant / Stomp", "ELD", "115"},
		{"Lightning Bolt (2X2) 117 *F*", "Lightning Bolt", "2X2", "117"},

		// No annotation at all.
		{"Sol Ring", "Sol Ring", "", ""},
		// Set with no collector number.
		{"Sol Ring (LEA)", "Sol Ring", "LEA", ""},
		// A name that really contains parentheses survives: the old parser cut at the
		// first " (" and would have truncated this to "Erase".
		{"Erase (Not the Urza's Legacy One)", "Erase (Not the Urza's Legacy One)", "", ""},
	}
	for _, tc := range cases {
		name, set, collector := splitAnnotation(tc.in)
		if name != tc.name || set != tc.setCode || collector != tc.collector {
			t.Errorf("splitAnnotation(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.in, name, set, collector, tc.name, tc.setCode, tc.collector)
		}
	}
}

func TestParseListCarriesPrinting(t *testing.T) {
	cards := ParseList("1 Balance (PLST) EMA-2\n1 Sol Ring")

	c, ok := find(cards, "Balance")
	if !ok || c.SetCode != "PLST" || c.Collector != "EMA-2" {
		t.Fatalf("Balance printing not carried: %+v ok=%v", c, ok)
	}
	if c, ok := find(cards, "Sol Ring"); !ok || c.SetCode != "" || c.Collector != "" {
		t.Fatalf("unannotated card should carry no printing: %+v", c)
	}
}

// A stray section header routes the rest of the list off the main board. That
// loss happens before any Scryfall call, so it can never surface as an
// unresolved name — the stats are the only way to see it.
func TestParseListStatsExposeSilentLosses(t *testing.T) {
	_, st := ParseListStats("1 Sol Ring\n1 Mana Crypt\nMaybeboard\n1 Black Lotus\n1 Sol Ring\n")

	if st.Main != 2 {
		t.Errorf("Main = %d, want 2", st.Main)
	}
	// Everything after the header, not just the next line, is routed away — which
	// is exactly how one stray word amputates the tail of a 540-card cube.
	if st.Maybe != 2 {
		t.Errorf("Maybe = %d, want 2 (Black Lotus and the second Sol Ring)", st.Maybe)
	}
	if st.Headers != 1 {
		t.Errorf("Headers = %d, want 1", st.Headers)
	}
	// The trailing "1 Sol Ring" lands on the maybeboard, so it does NOT merge with
	// the mainboard one — merges are per-board.
	if st.Merged != 0 {
		t.Errorf("Merged = %d, want 0", st.Merged)
	}
}

// The header check runs before the quantity strip, so a bare "Commander" is a
// section header while "1 Commander" is a card. Pinned because the ordering is
// load-bearing and easy to break.
func TestParseListSectionHeaderOrdering(t *testing.T) {
	cards := ParseList("Commander\n1 Sol Ring")
	if len(cards) != 1 || cards[0].Name != "Sol Ring" || cards[0].Board != "main" {
		t.Fatalf("bare 'Commander' should be a header: %+v", cards)
	}

	cards = ParseList("1 Commander")
	if len(cards) != 1 || cards[0].Name != "Commander" {
		t.Fatalf("'1 Commander' should be a card: %+v", cards)
	}
}
