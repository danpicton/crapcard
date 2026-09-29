import contextlib
import http.server
import importlib.util
import io
import json
import pathlib
import threading
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location("crapcard_cli", pathlib.Path(__file__).with_name("crapcard.py"))
cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cli)


class Handler(http.server.BaseHTTPRequestHandler):
    calls = []

    def log_message(self, *_):
        pass

    def respond(self, code, data, cookie=False):
        body = json.dumps(data).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        if cookie:
            self.send_header("Set-Cookie", "session=test; Path=/; HttpOnly; SameSite=Strict")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def handle_request(self):
        size = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(size)) if size else None
        self.calls.append((self.command, self.path, self.headers.get("Cookie"), body))
        if self.path == "/api/auth/login":
            if body != {"username": "dan", "password": "secret"}:
                return self.respond(401, {"error": "invalid credentials"})
            return self.respond(200, {"ok": True}, cookie=True)
        if self.headers.get("Cookie") != "session=test":
            return self.respond(401, {"error": "authentication required"})
        if self.path == "/api/decks" and self.command == "GET":
            return self.respond(200, [{"id": 4, "name": "Languages"}])
        if self.path == "/api/decks" and self.command == "POST":
            return self.respond(201, {"id": 5, **body})
        if self.path == "/api/note-types":
            return self.respond(200, [{"type": "basic", "fields": ["front", "back"]}])
        if self.path == "/api/notes" and self.command == "POST":
            return self.respond(201, {"id": 17, "cards": [{"id": 31}], **body})
        if self.path.startswith("/api/notes?"):
            return self.respond(200, {"items": [], "total": 0})
        if self.path == "/api/notes/17/preview":
            return self.respond(200, [{"question": "one", "answer": "two"}])
        self.respond(404, {"error": "no such endpoint"})

    do_GET = handle_request
    do_POST = handle_request


class CLITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def setUp(self):
        Handler.calls.clear()

    def invoke(self, *arguments, password="secret"):
        out, err = io.StringIO(), io.StringIO()
        with mock.patch.dict("os.environ", {"CRAPCARD_USERNAME": "dan", "CRAPCARD_PASSWORD": password}), \
                contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = cli.main(["--url", self.url, *arguments])
        return code, out.getvalue(), err.getvalue()

    def test_add_uses_session_and_note_payload(self):
        code, output, error = self.invoke("cards", "add", "--deck", "4", "--front", "one", "--back", "two", "--reverse")
        self.assertEqual((code, error), (0, ""))
        self.assertEqual(json.loads(output)["id"], 17)
        self.assertEqual(Handler.calls[1], ("POST", "/api/notes", "session=test", {
            "deck_id": 4, "type": "basic", "reversed": True,
            "fields": {"front": "one", "back": "two"},
        }))

    def test_supporting_commands(self):
        for arguments, expected in [
            (("decks", "list"), "/api/decks"),
            (("decks", "create", "New", "--description", "x"), "/api/decks"),
            (("types",), "/api/note-types"),
            (("cards", "list", "--deck", "4", "--limit", "10", "--offset", "20"), "/api/notes?deck_id=4&limit=10&offset=20"),
            (("cards", "preview", "17"), "/api/notes/17/preview"),
        ]:
            with self.subTest(arguments=arguments):
                Handler.calls.clear()
                code, output, error = self.invoke(*arguments)
                self.assertEqual((code, error), (0, ""))
                self.assertIsNotNone(json.loads(output))
                self.assertEqual(Handler.calls[1][1], expected)

    def test_errors_and_invalid_input(self):
        code, _, error = self.invoke("decks", "list", password="wrong")
        self.assertEqual(code, 1)
        self.assertIn("HTTP 401: invalid credentials", error)
        self.assertEqual(len(Handler.calls), 1)
        Handler.calls.clear()
        code, _, error = self.invoke("cards", "add", "--deck", "4", "--front", "one")
        self.assertEqual(code, 1)
        self.assertIn("--back or --back-file is required", error)
        self.assertEqual(Handler.calls, [])


if __name__ == "__main__":
    unittest.main()
