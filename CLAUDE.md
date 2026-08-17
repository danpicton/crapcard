# crapcard — project notes for Claude

## UI copy

Keep labels terse: bare minimum, no explanatory microcopy. Never append
clauses that explain what a control does — no "Reason (optional — shown in
the flagged cards view)", no "Also suspend — keep it out of study until
resumed", no helper paragraphs restating a button's effect. Just "Reason",
"Also suspend". If a control genuinely needs explanation, a `title` tooltip
is the ceiling. (See commit 05d1f5f "Cut explanatory microcopy; keep labels
terse" — this is a standing convention, not a one-off.)

## UI state indicators

Card states are shown as small glyph components, not text chips/pills:
flagged = `FlagIcon`, suspended = `PauseIcon`, buried = `SpadeIcon`. Follow
the existing icon component pattern (`ClozeIcon`, `BidirectionalIcon`): a
small inline SVG with a `title` tooltip carrying the meaning.
