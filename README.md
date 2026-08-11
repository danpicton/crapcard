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
- **Reversal.** One note optionally produces a second card testing back → front,
  scheduled independently of the first.
- **Paste images straight in.** A screenshot on the clipboard uploads and embeds
  itself in the card.
- **Decks.** A note belongs to one deck; study sessions are per-deck.
- **FSRS scheduling.** Each answer feeds the algorithm; the four answer buttons
  are labelled with the interval each would produce.
- **Keyboard review.** Space reveals, 1–4 grade.
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
| `GET`/`POST` | `/api/notes` | List (`?deck_id=`) / create notes |
| `GET`/`PUT`/`DELETE` | `/api/notes/{id}` | One note, with its cards |
| `GET` | `/api/decks/{id}/study/next` | Next due card, or `204` if none |
| `GET` | `/api/decks/{id}/study/counts` | Queue counts |
| `POST` | `/api/cards/{id}/answer` | `{"rating": 1..4}` |
| `POST` | `/api/images` | Raw image bytes (what a paste produces) |
| `GET` | `/api/images/{id}` | Fetch an image |
