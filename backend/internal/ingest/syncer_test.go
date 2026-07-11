package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/decklist"
)

func strptr(s string) *string { return &s }

// hashNamesV1 is the superseded fingerprint, kept here only so the versioning
// test can prove the new one produces a different value for the same list.
func hashNamesV1(names []string) string {
	norm := make([]string, len(names))
	for i, n := range names {
		norm[i] = strings.ToLower(strings.TrimSpace(n))
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:])
}

func TestPoolEntriesFromList(t *testing.T) {
	cases := []struct {
		name string
		in   *string
		want []string
	}{
		{"nil", nil, nil},
		{"empty", strptr("   \n\n"), nil},
		{
			"quantities and annotations",
			strptr("1 Sol Ring\n2x Lightning Bolt\nMana Crypt (LEA) 270\n// a comment\n"),
			[]string{"Lightning Bolt", "Mana Crypt", "Sol Ring"},
		},
		{
			"excludes sideboard",
			strptr("1 Sol Ring\nSideboard\n1 Pyroblast\n"),
			[]string{"Sol Ring"},
		},
		{
			"dedupes case-insensitively",
			strptr("1 Sol Ring\n1 sol ring\n"),
			[]string{"Sol Ring"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, _ := poolEntriesFromList(tc.in)
			got := make([]string, len(entries))
			for i, e := range entries {
				got[i] = e.Name
			}
			sort.Strings(got)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// The printing is carried through to the pool entry, and the parse stats account
// for the lines that never became entries.
func TestPoolEntriesCarryPrintingAndStats(t *testing.T) {
	list := strptr("1 Balance (PLST) EMA-2\n1 Sol Ring\n\n// note\nSideboard\n1 Pyroblast (ICE) 76\n")
	entries, st := poolEntriesFromList(list)

	if len(entries) != 2 {
		t.Fatalf("got %d main entries, want 2", len(entries))
	}
	if entries[0].SetCode != "PLST" || entries[0].Collector != "EMA-2" {
		t.Errorf("Balance printing = %q/%q, want PLST/EMA-2", entries[0].SetCode, entries[0].Collector)
	}
	if entries[1].SetCode != "" || entries[1].Collector != "" {
		t.Errorf("Sol Ring should carry no printing, got %q/%q", entries[1].SetCode, entries[1].Collector)
	}

	// The sideboarded card is a parsed entry that is NOT in the pool. That gap is
	// exactly the kind of silent loss the stats exist to expose.
	if st.Entries != 3 || st.Main != 2 || st.Side != 1 {
		t.Errorf("stats = %+v, want 3 entries / 2 main / 1 side", st)
	}
	if st.Headers != 1 || st.Comments != 1 || st.Annotated != 2 {
		t.Errorf("stats = %+v, want 1 header / 1 comment / 2 annotated", st)
	}
}

// The resolver version is folded into the hash, so an upgrade invalidates every
// stored content_hash. Without this the unchanged-list short-circuit would skip
// the resolve forever and keep serving the pool the old resolver built.
func TestHashEntriesIsVersioned(t *testing.T) {
	entries := []decklist.ParsedCard{{Name: "Sol Ring", Board: "main"}}
	got := hashEntries(entries)

	if got == hashNamesV1([]string{"Sol Ring"}) {
		t.Fatal("hash matches the v1 scheme; stored content_hash values would not be invalidated")
	}

	// The printing is part of the fingerprint: re-pointing a card at a different
	// printing has to trigger a re-resolve even though the name is unchanged.
	withPrinting := []decklist.ParsedCard{
		{Name: "Sol Ring", Board: "main", SetCode: "LEA", Collector: "270"},
	}
	if hashEntries(withPrinting) == got {
		t.Fatal("changing the printing did not change the hash")
	}

	// Order-independent.
	a := []decklist.ParsedCard{{Name: "Sol Ring"}, {Name: "Mana Crypt"}}
	b := []decklist.ParsedCard{{Name: "Mana Crypt"}, {Name: "Sol Ring"}}
	if hashEntries(a) != hashEntries(b) {
		t.Fatal("hash is order-dependent")
	}
}

func TestDescribeEntry(t *testing.T) {
	cases := []struct {
		in   decklist.ParsedCard
		want string
	}{
		{decklist.ParsedCard{Name: "Sol Ring"}, "Sol Ring"},
		{decklist.ParsedCard{Name: "Balance", SetCode: "PLST", Collector: "EMA-2"}, "Balance (PLST) EMA-2"},
		{decklist.ParsedCard{Name: "Sol Ring", SetCode: "LEA"}, "Sol Ring (LEA)"},
	}
	for _, tc := range cases {
		if got := describeEntry(tc.in); got != tc.want {
			t.Errorf("describeEntry(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
