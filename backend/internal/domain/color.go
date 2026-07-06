package domain

import "strings"

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
