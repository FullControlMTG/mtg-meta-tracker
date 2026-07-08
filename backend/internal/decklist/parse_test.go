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
