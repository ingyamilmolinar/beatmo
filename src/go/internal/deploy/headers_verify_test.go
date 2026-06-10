package deploy

// Wraps scripts/verify_deploy_headers.sh with two kinds of automated coverage:
//
//   1. A self-contained test that spawns a Go HTTP test server emulating the
//      deployed bucket (correct + broken header variants) and asserts the
//      script returns 0 on the correct case and non-zero on each broken case.
//      This runs by default in the fast test path.
//
//   2. An opt-in live probe gated by BEATMO_VERIFY_DEPLOY_URL: when set, the
//      same script runs against the real deployed URL. Skipped otherwise.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allDeployedFiles returns the files the verify script will probe -- mirrors
// the bash arrays in scripts/verify_deploy_headers.sh.
func allDeployedFiles() []string {
	return []string{
		"index.html",
		"synth_param_abi.gen.js",
		"audio.js",
		"wasm_exec.js",
		"drums.single.js",
		"insert_fx_worklet.js",
		"recording_capture_worklet.js",
		"recording_encoder_worker.js",
		"main.wasm",
	}
}

func contentTypeFor(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".wasm"):
		return "application/wasm"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	}
	return "application/octet-stream"
}

// startFakeBucket spins up an httptest server that serves the deploy manifest
// with header behavior controlled by `override`. The override callback may
// rewrite Content-Type / Cache-Control to simulate a regression.
func startFakeBucket(t *testing.T, override func(rel string, h http.Header)) *httptest.Server {
	t.Helper()
	files := allDeployedFiles()
	known := make(map[string]bool, len(files))
	for _, f := range files {
		known[f] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel == "" {
			rel = "index.html"
		}
		if !known[rel] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentTypeFor(rel))
		// Default Cache-Control mirrors the deploy script's choices.
		if rel == "index.html" || strings.HasSuffix(rel, ".gen.js") {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
		}
		if override != nil {
			override(rel, w.Header())
		}
		w.WriteHeader(http.StatusOK)
		// Body content doesn't matter -- the script uses HEAD.
	}))
	t.Cleanup(srv.Close)
	return srv
}

func verifyScriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(root, "scripts", "verify_deploy_headers.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("verify script missing: %v", err)
	}
	return p
}

// runVerify runs the script with BEATMO_VERIFY_DEPLOY_URL=url and returns
// (combinedOutput, err).
func runVerify(t *testing.T, scriptPath, url string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not on PATH: %v", err)
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skipf("curl not on PATH: %v", err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(), "BEATMO_VERIFY_DEPLOY_URL="+url)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestVerifyHeadersScript_PassesAgainstCorrectBucket(t *testing.T) {
	script := verifyScriptPath(t)
	srv := startFakeBucket(t, nil)
	out, err := runVerify(t, script, srv.URL)
	if err != nil {
		t.Fatalf("verify script failed against correct bucket:\n%s\nerr=%v", out, err)
	}
	if !strings.Contains(out, "[verify] OK") {
		t.Errorf("expected [verify] OK marker, got:\n%s", out)
	}
}

func TestVerifyHeadersScript_FlagsWrongWASMContentType(t *testing.T) {
	script := verifyScriptPath(t)
	srv := startFakeBucket(t, func(rel string, h http.Header) {
		if rel == "main.wasm" {
			// Simulate the historical GCS bug: WASM served as octet-stream.
			h.Set("Content-Type", "application/octet-stream")
		}
	})
	out, err := runVerify(t, script, srv.URL)
	if err == nil {
		t.Fatalf("expected non-zero exit when main.wasm has wrong MIME; got OK:\n%s", out)
	}
	if !strings.Contains(out, "main.wasm") || !strings.Contains(out, "application/wasm") {
		t.Errorf("error output must mention main.wasm + application/wasm, got:\n%s", out)
	}
}

func TestVerifyHeadersScript_FlagsMissingCacheControl(t *testing.T) {
	script := verifyScriptPath(t)
	srv := startFakeBucket(t, func(rel string, h http.Header) {
		h.Del("Cache-Control")
	})
	out, err := runVerify(t, script, srv.URL)
	if err == nil {
		t.Fatalf("expected non-zero exit when Cache-Control missing; got OK:\n%s", out)
	}
	if !strings.Contains(out, "Cache-Control") {
		t.Errorf("error output must mention Cache-Control, got:\n%s", out)
	}
}

func TestVerifyHeadersScript_Flags404(t *testing.T) {
	script := verifyScriptPath(t)
	// Bucket that 404s the recording worker -- simulates a missing-from-deploy
	// regression that survived the manifest drift test (defense in depth).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/recording_encoder_worker.js") {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel == "" {
			rel = "index.html"
		}
		w.Header().Set("Content-Type", contentTypeFor(rel))
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	out, err := runVerify(t, script, srv.URL)
	if err == nil {
		t.Fatalf("expected non-zero exit when an asset 404s; got OK:\n%s", out)
	}
	if !strings.Contains(out, "recording_encoder_worker.js") {
		t.Errorf("error output must name the missing file, got:\n%s", out)
	}
}

// TestVerifyHeadersScript_LiveProbe is an opt-in check that runs against the
// real deployed URL. Set BEATMO_VERIFY_DEPLOY_URL to enable.
func TestVerifyHeadersScript_LiveProbe(t *testing.T) {
	url := os.Getenv("BEATMO_VERIFY_DEPLOY_URL")
	if url == "" {
		t.Skip("BEATMO_VERIFY_DEPLOY_URL not set; live probe skipped")
	}
	script := verifyScriptPath(t)
	out, err := runVerify(t, script, url)
	if err != nil {
		t.Fatalf("live probe of %s failed:\n%s", url, out)
	}
	t.Logf("live probe OK against %s:\n%s", url, out)
}
