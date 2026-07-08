// Package decklist parses raw pasted decklists and resolves card names.
package decklist

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// ParsedCard is one entry from a raw decklist, before resolution.
type ParsedCard struct {
	Name     string
	Quantity int
	Board    string
}

// Leading "2 ", "2x ", "x2 " quantity prefixes.
var qtyRe = regexp.MustCompile(`^(?:(\d+)\s*x?|x\s*(\d+))\s+`)

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
	board := domain.BoardMain
	index := map[string]int{} // name|board -> position in out
	var out []ParsedCard

	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
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
			continue
		}

		// Comment lines.
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
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

		name := cleanName(line)
		if name == "" {
			continue
		}

		key := strings.ToLower(name) + "|" + lineBoard
		if pos, ok := index[key]; ok {
			out[pos].Quantity += qty
			continue
		}
		index[key] = len(out)
		out = append(out, ParsedCard{Name: name, Quantity: qty, Board: lineBoard})
	}
	return out
}

func stripSideboardPrefix(line string) (string, bool) {
	if len(line) >= 3 && strings.EqualFold(line[:3], "sb:") {
		return strings.TrimSpace(line[3:]), true
	}
	return line, false
}

// cleanName drops trailing set/collector annotations like "(NEO) 123" or
// "*F*" foil markers that some exports append after the card name.
func cleanName(s string) string {
	if i := strings.Index(s, " ("); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " *"); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
