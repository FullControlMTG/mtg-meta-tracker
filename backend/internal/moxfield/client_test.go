package moxfield

import "testing"

func TestParsePublicID(t *testing.T) {
	cases := map[string]string{
		"https://moxfield.com/decks/tycRs35hF0SSnOA96kk1ug": "tycRs35hF0SSnOA96kk1ug",
		"https://www.moxfield.com/decks/abc123/":            "abc123",
		"tycRs35hF0SSnOA96kk1ug":                            "tycRs35hF0SSnOA96kk1ug",
	}
	for in, want := range cases {
		if got := ParsePublicID(in); got != want {
			t.Errorf("ParsePublicID(%q) = %q, want %q", in, got, want)
		}
	}
}
