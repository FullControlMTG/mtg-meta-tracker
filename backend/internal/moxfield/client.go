// Package moxfield fetches a cube's card list from a Moxfield deck.
//
// The Moxfield API is unofficial; this is deliberately a thin adapter so it can
// be swapped for manual paste / CSV import if access changes (see docs/DESIGN.md
// §5). A cube is stored on Moxfield as a deck, so we read the deck's boards.
package moxfield

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

type Client struct {
	http      *http.Client
	userAgent string
}

func New(userAgent string) *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, userAgent: userAgent}
}

var publicIDRe = regexp.MustCompile(`decks/(?:all/)?([A-Za-z0-9_-]+)`)

// ParsePublicID extracts the public id from a Moxfield deck URL, or returns the
// input unchanged if it's already a bare id.
func ParsePublicID(urlOrID string) string {
	if m := publicIDRe.FindStringSubmatch(urlOrID); m != nil {
		return m[1]
	}
	return urlOrID
}

type deckResponse struct {
	Name   string `json:"name"`
	Boards map[string]struct {
		Cards map[string]struct {
			Quantity int `json:"quantity"`
			Card     struct {
				Name string `json:"name"`
			} `json:"card"`
		} `json:"cards"`
	} `json:"boards"`
}

// FetchCubeCardNames returns the distinct mainboard card names for a deck.
func (c *Client) FetchCubeCardNames(ctx context.Context, publicID string) ([]string, error) {
	url := fmt.Sprintf("https://api2.moxfield.com/v3/decks/all/%s", publicID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("moxfield status %d for deck %s", res.StatusCode, publicID)
	}

	var deck deckResponse
	if err := json.NewDecoder(res.Body).Decode(&deck); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var names []string
	for boardName, board := range deck.Boards {
		if boardName != "mainboard" {
			continue
		}
		for _, entry := range board.Cards {
			n := entry.Card.Name
			if n == "" {
				continue
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no mainboard cards found for deck %s", publicID)
	}
	return names, nil
}
