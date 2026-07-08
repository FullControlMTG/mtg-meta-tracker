package moxfield

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
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

func (c *Client) FetchCubeCardNames(ctx context.Context, publicID string) ([]string, error) {
	url := fmt.Sprintf("https://api2.moxfield.com/v3/decks/all/%s", publicID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", "https://www.moxfield.com/")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		// Moxfield sits behind Cloudflare and returns 403 for requests it
		// doesn't recognize as an approved client. Capture the body and a few
		// diagnostic headers so the failure is actionable from the logs.
		snippet := readBodySnippet(res.Body)
		return nil, fmt.Errorf(
			"moxfield status %d for deck %s (url=%s, server=%q, cf-ray=%q, retry-after=%q, body=%q)",
			res.StatusCode, publicID, url,
			res.Header.Get("Server"), res.Header.Get("Cf-Ray"), res.Header.Get("Retry-After"),
			snippet,
		)
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

// readBodySnippet returns a bounded, single-line excerpt of an error response
// body suitable for embedding in a log line. Cloudflare block pages are large
// HTML documents, so we cap the read and collapse whitespace.
func readBodySnippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 2<<10))
	return strings.Join(strings.Fields(string(b)), " ")
}
