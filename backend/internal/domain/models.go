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

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Bio          *string   `json:"bio,omitempty"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	Role         string    `json:"role"`
	PasswordHash *string   `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Public returns a view safe to expose to anonymous callers (no email/hash).
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

type Invite struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	TokenHash  string     `json:"-"`
	InvitedBy  *uuid.UUID `json:"invited_by,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Cube struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	MoxfieldPublicID *string    `json:"moxfield_public_id,omitempty"`
	Description      *string    `json:"description,omitempty"`
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

type Job struct {
	ID       uuid.UUID       `json:"id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Status   string          `json:"status"`
	Attempts int             `json:"attempts"`
}
