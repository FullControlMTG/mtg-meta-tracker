package ingest

import (
	"sort"
	"testing"
)

func strptr(s string) *string { return &s }

func TestPoolNamesFromList(t *testing.T) {
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
			got := poolNamesFromList(tc.in)
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
