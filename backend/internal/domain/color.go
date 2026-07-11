package domain

import (
	"regexp"
	"strings"
)

// Color identity bitset: W=1 U=2 B=4 R=8 G=16 (colorless=0).
type Color uint8

const (
	White Color = 1 << iota
	Blue
	Black
	Red
	Green
)

type ColorIdentity uint8

var symbolToColor = map[byte]Color{
	'W': White, 'U': Blue, 'B': Black, 'R': Red, 'G': Green,
}

func ParseColorIdentity(symbols []string) ColorIdentity {
	var ci ColorIdentity
	for _, s := range symbols {
		if s == "" {
			continue
		}
		if c, ok := symbolToColor[strings.ToUpper(s)[0]]; ok {
			ci |= ColorIdentity(c)
		}
	}
	return ci
}

func (ci ColorIdentity) Merge(other ColorIdentity) ColorIdentity { return ci | other }

const allColors = ColorIdentity(White | Blue | Black | Red | Green)

var basicLandTypeRE = map[*regexp.Regexp]Color{
	regexp.MustCompile(`(?i)\bplains\b`):   White,
	regexp.MustCompile(`(?i)\bisland\b`):   Blue,
	regexp.MustCompile(`(?i)\bswamp\b`):    Black,
	regexp.MustCompile(`(?i)\bmountain\b`): Red,
	regexp.MustCompile(`(?i)\bforest\b`):   Green,
}

var (
	landRE      = regexp.MustCompile(`(?i)\bland\b`)
	basicLandRE = regexp.MustCompile(`(?i)\bbasic land\b`)
)

func basicLandTypeColors(text string) ColorIdentity {
	var ci ColorIdentity
	for re, c := range basicLandTypeRE {
		if re.MatchString(text) {
			ci |= ColorIdentity(c)
		}
	}
	return ci
}

// GroupColors is the WUBRG bitset a card is *displayed* under, which is not the
// same question as its color identity.
//
// A nonland groups by its casting cost, so a mana rock whose only colored mana
// symbols sit in an activated ability (an Azorius Signet, a Talisman) groups with
// the artifacts rather than with the gold cards.
//
// A land groups by every color it is *related to*, since the colors a land cares
// about are rarely in its mana cost and often not in its identity either: the
// colors it can tap for, the basic land types it has or fetches (Flooded Strand
// searching for "a Plains or Island card" is an Azorius card), and, for anything
// fetching a plain "basic land", all five (Prismatic Vista sits with the gold).
//
// `produced` is Scryfall's produced_mana; its `C` symbol is not a color and drops
// out in ParseColorIdentity.
func GroupColors(typeLine, oracleText string, colors, identity ColorIdentity, produced []string) ColorIdentity {
	if !landRE.MatchString(typeLine) {
		return colors
	}
	ci := identity.
		Merge(basicLandTypeColors(typeLine)).
		Merge(basicLandTypeColors(oracleText)).
		Merge(ParseColorIdentity(produced))
	// Only the oracle text — a basic Plains' type line reads "Basic Land — Plains",
	// and it is a white card, not a five-color one.
	if basicLandRE.MatchString(oracleText) {
		ci = ci.Merge(allColors)
	}
	return ci
}

func (ci ColorIdentity) Count() int {
	n := 0
	for _, c := range []Color{White, Blue, Black, Red, Green} {
		if ci&ColorIdentity(c) != 0 {
			n++
		}
	}
	return n
}

func (ci ColorIdentity) String() string {
	if ci == 0 {
		return "C"
	}
	var b strings.Builder
	for _, p := range []struct {
		c Color
		r byte
	}{{White, 'W'}, {Blue, 'U'}, {Black, 'B'}, {Red, 'R'}, {Green, 'G'}} {
		if ci&ColorIdentity(p.c) != 0 {
			b.WriteByte(p.r)
		}
	}
	return b.String()
}

// v1 strategy is pure OR; see docs/DESIGN.md §8 for how it grows.
func InferDeckIdentity(cardIdentities []ColorIdentity) ColorIdentity {
	var deck ColorIdentity
	for _, ci := range cardIdentities {
		deck = deck.Merge(ci)
	}
	return deck
}
