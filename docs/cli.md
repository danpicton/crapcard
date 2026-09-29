# CrapCard CLI

The CLI uses the same HTTP API as the web app. It needs Python 3 and no packages.
Run it against a running CrapCard server:

```bash
export CRAPCARD_URL=https://cards.example.com
export CRAPCARD_USERNAME=dan
python3 scripts/crapcard.py decks list
```

The CLI prompts for a password each time. For noninteractive use, set
`CRAPCARD_PASSWORD` in the environment. It logs in for each invocation and keeps
the session cookie in memory; it does not save credentials or cookies to disk.
Use HTTPS when connecting to a remote server.

```bash
python3 scripts/crapcard.py decks create 'Spanish' --description 'Vocabulary'
python3 scripts/crapcard.py types
python3 scripts/crapcard.py cards add --deck 1 --front 'hola' --back 'hello'
python3 scripts/crapcard.py cards add --deck 1 --front 'hola' --back 'hello' --reverse
python3 scripts/crapcard.py cards list --deck 1 --limit 50 --offset 0
python3 scripts/crapcard.py cards preview 17
```

`cards add` creates a note, which produces one card by default or two with
`--reverse`. Its output includes the note ID and generated card IDs. The front
and back accept Markdown. For multiline content, use `--front-file` and
`--back-file`; either file may be `-` to read from stdin, but only one side can
read stdin per call. The server detects `{{c1::answer}}` markers in the front
and creates cloze cards from them; `--reverse` applies to basic notes only.
The server requires a nonempty front and back for a basic card.

All commands print JSON. `cards list` returns the API's paginated note envelope
(`items`, `total`, `limit`, `offset`); the `id` in each item is a note ID and
can be passed to `cards preview`. Errors go to stderr and return a nonzero exit
code. Run `python3 scripts/crapcard.py --help` for options.

Tests: `python3 -m unittest discover -s scripts -p 'test_crapcard.py'`.
