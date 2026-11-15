-- 0002 decklist-centric — decks stand alone; cubes find theirs by tag.
--
-- A deck used to belong to exactly one cube through decklists.cube_id, and died with
-- it. Decks are now the base unit: a deck carries a format and a set of tags, and a
-- cube (or any later collection) is metadata that selects its decks with a filter
-- over those tags rather than owning them. Cubes keep everything else — pool,
-- members, invites, combos, analytics runs — so only the deck → cube edge moves.
--
-- Read a cube's decks as `d.tags @> ARRAY[c.deck_tag]` (store.deckInCube).

-- The tag a cube's decks carry. Generated from the id, so it can never drift from
-- the cube or collide with another one, and needs no backfill on add or on restore.
ALTER TABLE cubes ADD COLUMN deck_tag text GENERATED ALWAYS AS ('cube:' || id::text) STORED;
ALTER TABLE cubes ADD CONSTRAINT cubes_deck_tag_key UNIQUE (deck_tag);

-- format is the game format a deck was built for ('cube' for every deck that predates
-- this file). Free text for now, but lower-case and trimmed so a filter on it is an
-- equality, not a case-folding scan.
ALTER TABLE decklists ADD COLUMN format text
    CONSTRAINT decklists_format_check CHECK (format = lower(btrim(format)) AND format <> '');
-- tags hold every collection a deck is filed under. Never NULL, and no NULL
-- elements: `tags @> ARRAY[NULL]` is never true, so a NULL would hide a deck from
-- every filter without saying so.
ALTER TABLE decklists ADD COLUMN tags text[] NOT NULL DEFAULT '{}'
    CONSTRAINT decklists_tags_check CHECK (array_position(tags, NULL) IS NULL);

-- Move each deck's cube link into its tags before the column goes.
UPDATE decklists d SET tags = ARRAY[c.deck_tag], format = 'cube'
    FROM cubes c WHERE c.id = d.cube_id;

CREATE INDEX idx_decklists_tags   ON decklists USING gin (tags);
CREATE INDEX idx_decklists_format ON decklists (format);

-- Takes idx_decklists_cube and the ON DELETE CASCADE foreign key with it: deleting a
-- cube no longer deletes the decks it listed.
ALTER TABLE decklists DROP COLUMN cube_id;
