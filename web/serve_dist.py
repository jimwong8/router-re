#!/usr/bin/env python3
"""Serve the built SPA (dist/) and reverse-proxy /api/v1 to the mgmtd backend.

Used for UI verification when `vite preview` stalls on large responses.
"""
import http.server
import socketserver
import urllib.request
import urllib.error
import os
import sys

DIST = os.path.join(os.path.dirname(os.path.abspath(__file__)), "dist")
BACKEND = "http://127.0.0.1:9090"


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIST, **kwargs)

    def _proxy(self, method):
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else None
        req = urllib.request.Request(BACKEND + self.path, data=body, method=method)
        for h in ("Content-Type", "Authorization"):
            if self.headers.get(h):
                req.add_header(h, self.headers[h])
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = resp.read()
                self.send_response(resp.status)
                self.send_header("Content-Type", resp.headers.get("Content-Type", "application/json"))
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
        except urllib.error.HTTPError as e:
            data = e.read()
            self.send_response(e.code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except Exception as e:
            msg = str(e).encode()
            self.send_response(502)
            self.send_header("Content-Length", str(len(msg)))
            self.end_headers()
            self.wfile.write(msg)

    def do_GET(self):
        if self.path.startswith("/api/v1"):
            return self._proxy("GET")
        # SPA fallback: unknown non-asset paths render index.html
        if not self.path.startswith("/assets") and "." not in self.path.split("/")[-1]:
            self.path = "/index.html"
        return super().do_GET()

    def do_POST(self):
        if self.path.startswith("/api/v1"):
            return self._proxy("POST")
        self.send_error(404)

    def log_message(self, format, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 4173
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.TCPServer(("127.0.0.1", port), Handler) as httpd:
        print(f"serving {DIST} on http://127.0.0.1:{port} (proxy -> {BACKEND})")
        httpd.serve_forever()
