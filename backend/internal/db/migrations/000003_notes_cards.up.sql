-- A note is the content the user authored. Its note_type decides which cards
-- it expands into, so adding cloze later means new note_type values and new
-- rows in cards — no change to these tables.
CREATE TABLE notes (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id    INTEGER  NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    note_type  TEXT     NOT NULL,
    -- Per-note options that vary by type (e.g. basic's "reversed"), stored as
    -- JSON so a new note type can add options without a migration.
    config     TEXT     NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notes_deck ON notes(deck_id);
CREATE INDEX idx_notes_user ON notes(user_id);

-- Fields are keyed by name rather than being fixed columns: a basic note has
-- front and back, a cloze note will have a single text field, and an image
-- cloze will add an image reference and its regions.
CREATE TABLE note_fields (
    note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    ord     INTEGER NOT NULL,
    name    TEXT    NOT NULL,
    value   TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (note_id, name)
);

-- A card is one scheduled item generated from a note. Everything from
-- `due` down is FSRS memory state.
--
-- user_id and deck_id are denormalised from the note so the due-queue query
-- stays a single-table index scan; both are kept in step inside the same
-- transaction that moves a note.
CREATE TABLE cards (
    id             INTEGER  PRIMARY KEY AUTOINCREMENT,
    note_id        INTEGER  NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    user_id        INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id        INTEGER  NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    template       TEXT     NOT NULL,
    suspended      INTEGER  NOT NULL DEFAULT 0,

    due            DATETIME NOT NULL,
    stability      REAL     NOT NULL DEFAULT 0,
    difficulty     REAL     NOT NULL DEFAULT 0,
    elapsed_days   INTEGER  NOT NULL DEFAULT 0,
    scheduled_days INTEGER  NOT NULL DEFAULT 0,
    reps           INTEGER  NOT NULL DEFAULT 0,
    lapses         INTEGER  NOT NULL DEFAULT 0,
    state          INTEGER  NOT NULL DEFAULT 0,
    last_review    DATETIME,

    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- One card per template per note: this is what lets a note be re-synced after
-- an edit without duplicating or resetting the cards that survive.
CREATE UNIQUE INDEX idx_cards_note_template ON cards(note_id, template);

-- The study queue's hot path.
CREATE INDEX idx_cards_queue ON cards(user_id, deck_id, suspended, due);

-- Every answer ever given. Kept in full because a future FSRS parameter
-- optimiser trains on exactly this.
CREATE TABLE review_log (
    id             INTEGER  PRIMARY KEY AUTOINCREMENT,
    card_id        INTEGER  NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id        INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating         INTEGER  NOT NULL,
    state          INTEGER  NOT NULL,
    elapsed_days   INTEGER  NOT NULL DEFAULT 0,
    scheduled_days INTEGER  NOT NULL DEFAULT 0,
    reviewed_at    DATETIME NOT NULL
);

CREATE INDEX idx_review_log_card ON review_log(card_id, reviewed_at);
CREATE INDEX idx_review_log_user ON review_log(user_id, reviewed_at);
