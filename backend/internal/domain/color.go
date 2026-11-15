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

// The five colors in WUBRG order, the order everything iterates and prints them in.
var colorOrder = []Color{White, Blue, Black, Red, Green}

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

// CardFace is the slice of a Scryfall card face the color rules read.
type CardFace struct {
	ManaCost string   `json:"mana_cost"`
	Colors   []string `json:"colors"`
}

// PrintedColors is what Scryfall says about a card's colors: its top-level
// `colors` and each face's. Scryfall omits the top-level field on a double-faced
// card and reports the colors per face, so taking `colors` at face value files
// every DFC under colorless; both rules below fall back to the faces.
//
// There are two rules because there are two questions, and neither is the card's
// color identity (which also counts the mana symbols in its rules text, so an
// Azorius Signet is WU):
//
//   - CardColors — what colors the card *is*, every face counted. How a card is
//     sorted and grouped.
//   - CastColors — what colors a deck has to *cast* to play it. How a deck's
//     colors are inferred.
//
// They differ on a transform card: Etali, Primal Conqueror is cast for red, but
// its back face is green, so it is a red-green card that a mono-red deck can
// still run without splashing green.
type PrintedColors struct {
	Colors []string   `json:"colors"`
	Faces  []CardFace `json:"card_faces"`
}

// CardColors is the union of the colors on every face of the card, castable or
// not: Etali, Primal Conqueror (red front, green back) is a red-green card, and
// sorts with the gold cards rather than with the red ones.
func (p PrintedColors) CardColors() ColorIdentity {
	ci := ParseColorIdentity(p.Colors)
	for _, f := range p.Faces {
		ci = ci.Merge(ParseColorIdentity(f.Colors))
	}
	return ci
}

// CastColors is the colors of the card's casting costs: only faces that have a
// mana cost count, since a face you can't cast can't make a deck its colors. That
// keeps both halves of a split card, an adventure, and a modal DFC, and drops the
// back of a transform card, which is turned up rather than cast: Tamiyo,
// Inquisitive Student is a blue card to cast whose back, Tamiyo, Seasoned Scholar,
// is GU, and unioning it made every UW deck running her a deck with a green splash.
func (p PrintedColors) CastColors() ColorIdentity {
	if len(p.Colors) > 0 {
		return ParseColorIdentity(p.Colors)
	}
	var ci ColorIdentity
	for _, f := range p.Faces {
		if f.ManaCost == "" {
			continue
		}
		ci = ci.Merge(ParseColorIdentity(f.Colors))
	}
	return ci
}

// GroupColors is the WUBRG bitset a card is *displayed* under, which is not the
// same question as its color identity.
//
// A nonland groups by its card colors (PrintedColors.CardColors), so a mana rock
// whose only colored mana symbols sit in an activated ability (an Azorius Signet,
// a Talisman) groups with the artifacts rather than with the gold cards, and a
// transform card whose faces are two colors (Etali, Primal Conqueror) groups with
// the gold cards rather than under the color of its front.
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
	for _, c := range colorOrder {
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
	for i, c := range colorOrder {
		if ci&ColorIdentity(c) != 0 {
			b.WriteByte("WUBRG"[i])
		}
	}
	return b.String()
}

// SplashThreshold is the share of a deck's nonland cards a color has to reach to
// be one of the deck's colors rather than a splash.
const SplashThreshold = 0.10

// DeckColorCard is one main-board entry's contribution to a deck's colors: the
// colors of its *casting cost* (PrintedColors.CastColors, never color_identity),
// whether it is a land, and how many copies the list runs.
type DeckColorCard struct {
	Colors   ColorIdentity
	IsLand   bool
	Quantity int
}

// DeckColors splits a deck's colors into the ones it is built on and the ones it
// merely splashes.
type DeckColors struct {
	Main   ColorIdentity
	Splash ColorIdentity
}

// InferDeckColors derives a deck's colors from what it *casts*, not from what it
// can tap for: a color counts only when it appears in the cost of a nonland card.
// A Mox Sapphire is a colorless artifact and a Hallowed Fountain is a land, so
// neither makes a deck blue — only a blue pip on something's cost does.
//
// A color on fewer than SplashThreshold of the nonland cards is a splash, held
// apart from the deck's real colors: a Selesnya deck with two red cards in it is a
// GW deck splashing red, not a Naya deck. Copies count, so 4 Lightning Bolts weigh
// four cards' worth.
//
// A deck that plays colored cards is never colorless, so if nothing clears the
// threshold the best-represented color (or colors, on a tie) is promoted rather
// than everything being written off as a splash.
func InferDeckColors(cards []DeckColorCard) DeckColors {
	counts := map[Color]int{}
	total := 0
	for _, c := range cards {
		if c.IsLand || c.Quantity <= 0 {
			continue
		}
		total += c.Quantity
		for _, col := range colorOrder {
			if c.Colors&ColorIdentity(col) != 0 {
				counts[col] += c.Quantity
			}
		}
	}
	if total == 0 {
		return DeckColors{}
	}

	var out DeckColors
	best := 0
	for _, col := range colorOrder {
		n := counts[col]
		if n == 0 {
			continue
		}
		if n > best {
			best = n
		}
		if float64(n)/float64(total) >= SplashThreshold {
			out.Main = out.Main.Merge(ColorIdentity(col))
		} else {
			out.Splash = out.Splash.Merge(ColorIdentity(col))
		}
	}
	if out.Main == 0 && best > 0 {
		for _, col := range colorOrder {
			if counts[col] == best {
				out.Main = out.Main.Merge(ColorIdentity(col))
				out.Splash &^= ColorIdentity(col)
			}
		}
	}
	return out
}

// IsLandType reports whether a Scryfall type line is a land's.
func IsLandType(typeLine string) bool { return landRE.MatchString(typeLine) }
