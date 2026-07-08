package scryfall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const (
	collectionURL = "https://api.scryfall.com/cards/collection"
	searchURL     = "https://api.scryfall.com/cards/search"
	batchSize     = 75
)

type Client struct {
	http        *http.Client
	userAgent   string
	minInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(userAgent string, minInterval time.Duration) *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		userAgent:   userAgent,
		minInterval: minInterval,
	}
}

// Scryfall asks for ~50-100ms spacing between requests.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.minInterval - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

type identifier struct {
	Name string `json:"name"`
}

type imageURIs struct {
	Small   string `json:"small"`
	Normal  string `json:"normal"`
	ArtCrop string `json:"art_crop"`
}

type scryCard struct {
	ID              string     `json:"id"`
	OracleID        string     `json:"oracle_id"`
	Name            string     `json:"name"`
	ManaCost        *string    `json:"mana_cost"`
	CMC             float64    `json:"cmc"`
	TypeLine        *string    `json:"type_line"`
	OracleText      *string    `json:"oracle_text"`
	Colors          []string   `json:"colors"`
	ColorIdentity   []string   `json:"color_identity"`
	Rarity          *string    `json:"rarity"`
	Layout          *string    `json:"layout"`
	Set             *string    `json:"set"`
	CollectorNumber *string    `json:"collector_number"`
	ImageURIs       *imageURIs `json:"image_uris"`
	CardFaces       []struct {
		ImageURIs *imageURIs `json:"image_uris"`
	} `json:"card_faces"`
}

func (c *Client) ResolveByNames(ctx context.Context, names []string) (cards []domain.Card, notFound []string, err error) {
	for start := 0; start < len(names); start += batchSize {
		end := start + batchSize
		if end > len(names) {
			end = len(names)
		}
		got, nf, err := c.resolveBatch(ctx, names[start:end])
		if err != nil {
			return nil, nil, err
		}
		cards = append(cards, got...)
		notFound = append(notFound, nf...)
	}

	// Fallback for names the collection endpoint can't match. Those are usually
	// flavor names on Secret Lair / Universes Beyond printings (e.g. "White
	// Tower of Ecthelion" -> Karakas), which only the search endpoint matches.
	// Best-effort and one request per name, so it runs only on the remainder.
	if len(notFound) > 0 {
		var stillMissing []string
		for _, name := range notFound {
			if card, ok := c.searchExact(ctx, name); ok {
				cards = append(cards, card)
			} else {
				stillMissing = append(stillMissing, name)
			}
		}
		notFound = stillMissing
	}
	return cards, notFound, nil
}

// searchExact resolves a single name via the search endpoint using an exact
// match (!"name"), which — unlike the collection endpoint — also matches a
// card's flavor name. It is best-effort: any error or non-match yields false so
// the overall sync still succeeds with whatever else resolved.
func (c *Client) searchExact(ctx context.Context, name string) (domain.Card, bool) {
	c.throttle()
	q := url.Values{}
	q.Set("q", `!"`+name+`"`)
	q.Set("unique", "cards")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL+"?"+q.Encode(), nil)
	if err != nil {
		return domain.Card{}, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return domain.Card{}, false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK { // 404 when nothing matches
		return domain.Card{}, false
	}
	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil || len(resp.Data) == 0 {
		return domain.Card{}, false
	}
	var sc scryCard
	if err := json.Unmarshal(resp.Data[0], &sc); err != nil {
		return domain.Card{}, false
	}
	card, err := toDomain(sc, resp.Data[0])
	if err != nil {
		return domain.Card{}, false
	}
	return card, true
}

// frontFace reduces a card name to its front face for Scryfall's collection
// endpoint, which matches a card's front-face name (e.g. "Bonecrusher Giant")
// but NOT its combined "Front // Back" name. Decklist sources (Moxfield) write
// double-faced / split / adventure cards as "Front / Back" or "Front // Back";
// everything before the first slash is the front face. Single-faced names (no
// slash) are returned unchanged.
func frontFace(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

func (c *Client) resolveBatch(ctx context.Context, names []string) ([]domain.Card, []string, error) {
	ids := make([]identifier, len(names))
	for i, n := range names {
		ids[i] = identifier{Name: frontFace(n)}
	}
	body, _ := json.Marshal(map[string]any{"identifiers": ids})

	var resp struct {
		Data     []json.RawMessage `json:"data"`
		NotFound []identifier      `json:"not_found"`
	}
	if err := c.doJSON(ctx, body, &resp); err != nil {
		return nil, nil, err
	}

	cards := make([]domain.Card, 0, len(resp.Data))
	for _, raw := range resp.Data {
		var sc scryCard
		if err := json.Unmarshal(raw, &sc); err != nil {
			return nil, nil, err
		}
		card, err := toDomain(sc, raw)
		if err != nil {
			return nil, nil, err
		}
		cards = append(cards, card)
	}
	nf := make([]string, 0, len(resp.NotFound))
	for _, id := range resp.NotFound {
		nf = append(nf, id.Name)
	}
	return cards, nf, nil
}

func (c *Client) doJSON(ctx context.Context, body []byte, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		c.throttle()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, collectionURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)

		res, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			_ = res.Body.Close()
			lastErr = fmt.Errorf("scryfall status %d", res.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("scryfall status %d", res.StatusCode)
		}
		return json.NewDecoder(res.Body).Decode(out)
	}
	return lastErr
}

func toDomain(sc scryCard, raw json.RawMessage) (domain.Card, error) {
	sid, err := uuid.Parse(sc.ID)
	if err != nil {
		return domain.Card{}, fmt.Errorf("bad scryfall id %q: %w", sc.ID, err)
	}
	card := domain.Card{
		ScryfallID:      sid,
		Name:            sc.Name,
		ManaCost:        sc.ManaCost,
		CMC:             sc.CMC,
		TypeLine:        sc.TypeLine,
		OracleText:      sc.OracleText,
		Colors:          int(domain.ParseColorIdentity(sc.Colors)),
		ColorIdentity:   int(domain.ParseColorIdentity(sc.ColorIdentity)),
		Rarity:          sc.Rarity,
		Layout:          sc.Layout,
		SetCode:         sc.Set,
		CollectorNumber: sc.CollectorNumber,
		Raw:             raw,
	}
	if oid, err := uuid.Parse(sc.OracleID); err == nil {
		card.OracleID = &oid
	}
	img := sc.ImageURIs
	if img == nil && len(sc.CardFaces) > 0 {
		img = sc.CardFaces[0].ImageURIs
	}
	if img != nil {
		card.ImageSmall = strPtr(img.Small)
		card.ImageNormal = strPtr(img.Normal)
		card.ImageArtCrop = strPtr(img.ArtCrop)
	}
	return card, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
