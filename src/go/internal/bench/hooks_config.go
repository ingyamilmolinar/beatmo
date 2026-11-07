package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// The wire format stays a string; the dispatcher uses this type so a typo reports "unknown" instead of taking the default
// branch.
type HookAction string

const (
	ActionLog       HookAction = "log"
	ActionExit      HookAction = "exit"
	ActionDumpPProf HookAction = "dump_pprof"
)

// The dispatcher's canonical order, so rendered documentation is stable.
func KnownActions() []HookAction {
	return []HookAction{ActionLog, ActionExit, ActionDumpPProf}
}

// IsKnownAction reports whether s names a supported hook action.
// Use to validate hooks-config files at load time and to guard the
// runtime dispatcher.
func IsKnownAction(s string) bool {
	switch HookAction(s) {
	case ActionLog, ActionExit, ActionDumpPProf:
		return true
	}
	return false
}

// HookRule pairs an event kind ("on") with a built-in action name ("do").
// Supported actions: see KnownActions.
type HookRule struct {
	On string `json:"on"`
	Do string `json:"do"`
}

// HooksConfig is the on-disk JSON schema for the -hooks-config flag.
type HooksConfig struct {
	Rules []HookRule `json:"rules"`
}

// Errors are wrapped for the caller's prefix; empty Rules is valid ("loaded, nothing to subscribe").
func ParseHooksConfig(data []byte) (HooksConfig, error) {
	var cfg HooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return HooksConfig{}, fmt.Errorf("parse: %w", err)
	}
	return cfg, nil
}

// LoadHooksConfig reads the file at path and parses it. Convenience
// wrapper around os.ReadFile + ParseHooksConfig.
func LoadHooksConfig(path string) (HooksConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HooksConfig{}, fmt.Errorf("read: %w", err)
	}
	return ParseHooksConfig(data)
}

// ActionLogger is the minimal logging surface SubscribeRules needs. It is
// intentionally narrower than internal/log so tests can substitute a
// recorder.
type ActionLogger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// Injected so tests exercise the dispatcher's switch without side effects.
type ActionRunner func(action string, e hooks.Event)

// ActionCounter is a process-wide counter map for hook actions. The
// concrete type is an atomic-friendly sync.Map so multiple goroutines
// can increment without contention.
type ActionCounter struct{ m sync.Map }

// Inc increments the counter for action by 1.
func (c *ActionCounter) Inc(action string) {
	v, _ := c.m.LoadOrStore(action, new(atomic.Int64))
	v.(*atomic.Int64).Add(1)
}

// Count returns the current value for action.
func (c *ActionCounter) Count(action string) int64 {
	v, ok := c.m.Load(action)
	if !ok {
		return 0
	}
	return v.(*atomic.Int64).Load()
}

// The runner is called synchronously from each subscriber, so a long action (dump_pprof) hands off to its own goroutine to
// free the bus worker. The cleanup is safe to call more than once.
func SubscribeRules(rules []HookRule, log ActionLogger, counter *ActionCounter, runner ActionRunner) (cleanup func()) {
	unsubs := make([]func(), 0, len(rules))
	for _, rule := range rules {
		kind := hooks.Kind(rule.On)
		action := rule.Do
		unsub := hooks.Subscribe(kind, func(e hooks.Event) {
			if counter != nil {
				counter.Inc(action)
			}
			if log != nil {
				log.Infof("[hooks] %s → %s payload=%v", e.Kind, action, e.Payload)
			}
			if runner != nil {
				runner(action, e)
			}
		})
		unsubs = append(unsubs, unsub)
		if log != nil {
			log.Infof("[hooks] subscribed: on=%s do=%s", rule.On, rule.Do)
		}
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}
