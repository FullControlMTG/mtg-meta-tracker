// Enum values for decklist fields. Kept in sync with the CHECK constraints in
// backend/internal/store/schema.sql and the consts in backend/internal/domain/models.go.

export const ARCHETYPES = ["aggro", "control", "midrange", "tempo", "combo"] as const;
export type Archetype = (typeof ARCHETYPES)[number];

export const STATUSES = ["draft", "active", "archived"] as const;
export type DecklistStatus = (typeof STATUSES)[number];
