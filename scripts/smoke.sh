#!/usr/bin/env bash
# Drives the whole product against a built binary: sign up, create a deck,
# author a reversed note, study both cards, and confirm the queue empties.
#
# This exercises the one thing the unit suites cannot — the real artefact,
# with the frontend embedded and every route wired together.
set -euo pipefail

SERVER=${1:?usage: smoke.sh /path/to/crapcard-server}
PORT=${PORT:-8123}
DB=$(mktemp -u /tmp/crapcard-smoke-XXXXXX.db)
COOKIES=$(mktemp)
BASE="http://localhost:${PORT}"

cleanup() {
	[[ -n "${SERVER_PID:-}" ]] && kill "$SERVER_PID" 2>/dev/null || true
	rm -f "$DB" "$DB-wal" "$DB-shm" "$COOKIES"
}
trap cleanup EXIT

CRAPCARD_DB_PATH="$DB" CRAPCARD_ADDR=":${PORT}" "$SERVER" >/tmp/crapcard-smoke.log 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 50); do
	if curl -sf "${BASE}/healthz" >/dev/null 2>&1; then break; fi
	sleep 0.2
done
curl -sf "${BASE}/healthz" >/dev/null || { echo "server never came up"; cat /tmp/crapcard-smoke.log; exit 1; }

api() { curl -sf -b "$COOKIES" -c "$COOKIES" "$@"; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

echo "→ the SPA is served from the binary"
curl -sf "${BASE}/" | grep -qi '<!doctype html' || { echo "index.html not embedded"; exit 1; }

echo "→ first-run setup"
api -X POST "${BASE}/api/auth/setup" -H 'Content-Type: application/json' \
	-d '{"username":"smoke","password":"hunter2hunter2"}' >/dev/null
api -X POST "${BASE}/api/auth/login" -H 'Content-Type: application/json' \
	-d '{"username":"smoke","password":"hunter2hunter2"}' >/dev/null

echo "→ create a deck"
DECK=$(api -X POST "${BASE}/api/decks" -H 'Content-Type: application/json' \
	-d '{"name":"Smoke","description":""}' | json '["id"]')

echo "→ author a reversed note (two cards from one note)"
api -X POST "${BASE}/api/notes" -H 'Content-Type: application/json' \
	-d "{\"deck_id\":${DECK},\"type\":\"basic\",\"reversed\":true,\"fields\":{\"front\":\"ciao\",\"back\":\"hello\"}}" >/dev/null

TOTAL=$(api "${BASE}/api/decks/${DECK}/study/counts" | json '["total"]')
[[ "$TOTAL" == "2" ]] || { echo "expected 2 due cards, got ${TOTAL}"; exit 1; }

echo "→ study both directions"
for _ in 1 2; do
	CARD=$(api "${BASE}/api/decks/${DECK}/study/next" | json '["card_id"]')
	api -X POST "${BASE}/api/cards/${CARD}/answer" -H 'Content-Type: application/json' \
		-d '{"rating":4}' >/dev/null
done

echo "→ the queue is empty (204)"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIES" "${BASE}/api/decks/${DECK}/study/next")
[[ "$CODE" == "204" ]] || { echo "expected 204 after finishing, got ${CODE}"; exit 1; }

echo "→ undo the last answer and the card comes back"
UNDONE=$(api -X POST "${BASE}/api/study/undo" | json '["card_id"]')
[[ "$UNDONE" == "$CARD" ]] || { echo "undo returned card ${UNDONE}, want ${CARD}"; exit 1; }
NEXT=$(api "${BASE}/api/decks/${DECK}/study/next" | json '["card_id"]')
[[ "$NEXT" == "$CARD" ]] || { echo "queue serves ${NEXT} after undo, want ${CARD}"; exit 1; }
api -X POST "${BASE}/api/cards/${CARD}/answer" -H 'Content-Type: application/json' \
	-d '{"rating":4}' >/dev/null

echo "→ unauthenticated access is refused"
CODE=$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/api/decks")
[[ "$CODE" == "401" ]] || { echo "expected 401 without a session, got ${CODE}"; exit 1; }

echo "smoke test passed"
