-- Flagging and burial, both card-level like suspension.
--
-- A flag marks a card for later attention; the reason is free text the user
-- reads back in the flagged-cards view. Burial hides a card until a moment in
-- the future — NULL means not buried, and a past timestamp means the card has
-- come back on its own, so nothing ever has to sweep the table to unbury.
ALTER TABLE cards ADD COLUMN flagged     INTEGER  NOT NULL DEFAULT 0;
ALTER TABLE cards ADD COLUMN flag_reason TEXT     NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN buried_until DATETIME;
