package bench

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// ─── EnvInt ───────────────────────────────────────────────────────────────

func TestEnvIntBehavior(t *testing.T) {
	const key = "__BEATMO_BENCH_TEST_ENVINT__"
	cases := []struct {
		set  bool
		val  string
		def  int
		want int
		desc string
	}{
		{set: false, def: 7, want: 7, desc: "unset → default"},
		{set: true, val: "", def: 7, want: 7, desc: "empty → default (unset semantically)"},
		{set: true, val: "5", def: 7, want: 5, desc: "valid positive"},
		{set: true, val: "0", def: 7, want: 7, desc: "zero is treated as unset"},
		{set: true, val: "-3", def: 7, want: 7, desc: "negative falls back"},
		{set: true, val: "abc", def: 7, want: 7, desc: "non-numeric falls back"},
		{set: true, val: "100", def: 7, want: 100, desc: "large positive ok"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			if c.set {
				t.Setenv(key, c.val)
			} else {
				_ = os.Unsetenv(key)
			}
			if got := EnvInt(key, c.def); got != c.want {
				t.Errorf("EnvInt(%q, %d)=%d want %d", c.val, c.def, got, c.want)
			}
		})
	}
}

// ─── ParseHooksConfig / LoadHooksConfig ───────────────────────────────────

func TestParseHooksConfigValid(t *testing.T) {
	body := []byte(`{
		"rules": [
			{"on": "record.start", "do": "log"},
			{"on": "record.stop", "do": "exit"}
		]
	}`)
	cfg, err := ParseHooksConfig(body)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("rules: %d want 2", len(cfg.Rules))
	}
	if cfg.Rules[0].On != "record.start" || cfg.Rules[0].Do != "log" {
		t.Errorf("first rule: %+v", cfg.Rules[0])
	}
	if cfg.Rules[1].On != "record.stop" || cfg.Rules[1].Do != "exit" {
		t.Errorf("second rule: %+v", cfg.Rules[1])
	}
}

func TestParseHooksConfigEmpty(t *testing.T) {
	// "{}" or "{\"rules\": []}" — both must parse and yield an empty rule set.
	for _, body := range [][]byte{[]byte("{}"), []byte(`{"rules": []}`)} {
		cfg, err := ParseHooksConfig(body)
		if err != nil {
			t.Errorf("err=%v body=%s", err, body)
		}
		if len(cfg.Rules) != 0 {
			t.Errorf("rules=%d want 0 for body %s", len(cfg.Rules), body)
		}
	}
}

func TestParseHooksConfigBadJSON(t *testing.T) {
	for _, body := range [][]byte{
		[]byte("{ this is not valid"),
		[]byte("[1, 2, 3]"), // wrong shape
		[]byte("null"),       // null isn't a struct
	} {
		// Note: "null" actually decodes successfully (Go json sees it as
		// a zero-valued struct) — accept either nil-error or error here,
		// but rules must be empty.
		cfg, err := ParseHooksConfig(body)
		if err == nil && len(cfg.Rules) != 0 {
			t.Errorf("body=%q: silently accepted with %d rules", body, len(cfg.Rules))
		}
	}
}

func TestLoadHooksConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	body := []byte(`{"rules": [{"on": "x", "do": "log"}]}`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg, err := LoadHooksConfig(path)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Errorf("rules: %+v", cfg.Rules)
	}
}

func TestLoadHooksConfigMissingFile(t *testing.T) {
	if _, err := LoadHooksConfig("/__no_such_file__.json"); err == nil {
		t.Errorf("want error for missing file")
	}
}

// ─── ActionCounter ────────────────────────────────────────────────────────

func TestActionCounterIncrementsConcurrently(t *testing.T) {
	c := &ActionCounter{}
	const N = 1000
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range N {
				c.Inc("log")
			}
		}()
	}
	wg.Wait()
	if got := c.Count("log"); got != 4*N {
		t.Errorf("log=%d want %d", got, 4*N)
	}
	if got := c.Count("never_incremented"); got != 0 {
		t.Errorf("zero-default broken: %d", got)
	}
}

// ─── SubscribeRules ───────────────────────────────────────────────────────

// recordingLogger collects log calls so tests can assert on them.
type recordingLogger struct {
	mu   sync.Mutex
	info []string
	errs []string
}

func (r *recordingLogger) Infof(f string, args ...any) {
	r.mu.Lock()
	r.info = append(r.info, f)
	r.mu.Unlock()
}
func (r *recordingLogger) Errorf(f string, args ...any) {
	r.mu.Lock()
	r.errs = append(r.errs, f)
	r.mu.Unlock()
}

func TestSubscribeRulesRoutesEventsToRunner(t *testing.T) {
	log := &recordingLogger{}
	counter := &ActionCounter{}

	var ranAction atomic.Value // string
	rules := []HookRule{
		{On: string(hooks.EventSeek), Do: "log"},
	}
	cleanup := SubscribeRules(rules, log, counter, func(action string, e hooks.Event) {
		ranAction.Store(action)
	})
	t.Cleanup(cleanup)

	// Publish on the global bus. The subscriber runs on a worker pool;
	// poll briefly until the runner records.
	hooks.PublishKind(hooks.EventSeek, hooks.SeekPayload{Beats: 5})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if v, _ := ranAction.Load().(string); v == "log" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, _ := ranAction.Load().(string); got != "log" {
		t.Errorf("runner not invoked with 'log'; got %q", got)
	}
	if c := counter.Count("log"); c != 1 {
		t.Errorf("counter[log]=%d want 1", c)
	}

	// Logger received the subscribe-line + per-event line.
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.info) < 2 {
		t.Errorf("logger only saw %d info lines: %+v", len(log.info), log.info)
	}
}

func TestSubscribeRulesCleanupStopsDelivery(t *testing.T) {
	log := &recordingLogger{}
	counter := &ActionCounter{}

	rules := []HookRule{{On: string(hooks.EventSeek), Do: "log"}}
	cleanup := SubscribeRules(rules, log, counter, nil)
	cleanup() // immediately

	hooks.PublishKind(hooks.EventSeek, hooks.SeekPayload{Beats: 1})

	// Wait briefly to give any latent dispatch a chance.
	time.Sleep(60 * time.Millisecond)
	if got := counter.Count("log"); got != 0 {
		t.Errorf("counter incremented after cleanup: %d", got)
	}
}

func TestSubscribeRulesNilLoggerAndCounterAreSafe(t *testing.T) {
	rules := []HookRule{{On: string(hooks.EventSeek), Do: "log"}}
	// Nil logger, counter, and runner — all fields tolerated.
	cleanup := SubscribeRules(rules, nil, nil, nil)
	t.Cleanup(cleanup)
	hooks.PublishKind(hooks.EventSeek, hooks.SeekPayload{Beats: 1})
	// Just survive — give the bus a moment to deliver.
	time.Sleep(30 * time.Millisecond)
}
