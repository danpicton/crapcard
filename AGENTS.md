# crapcard — project notes for coding agents

## Agent skills

### Issue tracker

Issues and specs live in GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five default triage labels. See `docs/agents/triage-labels.md`.

### Domain docs

Use one root `GLOSSARY.md` and `docs/adr/` for decisions. See `docs/agents/domain.md`.

## Development workflow

For features and bug fixes, agree on the observable test seam, then work in
small red → green slices: write one failing behaviour test, run it to confirm
the failure, implement only enough to pass, and repeat. See `docs/testing.md`
for test layers and commands. Follow `docs/issue-pr-guidelines.md` when writing
or implementing an issue.

## UI copy

Keep labels terse: bare minimum, no explanatory microcopy. Never append
clauses that explain what a control does — no "Reason (optional — shown in
the flagged cards view)", no "Also suspend — keep it out of study until
resumed", no helper paragraphs restating a button's effect. Just "Reason",
"Also suspend". If a control genuinely needs explanation, a `title` tooltip
is the ceiling. (See commit 05d1f5f "Cut explanatory microcopy; keep labels
terse" — this is a standing convention, not a one-off.)

## Feature behaviour

Never bundle one action into another implicitly (e.g. auto-flagging a card
because a suspension carried a reason). Pairings are offered as explicit
opt-ins; the user decides.

## UI state indicators

Card states are shown as small glyph components, not text chips/pills:
flagged = `FlagIcon`, suspended = `PauseIcon`, buried = `SpadeIcon`. Follow
the existing icon component pattern (`ClozeIcon`, `BidirectionalIcon`): a
small inline SVG with a `title` tooltip carrying the meaning.
