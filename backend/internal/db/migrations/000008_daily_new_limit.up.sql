ALTER TABLE decks ADD COLUMN daily_new_limit INTEGER NOT NULL DEFAULT -1 CHECK(daily_new_limit >= -1);
CREATE TABLE study_extra (
    deck_id INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    day_end DATETIME NOT NULL,
    extra INTEGER NOT NULL CHECK(extra >= 0),
    PRIMARY KEY(deck_id, day_end)
);
