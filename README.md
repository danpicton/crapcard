# CrapCard

A spaced-repetition flashcard app, in the shape of [crapnote](https://github.com/danpicton/crapnote).

| Layer | Technology |
|---|---|
| Frontend | Svelte 5 (SvelteKit), Milkdown, Vitest |
| Backend | Go 1.24, `net/http` stdlib router |
| Scheduling | [FSRS](https://github.com/open-spaced-repetition/go-fsrs) via `go-fsrs/v3` |
| Database | SQLite (images stored as blobs alongside everything else) |
| Deployment | Single Docker container; Go binary with the Svelte build embedded via `go:embed` |

Themes, layout conventions and project structure deliberately mirror crapnote —
same seven themes, same CSS custom properties, same `data-theme` mechanism — so
the two apps look and feel like siblings.

---

## What it does today

- **Two-sided text/image cards.** Front and back are markdown, edited in
  Milkdown.
- **Bidirectional notes.** One note optionally produces a second card testing
  the other way, scheduled independently of the first.
- **Paste images straight in.** A screenshot on the clipboard uploads and embeds
  itself in the card. Drag the corner to resize it (double-click the handle to
  restore its natural size), and describe it in the alt-text box that appears
  on hover.
- **Preview.** See every card a note produces, exactly as review will ask them,
  images rendered and all. In the note *list*, images flatten to their alt
  text (`[alt]`, or `[image]` when undescribed), so a missing description is
  still easy to spot.
- **Autosave.** The card editor saves itself as you type — a new card is
  created the moment both sides have content, and every pause persists the
  latest wording. The only buttons left are Done and Preview.
- **Decks.** A note belongs to one deck; study sessions are per-deck, with a
  paginated card list.
- **Offline, without losing anything.** A service worker keeps the app shell
  openable with no network, and everything that must reach the server —
  answers, autosaved edits, new cards — goes into a persistent outbox when
  offline, replayed in order on reconnect. The top bar shows offline/syncing
  state, and a study session pauses mid-card and resumes by itself when the
  connection returns.
- **FSRS scheduling.** Each answer feeds the algorithm; the four answer buttons
  are labelled with the interval each would produce. Review cards are due at
  day granularity in *your* timezone — a card due at 14:00 shows up in the
  09:00 session — while learning steps keep their exact intra-day timing.
  Due learning and review cards are always served before new cards, so a
  freshly authored batch cannot starve the reviews scheduled for today.
- **Undo.** The most recent answer can be taken back — the card returns,
  revealed, ready to be graded properly. Repeat to step further back.
- **Keyboard review.** Space reveals, 1–4 grade (space again for Good, with a
  beat of cooldown so a held key cannot grade unread cards), U undoes.
- **Signing in lands on a card.** The next thing due, from the deck you were
  last working through.
- **Multi-user.** Sessions, per-user scoping on every query.

### Not yet

Text cloze and image cloze deletion. They are the next feature, and the data
model was built for them — see below.

---

## Project structure

```
/
├── backend/
│   ├── cmd/server/          # main entrypoint, HTTP mux, embedded UI
│   ├── internal/
│   │   ├── auth/            # users, sessions, login, middleware
│   │   ├── cards/           # the scheduled items + FSRS state + review log
│   │   ├── db/              # Open(), embedded migration runner
│   │   │   └── migrations/  # versioned *.up.sql / *.down.sql
│   │   ├── decks/
│   │   ├── httpx/           # shared HTTP helpers
│   │   ├── images/          # clipboard image upload/serving
│   │   ├── notes/           # authored content + note types + card generation
│   │   ├── srs/             # the scheduler, wrapping go-fsrs
│   │   └── study/           # the review loop
│   ├── static/              # go:embed target — populated from frontend build
│   └── Makefile
├── frontend/                # SvelteKit app
├── scripts/smoke.sh         # end-to-end check against a built binary
└── Dockerfile               # multi-stage: node → go (CGO) → distroless/cc
```

---

## How the card model works

This is the part worth understanding, because it is what makes cloze an
additive change rather than a rewrite.

A **note** is what you author. A **card** is what gets scheduled. One note
produces one or more cards, decided by its **note type**:

```
note (type: basic, fields: front/back, config: {reversed: true})
  ├── card (template: "forward")   ← independent FSRS state
  └── card (template: "reverse")   ← independent FSRS state
```

A note type answers exactly two questions — which cards does this note produce
(`Generate`), and what goes on each side of one of them (`Render`) — so the
review queue, the scheduler and the API never learn what a note type *is*. They
deal in cards and an opaque template string.

Adding cloze therefore means registering another `Generator` in
`internal/notes/notetype.go`, which produces `cloze:1`, `cloze:2`, … templates
from a single text field. Nothing in `study`, `srs` or `cards` changes.

Image sizing rides in the image URL as `?w=<pixels>` rather than in markdown
syntax or an HTML tag. That keeps a card plain CommonMark — any other renderer
still shows the image, alt text stays real alt text, and no user-authored HTML
is ever rendered from our own origin. The server ignores the parameter.

Three schema decisions exist to support that:

- **Fields are rows, not columns** (`note_fields`: `note_id, ord, name, value`).
  A basic note has `front`/`back`; a cloze note will have one `text` field, and
  image cloze will add an image reference plus regions. No migration per type.
- **Per-note options are JSON** (`notes.config`). `reversed` lives there today;
  cloze options join it without a schema change.
- **Editing a note never resets scheduling.** On save, cards whose template
  survives are left completely alone; only templates that disappear take their
  history with them. Rewording a card keeps everything you have learned.

### On the scheduler

`internal/srs` wraps go-fsrs behind an interface. That boundary is deliberate:
go-fsrs implements FSRS *scheduling* but not *parameter optimisation* — training
weights from your own review history exists only in the Rust (`fsrs-rs`, which
Anki ships) and Python ports. The full review log is recorded, and `Params` are
stored as JSON, so an optimiser can later run out of process and hand back
weights without the review pipeline noticing.

---

## Prerequisites

- Go 1.24 (`go version`)
- gcc / build-essential (required for `mattn/go-sqlite3` CGO)
- Node 22+ / npm

## Running it

```bash
# Backend (serves a placeholder page until the frontend is embedded)
cd backend
make run          # or: CGO_ENABLED=1 go run ./cmd/server

# Frontend dev server, proxying /api to the backend above
cd frontend
npm install
npm run dev
```

Then open the dev server and complete first-run setup — the first account you
create is the administrator.

### Production build

```bash
cd backend
make build-prod   # builds the frontend, embeds it, compiles ./server
./server
```

Or with Docker:

```bash
docker build -t crapcard .
docker run -p 8080:8080 -v crapcard-data:/data crapcard
```

### Configuration

| Variable | Default | Meaning |
|---|---|---|
| `CRAPCARD_ADDR` | `:8080` | Listen address |
| `CRAPCARD_DB_PATH` | `crapcard.db` | SQLite file (`/data/crapcard.db` in Docker) |
| `CRAPCARD_PAGE_SIZE` | `50` | Default cards per page in a deck listing. Users can override it in Settings, and per deck from the list header. |
| `TRUST_PROXY` | unset | Honour `X-Forwarded-*`. Only enable behind a proxy you control that strips inbound copies — any client can otherwise forge them. |

## Tests

```bash
cd backend  && make test && make lint
cd frontend && npm test && npm run check
./scripts/smoke.sh /path/to/server     # whole stack, real binary
```

The whole thing was built test-first, red then green, one vertical slice at a
time.

---

## API

All routes need a session cookie except `/healthz` and the setup/login handshake.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | Liveness |
| `GET` | `/api/auth/setup-status` | Does this instance need its first admin? |
| `POST` | `/api/auth/setup` | Create the first admin (409 once one exists) |
| `POST` | `/api/auth/login` / `logout` | Session cookie |
| `GET` | `/api/auth/me` | Current user |
| `GET`/`POST` | `/api/decks` | List / create decks |
| `GET`/`PUT`/`DELETE` | `/api/decks/{id}` | One deck |
| `GET` | `/api/note-types` | What note types exist, and their fields |
| `GET`/`POST` | `/api/notes` | List (`?deck_id=`, `?limit=`, `?offset=`) / create |
| `GET`/`PUT`/`DELETE` | `/api/notes/{id}` | One note, with its cards |
| `GET` | `/api/config` | Deployment settings the client needs (page size) |
| `GET` | `/api/notes/{id}/preview` | Every card the note produces, rendered |
| `GET` | `/api/study/next` | Next due card from any deck, or `204` if none |
| `GET` | `/api/decks/{id}/study/next` | Next due card in one deck, or `204` |
| `GET` | `/api/decks/{id}/study/counts` | Queue counts |
| `POST` | `/api/cards/{id}/answer` | `{"rating": 1..4}` |
| `POST` | `/api/study/undo` | Revert the latest answer; returns the card, or `204` if nothing to undo |

The study endpoints accept `?tz_offset=<minutes east of UTC>` (what
JavaScript's `-getTimezoneOffset()` reports) so review cards can be gated on
the end of the client's calendar day. Without it, days roll over at UTC
midnight.
| `POST` | `/api/images` | Raw image bytes (what a paste produces) |
| `GET` | `/api/images/{id}` | Fetch an image |
