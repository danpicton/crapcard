-- A suspension carries its own optional reason, independent of flagging —
-- the two states are siblings, each with a note to your future self.
ALTER TABLE cards ADD COLUMN suspend_reason TEXT NOT NULL DEFAULT '';
