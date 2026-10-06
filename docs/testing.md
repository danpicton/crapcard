# Testing workflow

For each feature or bug fix, agree on the observable boundary to test. Start
with one behaviour that can fail at that boundary. Write and run its test to
see red, make the smallest change that turns it green, then repeat with the
next behaviour. Keep each cycle a vertical slice rather than writing a batch
of tests before implementation.

Test through public interfaces so a refactor does not break a test while the
behaviour still works. Use expected results from a concrete example or the
issue's acceptance criteria, rather than calculating them the same way as the
implementation. Prefer real collaborators at the chosen boundary; mock only
external effects that make the test slow or nondeterministic.

## Test layers

| Behaviour | Existing test location | Focused command |
| --- | --- | --- |
| Go package behaviour | `backend/internal/**/*_test.go` | `cd backend && go test ./internal/<package>` |
| HTTP and server wiring | `backend/cmd/server/*_test.go` | `cd backend && go test ./cmd/server` |
| Frontend behaviour | `frontend/src/**/*.test.ts` | `cd frontend && npm test -- <test-file>` |
| CLI commands and HTTP usage | `scripts/test_crapcard.py` | `python3 -m unittest discover -s scripts -p 'test_crapcard.py'` |
| Embedded app journey | `scripts/smoke.sh` | `make smoke` |

Before merging, run `make test` and `make lint` at the repo root. CI also runs
the Go race detector, frontend type checks and build, and the embedded binary
smoke test.

For documentation-only changes, review links and consistency with the code;
no behaviour test is needed.
