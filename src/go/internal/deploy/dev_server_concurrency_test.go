package deploy

// `make serve` (scripts/serve_nocache.py) must answer a request while another
// response is still in flight.
//
// index.html's FIRST <head> script starts fetch("main.wasm") so the download
// overlaps the rest of the boot (landing page spec, "boot order is
// game-first"). Nothing reads that response until audio.js and its static
// imports have loaded, and Chromium applies backpressure to an unread body:
// it stops reading the socket after a few MB. A server that handles ONE
// connection at a time is then stuck writing the wasm, never serves audio.js's
// imports (synth_param_abi.gen.js, chain_spec.gen.js, dsp_host.js),
// and the module that would consume the wasm never runs. The page sits on
// "Loading..." forever with no console error. Firefox and WebKit buffer the
// whole body, so they boot; production (HTTP/2) and every browser-test server
// (Node) serve concurrently, so nothing else saw it.
//
// This test reproduces Chromium's behaviour without a browser: one client
// requests a large file and never reads it, a second client asks for a small
// file and must get it promptly. Browser end-to-end:
// src/js/dev_server_boot.browser.test.js.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevServerServesWhileAnotherResponseIsUnread(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH: `make serve` cannot run here either")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "serve_nocache.py")

	dir := t.TempDir()
	// Far larger than the kernel's loopback socket buffers, like main.wasm
	// (~30 MB): a writer that is not read from blocks well before the end.
	if err := os.WriteFile(filepath.Join(dir, "main.wasm"), make([]byte, 64<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audio.js"), []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	port := freePort(t)
	cmd := exec.Command(python, script, fmt.Sprint(port))
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", script, err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitListening(t, addr)

	// Client 1: the early wasm fetch. Request it, read the status line, then
	// stop reading -- what Chromium does until instantiateStreaming starts.
	stalled, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stalled.Close() })
	fmt.Fprintf(stalled, "GET /main.wasm HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_ = stalled.SetReadDeadline(time.Now().Add(5 * time.Second))
	status, err := bufio.NewReader(stalled).ReadString('\n')
	if err != nil {
		t.Fatalf("wasm request got no status line: %v", err)
	}
	t.Logf("unread wasm response: %q", status)
	// Give the server time to fill the socket buffers and block in write().
	time.Sleep(300 * time.Millisecond)

	// Client 2: one of audio.js's static imports.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/audio.js")
	if err != nil {
		t.Fatalf("serve_nocache.py did not answer GET /audio.js while the main.wasm response was unread: %v\n"+
			"A one-connection-at-a-time server deadlocks the page in Chromium (index.html fetches main.wasm "+
			"before audio.js's imports load): use a threading server.", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "export {};\n" {
		t.Fatalf("GET /audio.js = %d %q, want 200 with the file", resp.StatusCode, body)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("serve_nocache.py never listened on %s", addr)
}
