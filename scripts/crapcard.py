#!/usr/bin/env python3
"""Small command-line client for a CrapCard server (Python 3, standard library)."""

import argparse
import getpass
import http.cookiejar
import json
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request


class APIError(Exception):
    pass


class Client:
    def __init__(self, base_url, opener=None):
        parsed = urllib.parse.urlsplit(base_url)
        if parsed.scheme not in ("http", "https") or not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError("CRAPCARD_URL must be an http(s) URL without credentials, query, or fragment")
        self.base_url = base_url.rstrip("/") + "/"
        self.opener = opener or urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
        )

    def request(self, method, path, payload=None):
        url = urllib.parse.urljoin(self.base_url, "api/" + path)
        data = json.dumps(payload).encode("utf-8") if payload is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        try:
            with self.opener.open(req, timeout=20) as response:
                return json.load(response)
        except urllib.error.HTTPError as exc:
            try:
                message = json.load(exc).get("error", exc.reason)
            except (ValueError, AttributeError):
                message = exc.reason
            raise APIError(f"HTTP {exc.code}: {message}") from exc
        except urllib.error.URLError as exc:
            raise APIError(f"connection failed: {exc.reason}") from exc

    def login(self, username, password):
        self.request("POST", "auth/login", {"username": username, "password": password})


def positive_id(value):
    try:
        number = int(value)
        if number > 0:
            return number
    except ValueError:
        pass
    raise argparse.ArgumentTypeError("must be a positive integer")


def card_text(value, file, label):
    if value is not None and file is not None:
        raise ValueError(f"choose --{label} or --{label}-file")
    if file is not None:
        value = sys.stdin.read() if file == "-" else pathlib.Path(file).read_text(encoding="utf-8")
    if not value or not value.strip():
        raise ValueError(f"--{label} or --{label}-file is required")
    return value


def parser():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--url", default=os.environ.get("CRAPCARD_URL", "http://localhost:8080"), help="server URL (default: CRAPCARD_URL or http://localhost:8080)")
    commands = p.add_subparsers(dest="resource", required=True)

    decks = commands.add_parser("decks", help="list or create decks")
    deck_commands = decks.add_subparsers(dest="action", required=True)
    deck_commands.add_parser("list")
    create = deck_commands.add_parser("create")
    create.add_argument("name")
    create.add_argument("--description", default="")

    commands.add_parser("types", help="list available note types and fields")
    cards = commands.add_parser("cards", help="add, list, or preview cards")
    card_commands = cards.add_subparsers(dest="action", required=True)
    add = card_commands.add_parser("add")
    add.add_argument("--deck", type=positive_id, required=True, help="deck ID from decks list")
    for side in ("front", "back"):
        add.add_argument("--" + side, help=side + " markdown")
        add.add_argument("--" + side + "-file", help="read markdown from a UTF-8 file (or - for stdin)")
    add.add_argument("--reverse", action="store_true", help="also create a reverse card")
    listing = card_commands.add_parser("list")
    listing.add_argument("--deck", type=positive_id, required=True)
    listing.add_argument("--limit", type=positive_id, default=50)
    listing.add_argument("--offset", type=int, default=0)
    preview = card_commands.add_parser("preview")
    preview.add_argument("id", type=positive_id, help="note ID returned by cards add/list")
    return p


def run(args, client):
    if args.resource == "decks":
        if args.action == "list":
            return client.request("GET", "decks")
        return client.request("POST", "decks", {"name": args.name, "description": args.description})
    if args.resource == "types":
        return client.request("GET", "note-types")
    if args.action == "add":
        # A basic note is what the server accepts for ordinary two-sided cards.
        # It detects cloze markers in the front and produces cloze cards itself.
        if args.front_file == "-" and args.back_file == "-":
            raise ValueError("only one side may read from stdin")
        front = card_text(args.front, args.front_file, "front")
        back = card_text(args.back, args.back_file, "back")
        return client.request("POST", "notes", {
            "deck_id": args.deck, "type": "basic", "reversed": args.reverse,
            "fields": {"front": front, "back": back},
        })
    if args.action == "list":
        if args.offset < 0:
            raise ValueError("--offset must be nonnegative")
        query = urllib.parse.urlencode({"deck_id": args.deck, "limit": args.limit, "offset": args.offset})
        return client.request("GET", "notes?" + query)
    return client.request("GET", f"notes/{args.id}/preview")


def main(argv=None):
    args = parser().parse_args(argv)
    try:
        # Validate input before prompting for credentials or contacting the server.
        if args.resource == "cards" and args.action == "add":
            if args.front_file == "-" and args.back_file == "-":
                raise ValueError("only one side may read from stdin")
            args.front = card_text(args.front, args.front_file, "front")
            args.back = card_text(args.back, args.back_file, "back")
            args.front_file = args.back_file = None
        client = Client(args.url)
        username = os.environ.get("CRAPCARD_USERNAME") or input("Username: ")
        password = os.environ.get("CRAPCARD_PASSWORD") or getpass.getpass("Password: ")
        client.login(username, password)
        print(json.dumps(run(args, client), indent=2, ensure_ascii=False))
        return 0
    except (APIError, ValueError, OSError, EOFError, KeyboardInterrupt) as exc:
        print(f"crapcard: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
