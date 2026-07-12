package scryfall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const (
	defaultBaseURL = "https://api.scryfall.com"
	batchSize      = 75
)

// How a query was resolved, for logging and for the sync summary.
const (
	ViaPrinting = "printing" // matched {set, collector_number}
	ViaName     = "name"     // matched {name}
	ViaSearch   = "search"   // matched the search endpoint's exact-name operator
)

type Client struct {
	http        *http.Client
	baseURL     string
	userAgent   string
	minInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(userAgent string, minInterval time.Duration) *Client {
	return NewWithBaseURL(defaultBaseURL, userAgent, minInterval)
}

// NewWithBaseURL points the client at an alternate host. Tests use it to stand
// up an httptest server.
func NewWithBaseURL(baseURL, userAgent string, minInterval time.Duration) *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		baseURL:     strings.TrimRight(baseURL, "/"),
		userAgent:   userAgent,
		minInterval: minInterval,
	}
}

// Query is one card we want. SetCode and Collector are the "(ELD) 115"
// annotation from the pasted list; both empty means "resolve by name alone".
type Query struct {
	Name      string
	SetCode   string
	Collector string
}

// Result is the outcome for exactly one Query. Resolve returns one Result per
// input Query, in input order — callers bind a Result to its source entry by
// position, never by name. Matching on the name is what silently dropped
// double-faced and flavor-named cards: Scryfall echoes a canonical name
// ("Bonecrusher Giant // Stomp", "Karakas") that need not equal what was pasted
// ("Bonecrusher Giant / Stomp", "White Tower of Ecthelion").
type Result struct {
	Query Query
	Card  *domain.Card // nil when nothing matched
	Via   string       // ViaPrinting | ViaName | ViaSearch, "" when unresolved
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
	Name            string `json:"name,omitempty"`
	Set             string `json:"set,omitempty"`
	CollectorNumber string `json:"collector_number,omitempty"`
}

type imageURIs struct {
	Small   string `json:"small"`
	Normal  string `json:"normal"`
	ArtCrop string `json:"art_crop"`
}

type cardFace struct {
	Name       string     `json:"name"`
	FlavorName string     `json:"flavor_name"`
	Colors     []string   `json:"colors"`
	ImageURIs  *imageURIs `json:"image_uris"`
}

type scryCard struct {
	ID              string     `json:"id"`
	OracleID        string     `json:"oracle_id"`
	Name            string     `json:"name"`
	FlavorName      string     `json:"flavor_name"`
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
	CardFaces       []cardFace `json:"card_faces"`
}

// Resolve looks up every query and returns one Result per query, in order.
//
// Three passes, each filling only the slots still unresolved:
//
//  1. printing — {set, collector_number}, the exact card the list named
//  2. name     — {name}, reduced to the front face
//  3. search   — !"name", which unlike the collection endpoint also matches a
//     printing's flavor name (Secret Lair / Universes Beyond)
func (c *Client) Resolve(ctx context.Context, qs []Query) ([]Result, error) {
	results := make([]Result, len(qs))
	for i, q := range qs {
		results[i] = Result{Query: q}
	}

	if err := c.resolvePrintings(ctx, results); err != nil {
		return nil, err
	}
	if err := c.resolveNames(ctx, results); err != nil {
		return nil, err
	}
	c.resolveSearch(ctx, results)

	var printing, name, search, unresolved int
	for _, r := range results {
		switch r.Via {
		case ViaPrinting:
			printing++
		case ViaName:
			name++
		case ViaSearch:
			search++
		default:
			unresolved++
		}
	}
	log.Printf("scryfall: resolve: %d queries -> %d printing, %d name, %d search, %d unresolved",
		len(qs), printing, name, search, unresolved)
	return results, nil
}

// resolvePrintings fills slots whose query names an exact printing. The join is
// exact: Scryfall echoes set and collector_number back on the card itself, so we
// match the response against the request rather than trusting response order.
func (c *Client) resolvePrintings(ctx context.Context, results []Result) error {
	var idx []int
	for i, r := range results {
		if r.Query.SetCode != "" && r.Query.Collector != "" {
			idx = append(idx, i)
		}
	}
	return c.collectionPass(ctx, results, idx, "printing", ViaPrinting,
		func(q Query) identifier {
			return identifier{Set: strings.ToLower(q.SetCode), CollectorNumber: q.Collector}
		},
		func(q Query, sc scryCard) bool {
			return sc.Set != nil && sc.CollectorNumber != nil &&
				strings.EqualFold(*sc.Set, q.SetCode) &&
				strings.EqualFold(*sc.CollectorNumber, q.Collector)
		})
}

// resolveNames fills whatever the printing pass left, matching on any name the
// card answers to.
func (c *Client) resolveNames(ctx context.Context, results []Result) error {
	var idx []int
	for i, r := range results {
		if r.Card == nil {
			idx = append(idx, i)
		}
	}
	return c.collectionPass(ctx, results, idx, "name", ViaName,
		func(q Query) identifier { return identifier{Name: frontFace(q.Name)} },
		func(q Query, sc scryCard) bool {
			aliases := nameAliases(sc)
			_, ok := aliases[strings.ToLower(q.Name)]
			if ok {
				return true
			}
			_, ok = aliases[strings.ToLower(frontFace(q.Name))]
			return ok
		})
}

