package httpapi

import (
	"errors"
	"net/http"
	"testing"
)

const (
	idA = "3f1e6f6e-0b8a-4a4a-9c1a-6f0d5b8a1111"
	idB = "3f1e6f6e-0b8a-4a4a-9c1a-6f0d5b8a2222"
	idC = "3f1e6f6e-0b8a-4a4a-9c1a-6f0d5b8a3333"
)

func TestComboRequestParse(t *testing.T) {
	cases := []struct {
		desc      string
		req       comboRequest
		wantCards int
		wantName  string
		wantDesc  bool
		wantErr   int // 0 for success, else the expected status
	}{
		{
			desc:      "a pair, trimmed",
			req:       comboRequest{Name: "  Thoracle  ", CardIDs: []string{idA, " " + idB}},
			wantCards: 2,
			wantName:  "Thoracle",
		},
		{
			desc:      "blank description is left unset, not stored as empty",
			req:       comboRequest{Name: "Thoracle", Description: "   ", CardIDs: []string{idA, idB}},
			wantCards: 2,
			wantName:  "Thoracle",
		},
		{
			desc:      "description survives",
			req:       comboRequest{Name: "Thoracle", Description: "wins on the trigger", CardIDs: []string{idA, idB}},
			wantCards: 2,
			wantName:  "Thoracle",
			wantDesc:  true,
		},
		{
			// Naming a card twice is a slip in the form, and the two rows it would
			// insert collide on combo_cards' primary key.
			desc:      "duplicates collapse",
			req:       comboRequest{Name: "Thoracle", CardIDs: []string{idA, idB, idA, idC}},
			wantCards: 3,
			wantName:  "Thoracle",
		},
		{
			desc:    "a duplicate cannot stand in for a second card",
			req:     comboRequest{Name: "Thoracle", CardIDs: []string{idA, idA}},
			wantErr: http.StatusBadRequest,
		},
		{
			desc:    "one card is not a combo",
			req:     comboRequest{Name: "Thoracle", CardIDs: []string{idA}},
			wantErr: http.StatusBadRequest,
		},
		{
			desc:    "name required",
			req:     comboRequest{Name: "  ", CardIDs: []string{idA, idB}},
			wantErr: http.StatusBadRequest,
		},
		{
			desc:    "non-uuid card",
			req:     comboRequest{Name: "Thoracle", CardIDs: []string{idA, "Demonic Consultation"}},
			wantErr: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			in, err := tc.req.parse()
			if tc.wantErr != 0 {
				var ae apiError
				if err == nil {
					t.Fatalf("parse() = %+v, want error", in)
				}
				if !errors.As(err, &ae) || ae.status != tc.wantErr {
					t.Fatalf("parse() error = %v, want status %d", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse(): %v", err)
			}
			if in.Name != tc.wantName {
				t.Errorf("name = %q, want %q", in.Name, tc.wantName)
			}
			if len(in.CardIDs) != tc.wantCards {
				t.Errorf("cards = %d, want %d", len(in.CardIDs), tc.wantCards)
			}
			if (in.Description != nil) != tc.wantDesc {
				t.Errorf("description = %v, want set: %v", in.Description, tc.wantDesc)
			}
		})
	}
}

// The ceiling is enforced on the parsed, de-duplicated list.
func TestComboRequestParseCeiling(t *testing.T) {
	ids := make([]string, 0, maxComboCards+1)
	for i := 0; i <= maxComboCards; i++ {
		// Vary the last hex digit so every id is distinct.
		ids = append(ids, idA[:len(idA)-1]+string("0123456789abcdef"[i]))
	}
	if _, err := (comboRequest{Name: "Everything", CardIDs: ids}).parse(); err == nil {
		t.Fatalf("parse() accepted %d cards, want the %d ceiling enforced", len(ids), maxComboCards)
	}
}
