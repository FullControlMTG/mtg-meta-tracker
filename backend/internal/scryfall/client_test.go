package scryfall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFrontFace(t *testing.T) {
	cases := map[string]string{
		"Sol Ring": "Sol Ring",
		"Fable of the Mirror-Breaker / Reflection of Kiki-Jiki":  "Fable of the Mirror-Breaker",
		"Fable of the Mirror-Breaker // Reflection of Kiki-Jiki": "Fable of the Mirror-Breaker",
		"Life // Death":                                  "Life",
		"Bonecrusher Giant//Stomp":                       "Bonecrusher Giant",
		"Aang, Swift Savior / Aang and La, Ocean's Fury": "Aang, Swift Savior",
	}
	for in, want := range cases {
		if got := frontFace(in); got != want {
			t.Errorf("frontFace(%q) = %q, want %q", in, got, want)
		}
	}
}

// A stub Scryfall printing. id must be a valid uuid or toDomain rejects it.
func card(id, name, set, num string) map[string]any {
	return map[string]any{
		"id":               id,
		"oracle_id":        "00000000-0000-0000-0000-0000000000ff",
		"name":             name,
		"set":              set,
		"collector_number": num,
		"cmc":              1.0,
		"colors":           []string{},
		"color_identity":   []string{},
	}
}

func uid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

// collectionReq is the request body we send to /cards/collection.
type collectionReq struct {
	Identifiers []identifier `json:"identifiers"`
}

func decodeIDs(t *testing.T, r *http.Request) []identifier {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var req collectionReq
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return req.Identifiers
}

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewWithBaseURL(srv.URL, "test-agent", 0)
}

// The printing pass sends {set, collector_number} — the whole point of keeping
// the "(PLST) EMA-2" annotation — and the awkward real-world collector numbers
// survive the round trip.
func TestResolveByPrinting(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := decodeIDs(t, r)
		if len(ids) != 3 {
			t.Fatalf("got %d identifiers, want 3", len(ids))
		}
		for _, id := range ids {
			if id.Name != "" {
				t.Errorf("printing pass sent a name identifier: %+v", id)
			}
			if id.Set == "" || id.CollectorNumber == "" {
				t.Errorf("printing identifier missing set/collector: %+v", id)
			}
		}
		writeJSON(w, map[string]any{
			"data": []any{
				card(uid(1), "Balance", "plst", "EMA-2"),
				card(uid(2), "Jace, Wielder of Mysteries", "war", "54★"),
				card(uid(3), "Hymn to Tourach", "fem", "38a"),
			},
			"not_found": []any{},
		})
	})

	qs := []Query{
		{Name: "Balance", SetCode: "PLST", Collector: "EMA-2"},
		{Name: "Jace, Wielder of Mysteries", SetCode: "WAR", Collector: "54★"},
		{Name: "Hymn to Tourach", SetCode: "FEM", Collector: "38a"},
	}
	got, err := c.Resolve(context.Background(), qs)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for i, r := range got {
		if r.Card == nil {
			t.Fatalf("query %d (%s) unresolved", i, r.Query.Name)
		}
		if r.Via != ViaPrinting {
			t.Errorf("query %d resolved via %q, want %q", i, r.Via, ViaPrinting)
		}
		if r.Card.Name != qs[i].Name {
			t.Errorf("query %d bound to %q, want %q", i, r.Card.Name, qs[i].Name)
		}
	}
}

// The regression that cost 94 cards: Scryfall may return data in any order, and
// a card's canonical name need not equal the pasted one. Results must still bind
// to their own query, by position.
func TestResolveBindsResultsToQueriesRegardlessOfOrder(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Deliberately reversed relative to the request.
		writeJSON(w, map[string]any{
			"data": []any{
				card(uid(3), "Sol Ring", "lea", "270"),
				card(uid(2), "Lightning Bolt", "2x2", "117"),
				card(uid(1), "Balance", "plst", "EMA-2"),
			},
			"not_found": []any{},
		})
	})

	qs := []Query{
		{Name: "Balance", SetCode: "PLST", Collector: "EMA-2"},
		{Name: "Lightning Bolt", SetCode: "2X2", Collector: "117"},
		{Name: "Sol Ring", SetCode: "LEA", Collector: "270"},
	}
	got, err := c.Resolve(context.Background(), qs)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got) != len(qs) {
		t.Fatalf("got %d results, want %d", len(got), len(qs))
	}
	for i, r := range got {
		if r.Query != qs[i] {
			t.Fatalf("result %d carries query %+v, want %+v", i, r.Query, qs[i])
		}
		if r.Card == nil || r.Card.Name != qs[i].Name {
			t.Fatalf("result %d bound to the wrong card: %+v", i, r.Card)
		}
	}
}