// collectionPass batches the selected queries against /cards/collection and
// assigns each returned card to the (single) query it matches.
func (c *Client) collectionPass(
	ctx context.Context,
	results []Result,
	idx []int,
	label string,
	via string,
	toID func(Query) identifier,
	matches func(Query, scryCard) bool,
) error {
	if len(idx) == 0 {
		return nil
	}
	batches := (len(idx) + batchSize - 1) / batchSize

	for b := 0; b < batches; b++ {
		start := b * batchSize
		end := start + batchSize
		if end > len(idx) {
			end = len(idx)
		}
		chunk := idx[start:end]

		ids := make([]identifier, len(chunk))
		for i, at := range chunk {
			ids[i] = toID(results[at].Query)
		}
		body, err := json.Marshal(map[string]any{"identifiers": ids})
		if err != nil {
			return err
		}

		var resp struct {
			Data     []json.RawMessage `json:"data"`
			NotFound []identifier      `json:"not_found"`
		}
		if err := c.postJSON(ctx, c.baseURL+"/cards/collection", body, &resp); err != nil {
			return err
		}

		unmatched := 0
		for _, raw := range resp.Data {
			var sc scryCard
			if err := json.Unmarshal(raw, &sc); err != nil {
				return err
			}
			placed := false
			for _, at := range chunk {
				if results[at].Card != nil || !matches(results[at].Query, sc) {
					continue
				}
				card, err := toDomain(sc, raw)
				if err != nil {
					return err
				}
				results[at].Card = &card
				results[at].Via = via
				placed = true
				break
			}
			if !placed {
				unmatched++
			}
		}

		// An unmatched card is a card Scryfall found and we then threw away — the
		// exact failure that made 19 cards vanish silently. Never let it be quiet.
		log.Printf("scryfall: %s batch %d/%d: %d identifiers -> %d data, %d not_found, %d unmatched",
			label, b+1, batches, len(chunk), len(resp.Data), len(resp.NotFound), unmatched)
	}
	return nil
}

// resolveSearch is the last resort, one request per still-missing query. It is
// best-effort: a miss leaves the slot unresolved rather than failing the import.
func (c *Client) resolveSearch(ctx context.Context, results []Result) {
	for i := range results {
		if results[i].Card != nil {
			continue
		}
		q := results[i].Query
		card, ok := c.searchExact(ctx, q)
		if !ok {
			log.Printf("scryfall: search fallback %q -> miss", q.Name)
			continue
		}
		results[i].Card = &card
		results[i].Via = ViaSearch

		set, num := "?", "?"
		if card.SetCode != nil {
			set = *card.SetCode
		}
		if card.CollectorNumber != nil {
			num = *card.CollectorNumber
		}
		log.Printf("scryfall: search fallback %q -> %s (%s/%s)", q.Name, card.Name, set, num)
	}
}

// searchExact resolves one query via the search endpoint's exact-name operator
// (!"name"), which — unlike the collection endpoint — also matches a printing's
// flavor name, e.g. "White Tower of Ecthelion" -> Karakas. It tries the query's
// own set first, then retries unscoped: a flavor-named printing does not always
// live in the set the list claims.
func (c *Client) searchExact(ctx context.Context, q Query) (domain.Card, bool) {
	if q.SetCode != "" {
		if card, ok := c.searchOnce(ctx, `!"`+q.Name+`" set:`+strings.ToLower(q.SetCode)); ok {
			return card, true
		}
	}
	return c.searchOnce(ctx, `!"`+q.Name+`"`)
}

func (c *Client) searchOnce(ctx context.Context, query string) (domain.Card, bool) {
	v := url.Values{}
	v.Set("q", query)
	v.Set("unique", "cards")

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	// A 404 here just means "no match" — expected, not an error worth surfacing.
	if err := c.getJSON(ctx, c.baseURL+"/cards/search?"+v.Encode(), &resp); err != nil {
		return domain.Card{}, false
	}
	if len(resp.Data) == 0 {
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

// nameAliases collects every name a printing answers to, lowercased: its
// canonical name, its front face, its flavor name, and the same for each face.
func nameAliases(sc scryCard) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out[strings.ToLower(s)] = struct{}{}
		}
	}
	add(sc.Name)
	add(frontFace(sc.Name))
	add(sc.FlavorName)
	add(frontFace(sc.FlavorName))
	for _, f := range sc.CardFaces {
		add(f.Name)
		add(f.FlavorName)
	}
	return out
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

func (c *Client) postJSON(ctx context.Context, url string, body []byte, out any) error {
	return c.do(ctx, http.MethodPost, url, body, out)
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	return c.do(ctx, http.MethodGet, url, nil, out)
}

// do issues a request, retrying on 429 and 5xx. Both endpoints go through here:
// the search fallback used to treat a rate-limit as "card not found", which
// deleted cards from the pool whenever Scryfall throttled us.
func (c *Client) do(ctx context.Context, method, url string, body []byte, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		c.throttle()

		var rdr *bytes.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		var req *http.Request
		var err error
		if rdr != nil {
			req, err = http.NewRequestWithContext(ctx, method, url, rdr)
		} else {
			req, err = http.NewRequestWithContext(ctx, method, url, nil)
		}
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
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
			log.Printf("scryfall: %s %s: status %d, retrying", method, url, res.StatusCode)
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
		Colors:          int(castColors(sc)),
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

// The colors of a card's casting cost. Scryfall reports them per face on a
// double-faced card and omits the top-level field entirely, so taking `colors` at
// face value files every DFC under colorless; union the faces instead.
func castColors(sc scryCard) domain.ColorIdentity {
	if len(sc.Colors) > 0 {
		return domain.ParseColorIdentity(sc.Colors)
	}
	var ci domain.ColorIdentity
	for _, f := range sc.CardFaces {
		ci = ci.Merge(domain.ParseColorIdentity(f.Colors))
	}
	return ci
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
