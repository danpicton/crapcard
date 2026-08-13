CREATE TABLE decks (
    id          INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT     NOT NULL,
    description TEXT     NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Deck names are unique per user, not globally: two people may each keep an
-- "Italian" deck.
CREATE UNIQUE INDEX idx_decks_user_name ON decks(user_id, name);