// A double-faced card written "Front / Back" is queried by its front face and
// comes back as "Front // Back". It used to be fetched and then thrown away
// because the canonical name did not equal the pasted one.
func TestResolveDoubleFacedName(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := decodeIDs(t, r)
		if ids[0].Name != "Bonecrusher Giant" {
			t.Errorf("sent name %q, want the front face %q", ids[0].Name, "Bonecrusher Giant")
		}
		writeJSON(w, map[string]any{
			"data":      []any{card(uid(1), "Bonecrusher Giant // Stomp", "eld", "115")},
			"not_found": []any{},
		})
	})

	got, err := c.Resolve(context.Background(), []Query{{Name: "Bonecrusher Giant / Stomp"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Card == nil {
		t.Fatal("double-faced card came back unresolved")
	}
	if got[0].Via != ViaName {
		t.Errorf("resolved via %q, want %q", got[0].Via, ViaName)
	}
}

// A flavor name misses both collection passes and is rescued by the search
// endpoint, which is the only one that matches flavor names.
func TestResolveFlavorNameFallsBackToSearch(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cards/collection" {
			ids := decodeIDs(t, r)
			nf := make([]any, len(ids))
			for i, id := range ids {
				nf[i] = id
			}
			writeJSON(w, map[string]any{"data": []any{}, "not_found": nf})
			return
		}
		// /cards/search
		if q := r.URL.Query().Get("q"); !strings.Contains(q, "White Tower of Ecthelion") {
			t.Errorf("search q = %q, want the exact-name operator", q)
		}
		writeJSON(w, map[string]any{"data": []any{card(uid(9), "Karakas", "ltc", "367")}})
	})

	got, err := c.Resolve(context.Background(),
		[]Query{{Name: "White Tower of Ecthelion", SetCode: "LTC", Collector: "367"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Card == nil {
		t.Fatal("flavor name came back unresolved")
	}
	if got[0].Card.Name != "Karakas" {
		t.Errorf("resolved to %q, want Karakas", got[0].Card.Name)
	}
	if got[0].Via != ViaSearch {
		t.Errorf("resolved via %q, want %q", got[0].Via, ViaSearch)
	}
}

// A genuine miss leaves the slot unresolved rather than failing the whole import,
// and the result still carries its own query so the caller can name what failed.
func TestResolveUnresolvedIsReported(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cards/collection" {
			ids := decodeIDs(t, r)
			nf := make([]any, len(ids))
			for i, id := range ids {
				nf[i] = id
			}
			writeJSON(w, map[string]any{"data": []any{}, "not_found": nf})
			return
		}
		http.Error(w, `{"object":"error"}`, http.StatusNotFound)
	})

	got, err := c.Resolve(context.Background(), []Query{{Name: "Definitely Not A Card"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Card != nil {
		t.Fatal("expected no card")
	}
	if got[0].Via != "" {
		t.Errorf("Via = %q, want empty", got[0].Via)
	}
	if got[0].Query.Name != "Definitely Not A Card" {
		t.Errorf("result lost its query: %+v", got[0].Query)
	}
}

// A 429 used to be indistinguishable from "card not found" on the search path,
// which silently deleted cards from the pool whenever Scryfall throttled us.
func TestSearchRetriesOnRateLimit(t *testing.T) {
	var searches int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cards/collection" {
			ids := decodeIDs(t, r)
			nf := make([]any, len(ids))
			for i, id := range ids {
				nf[i] = id
			}
			writeJSON(w, map[string]any{"data": []any{}, "not_found": nf})
			return
		}
		if atomic.AddInt32(&searches, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, map[string]any{"data": []any{card(uid(7), "Karakas", "ltc", "367")}})
	})

	got, err := c.Resolve(context.Background(), []Query{{Name: "White Tower of Ecthelion"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Card == nil {
		t.Fatal("a rate-limited search dropped the card instead of retrying")
	}
	if searches < 2 {
		t.Errorf("search issued %d times, want a retry", searches)
	}
}

// Scryfall's collection endpoint caps a request at 75 identifiers.
func TestResolveBatchesAt75(t *testing.T) {
	var batches int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := decodeIDs(t, r)
		n := atomic.AddInt32(&batches, 1)
		if len(ids) > batchSize {
			t.Errorf("batch %d had %d identifiers, over the %d cap", n, len(ids), batchSize)
		}
		data := make([]any, len(ids))
		for i, id := range ids {
			data[i] = card(uid(int(n)*100+i), id.Name, "tst", fmt.Sprint(i))
		}
		writeJSON(w, map[string]any{"data": data, "not_found": []any{}})
	})

	qs := make([]Query, 80)
	for i := range qs {
		qs[i] = Query{Name: fmt.Sprintf("Card %d", i)}
	}
	got, err := c.Resolve(context.Background(), qs)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if batches != 2 {
		t.Errorf("issued %d requests for 80 queries, want 2", batches)
	}
	if len(got) != 80 {
		t.Fatalf("got %d results, want 80", len(got))
	}
	for i, r := range got {
		if r.Card == nil {
			t.Fatalf("query %d (%s) unresolved", i, r.Query.Name)
		}
		if r.Card.Name != qs[i].Name {
			t.Fatalf("query %d bound to %q, want %q", i, r.Card.Name, qs[i].Name)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
