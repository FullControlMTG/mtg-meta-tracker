// Package decklist parses raw pasted decklists and resolves card names.
package decklist

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// ParsedCard is one entry from a raw decklist, before resolution. SetCode and
// Collector carry the "(ELD) 115" annotation most exports append; both are ""
// for a bare name. Name keeps the pasted spelling, including a "Front / Back"
// slash — reducing that to a front face is the Scryfall client's job.
type ParsedCard struct {
	Name      string
	Quantity  int
	Board     string
	SetCode   string
	Collector string
	Line      int
}

// Stats accounts for every line of a raw list. Cards can go missing before we
// ever reach Scryfall — a stray section header routes the rest of the list off
// the main board, and same-name entries merge — and neither loss can show up as
// an unresolved name. These counters are what make those drops visible.
type Stats struct {
	Lines     int
	Blank     int
	Comments  int
	Headers   int
	Entries   int
	Merged    int
	Main      int
	Side      int
	Maybe     int
	Annotated int
}

// Leading "2 ", "2x ", "x2 " quantity prefixes.
var qtyRe = regexp.MustCompile(`^(?:(\d+)\s*x?|x\s*(\d+))\s+`)

// Trailing "*F*" / "*Foil*" markers.
var markerRe = regexp.MustCompile(`\s+\*[^*]*\*\s*$`)

// A trailing "(SET) 123" annotation. End-anchored so a card whose name really
// contains parentheses survives. The collector number is \S+ rather than \w+
// because real ones include "EMA-2", "1p", "38a" and "54★".
var annotationRe = regexp.MustCompile(`\s+\(([A-Za-z0-9]{2,6})\)\s+(\S+)\s*$`)

// A trailing "(SET)" with no collector number.
var setOnlyRe = regexp.MustCompile(`\s+\(([A-Za-z0-9]{2,6})\)\s*$`)

// A line that only names a section, e.g. "Sideboard", "// Maybeboard", "SB:".
func sectionBoard(line string) (string, bool) {
	s := strings.ToLower(strings.Trim(line, " \t:/#"))
	switch {
	case s == "sideboard" || s == "sb":
		return domain.BoardSide, true
	case s == "maybeboard" || s == "maybe" || s == "considering":
		return domain.BoardMaybe, true
	case s == "deck" || s == "mainboard" || s == "main" || s == "commander":
		return domain.BoardMain, true
	}
	return "", false
}

// ParseList turns raw list text into card entries. It understands quantity
// prefixes ("2 Foo", "2x Foo", bare "Foo" = 1), section headers
// (Sideboard/Maybeboard/…), and "SB:" line prefixes. Comment lines (// or #)
// and blanks are skipped. Duplicate (name, board) entries are merged.
func ParseList(raw string) []ParsedCard {
	cards, _ := ParseListStats(raw)
	return cards
}

// ParseListStats is ParseList plus a per-line accounting of what happened.
func ParseListStats(raw string) ([]ParsedCard, Stats) {
	board := domain.BoardMain
	index := map[string]int{} // name|board -> position in out
	var out []ParsedCard
	var st Stats

	for i, rawLine := range strings.Split(raw, "\n") {
		st.Lines++
		lineNo := i + 1

		line := strings.TrimSpace(rawLine)
		if line == "" {
			st.Blank++
			continue
		}

		lineBoard := board

		// "SB:" prefix marks a single sideboard entry without changing section.
		if rest, ok := stripSideboardPrefix(line); ok {
			lineBoard = domain.BoardSide
			line = rest
		}

		// Whole-line section header switches the active board.
		if b, ok := sectionBoard(line); ok {
			board = b
			st.Headers++
			continue
		}

		// Comment lines.
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			st.Comments++
			continue
		}

		qty := 1
		if m := qtyRe.FindStringSubmatch(line); m != nil {
			num := m[1]
			if num == "" {
				num = m[2]
			}
			if n, err := strconv.Atoi(num); err == nil && n > 0 {
				qty = n
			}
			line = strings.TrimSpace(line[len(m[0]):])
		}

		name, set, collector := splitAnnotation(line)
		if name == "" {
			continue
		}
		if set != "" {
			st.Annotated++
		}

		// Merged on name alone, not on printing: decklist_cards is keyed by
		// (decklist_id, card_name, board), so two printings of one card cannot be
		// two rows.
		key := strings.ToLower(name) + "|" + lineBoard
		if pos, ok := index[key]; ok {
			out[pos].Quantity += qty
			st.Merged++
			continue
		}
		index[key] = len(out)
		out = append(out, ParsedCard{
			Name:      name,
			Quantity:  qty,
			Board:     lineBoard,
			SetCode:   set,
			Collector: collector,
			Line:      lineNo,
		})

		st.Entries++
		switch lineBoard {
		case domain.BoardSide:
			st.Side++
		case domain.BoardMaybe:
			st.Maybe++
		default:
			st.Main++
		}
	}
	return out, st
}

func stripSideboardPrefix(line string) (string, bool) {
	if len(line) >= 3 && strings.EqualFold(line[:3], "sb:") {
		return strings.TrimSpace(line[3:]), true
	}
	return line, false
}

// splitAnnotation peels the trailing "(SET) 123" printing annotation and any
// "*F*" foil marker off a line, returning the bare name plus whatever printing
// it named. A line with no annotation yields empty set and collector.
func splitAnnotation(s string) (name, set, collector string) {
	s = strings.TrimSpace(markerRe.ReplaceAllString(s, ""))

	if m := annotationRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(s[:len(s)-len(m[0])]), m[1], m[2]
	}
	if m := setOnlyRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(s[:len(s)-len(m[0])]), m[1], ""
	}
	return strings.TrimSpace(s), "", ""
}
