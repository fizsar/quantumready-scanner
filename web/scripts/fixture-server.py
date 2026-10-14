"""Serve the built frontend (web/dist) with fixture API answers, for screenshots.

    python3 -I web/scripts/fixture-server.py web/dist web/src/test/lab 8099

GET /api/scans/<name>?lang=xx answers <fixtures>/<name>.<xx>.json; any other
path serves a file from dist or index.html (SPA routes). Listens on 127.0.0.1
only. Never talks to the real API.
"""

import http.server
import pathlib
import sys
import urllib.parse

dist, fixtures, port = pathlib.Path(sys.argv[1]).resolve(), pathlib.Path(sys.argv[2]).resolve(), int(sys.argv[3])


class Handler(http.server.BaseHTTPRequestHandler):
    def send(self, code, body, ctype):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        url = urllib.parse.urlparse(self.path)
        if url.path.startswith("/api/scans/"):
            name = url.path.rsplit("/", 1)[-1]
            lang = urllib.parse.parse_qs(url.query).get("lang", ["es"])[0]
            f = (fixtures / f"{name}.{lang}.json").resolve()
            if f.parent == fixtures and f.is_file():
                return self.send(200, f.read_bytes(), "application/json")
            return self.send(404, b'{"error":{"code":"not_found","message":"not found"}}', "application/json")
        f = (dist / url.path.lstrip("/")).resolve()
        if not (f.is_file() and dist in f.parents):
            f = dist / "index.html"
        ctype = {".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".html": "text/html"}.get(f.suffix, "application/octet-stream")
        self.send(200, f.read_bytes(), ctype)

    def log_message(self, *args):
        pass


http.server.ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
