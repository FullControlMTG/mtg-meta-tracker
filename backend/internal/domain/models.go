package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Decklist board sections.
const (
	BoardMain  = "main"
	BoardSide  = "side"
	BoardMaybe = "maybe"
)

// Decklist lifecycle status.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Decklist archetypes.
const (
	ArchetypeAggro    = "aggro"
	ArchetypeControl  = "control"
	ArchetypeMidrange = "midrange"
	ArchetypeTempo    = "tempo"
	ArchetypeCombo    = "combo"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        *string   `json:"email,omitempty"`
	DisplayName  string    `json:"display_name"`
	Bio          *string   `json:"bio,omitempty"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	Role         string    `json:"role"`
	PasswordHash *string   `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Anonymous-safe view: omits email/hash.
func (u User) Public() map[string]any {
	return map[string]any{
		"id":           u.ID,
		"username":     u.Username,
		"display_name": u.DisplayName,
		"bio":          u.Bio,
		"avatar_url":   u.AvatarURL,
		"role":         u.Role,
		"created_at":   u.CreatedAt,
	}
}

type Session struct {
	ID        string    `json:"-"`
	UserID    uuid.UUID `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Cube struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	MoxfieldPublicID *string    `json:"moxfield_public_id,omitempty"`
	Description      *string    `json:"description,omitempty"`
	CardList         *string    `json:"card_list,omitempty"` // raw pasted decklist; source of truth for the pool
	ContentHash      *string    `json:"-"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Card struct {
	ScryfallID      uuid.UUID       `json:"scryfall_id"`
	OracleID        *uuid.UUID      `json:"oracle_id,omitempty"`
	Name            string          `json:"name"`
	ManaCost        *string         `json:"mana_cost,omitempty"`
	CMC             float64         `json:"cmc"`
	TypeLine        *string         `json:"type_line,omitempty"`
	OracleText      *string         `json:"oracle_text,omitempty"`
	Colors          int             `json:"colors"`
	ColorIdentity   int             `json:"color_identity"`
	Rarity          *string         `json:"rarity,omitempty"`
	Layout          *string         `json:"layout,omitempty"`
	ImageSmall      *string         `json:"image_small,omitempty"`
	ImageNormal     *string         `json:"image_normal,omitempty"`
	ImageArtCrop    *string         `json:"image_art_crop,omitempty"`
	SetCode         *string         `json:"set_code,omitempty"`
	CollectorNumber *string         `json:"collector_number,omitempty"`
	Raw             json.RawMessage `json:"-"`
}

type Decklist struct {
	ID            uuid.UUID `json:"id"`
	CubeID        uuid.UUID `json:"cube_id"`
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description,omitempty"`
	ColorIdentity int       `json:"color_identity"`
	// The colors the deck merely splashes (see domain.InferDeckColors); disjoint
	// from ColorIdentity, and excluded from the meta's color analytics.
	SplashColors int     `json:"splash_colors"`
	Archetype    *string `json:"archetype,omitempty"`
	SourceURL    *string `json:"source_url,omitempty"`
	DecklistRaw  string  `json:"decklist_raw"`
	CardCount    int     `json:"card_count"`
	Status       string  `json:"status"`

	// Record (nullable / added after the fact).
	GamesPlayed     int        `json:"games_played"`
	Wins            int        `json:"wins"`
	Losses          int        `json:"losses"`
	EventName       *string    `json:"event_name,omitempty"`
	PlayedAt        *time.Time `json:"played_at,omitempty"`
	RecordUpdatedAt *time.Time `json:"record_updated_at,omitempty"`
	Winrate         *float64   `json:"winrate,omitempty"` // generated column

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DecklistCard struct {
	DecklistID uuid.UUID  `json:"decklist_id"`
	CardID     *uuid.UUID `json:"card_id,omitempty"`
	CardName   string     `json:"card_name"`
	Quantity   int        `json:"quantity"`
	IsResolved bool       `json:"is_resolved"`
	Board      string     `json:"board"`
}

type Job struct {
	ID       uuid.UUID       `json:"id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Status   string          `json:"status"`
	Attempts int             `json:"attempts"`
}
