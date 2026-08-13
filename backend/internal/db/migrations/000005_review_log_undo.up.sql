-- Snapshot of the card's scheduling state as it was *before* the answer, so
-- the most recent review can be undone by putting the card back exactly where
-- it stood. Nullable: rows written before this migration carry no snapshot
-- and simply cannot be undone.
ALTER TABLE review_log ADD COLUMN prev_due            DATETIME;
ALTER TABLE review_log ADD COLUMN prev_stability      REAL;
ALTER TABLE review_log ADD COLUMN prev_difficulty     REAL;
ALTER TABLE review_log ADD COLUMN prev_elapsed_days   INTEGER;
ALTER TABLE review_log ADD COLUMN prev_scheduled_days INTEGER;
ALTER TABLE review_log ADD COLUMN prev_reps           INTEGER;
ALTER TABLE review_log ADD COLUMN prev_lapses         INTEGER;
ALTER TABLE review_log ADD COLUMN prev_state          INTEGER;
ALTER TABLE review_log ADD COLUMN prev_last_review    DATETIME;
