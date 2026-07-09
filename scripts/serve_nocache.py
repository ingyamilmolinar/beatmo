#!/usr/bin/env python3
"""No-cache static server for `make serve`.

Plain `python3 -m http.server` sends no Cache-Control, so a browser happily
serves a STALE audio.js / drums.single.js / main.wasm after a rebuild — you run
`make wasm` but the page keeps running old code ("I rebuilt but it's still
broken"). This handler forces no-store on every response and pins the JS/WASM
MIME types (Python's mimetypes db pre-3.12 returns application/octet-stream for
.wasm, which can break WebAssembly.instantiateStreaming on some browsers).

Usage: cd src/js && python3 ../scripts/serve_nocache.py [PORT]
"""
import http.server
import socketserver
import sys

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8080


class NoCacheHandler(http.server.SimpleHTTPRequestHandler):
    extensions_map = {
        **http.server.SimpleHTTPRequestHandler.extensions_map,
        ".wasm": "application/wasm",
        ".js": "application/javascript",
        ".mjs": "application/javascript",
    }

    def end_headers(self):
        self.send_header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
        self.send_header("Pragma", "no-cache")
        self.send_header("Expires", "0")
        super().end_headers()


socketserver.TCPServer.allow_reuse_address = True
with socketserver.TCPServer(("", PORT), NoCacheHandler) as httpd:
    print(f"serving {sys.path[0] and '.'} on http://localhost:{PORT} (no-cache, .wasm=application/wasm)")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
