# Issue and PR guidelines

Issues and PRs here are read by humans **and** by coding agents picking work up cold. The templates in [`.github/`](../.github/) enforce the structure; this page records the why. The rules are short because most of "agent-readable" is just good practice for humans too — the delta is explicitness.

## Issues

Write every issue as though briefing someone brand new to the codebase. Concretely:

- **Title says where, not just what.** Prefix with the area in brackets — `[backend/notes]`, `[backend/srs]`, `[backend/study]`, `[frontend]`, `[editor]`, `[study]`, `[offline]`, `[ci]`, `[docs]`. A panel of issues should be navigable by title alone.
- **Context is mandatory.** One or two sentences of *why*. A human teammate can fill in unstated rationale from hallway context; an agent cannot.
- **Acceptance criteria are checkable bullets.** "Done" must be decidable without asking the author. If a criterion can't be phrased as a tickable box, the issue isn't ready.
- **State what's out of scope.** Agents (and enthusiastic humans) wander; an explicit fence is cheaper than review comments.
- **Give pointers.** Relevant files, docs, test fixtures, or an example of the pattern to follow. Embedding a short code sample of the shape you want is high-leverage.
- **Name the test layer.** Say which layer the work lands in — Go handler/service/repository tests in `backend/internal/`, whole-server HTTP tests in `backend/cmd/server/`, Vitest unit tests in `frontend/src/`, or the embedded-binary smoke run in `scripts/smoke.sh` — and what proves it works.
- **Scope narrowly.** "Fix the scheduling" is a bad issue; "carry the learning-step interval through undo in `backend/internal/study/service.go`" is a good one. Split rather than broaden.
- **Point at the conventions the work touches.** [`CLAUDE.md`](../CLAUDE.md) holds the standing ones — terse UI labels with no explanatory microcopy, no action implicitly bundled into another, card states as glyph components rather than text chips. An issue that would breach one should say so deliberately.

## PRs

A reviewer should find any answer in under a minute. Four short sections, optional ones deleted rather than left as "N/A":

- **What & why** — one or two sentences; link the issue (`Closes #N`) instead of repeating it.
- **How** — the approach, plus alternatives rejected and why. This is the part the diff can't show.
- **Test plan** — what was run and what proves it works, not just "tests pass".
- **Risk & rollback** — only when the change can break something at runtime. Migrations and anything touching the offline outbox or FSRS state always qualify.

Keep the diff focused: one issue per PR. If the work grew, split the PR rather than grow the description.

## Sources

Distilled from GitHub's coding-agent guidance ([assigning issues to coding agents](https://github.blog/ai-and-ml/github-copilot/assigning-and-completing-issues-with-coding-agent-in-github-copilot/), [WRAP up your backlog](https://github.blog/ai-and-ml/github-copilot/wrap-up-your-backlog-with-github-copilot-coding-agent/)) and standard PR-template practice ([Graphite](https://graphite.com/guides/github-pr-description-best-practices), [minware](https://www.minware.com/blog/effective-pr-template)).
