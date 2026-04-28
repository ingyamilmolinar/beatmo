package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// HookAction is a typed enumeration of supported hook action names.
// Wire format stays string-based (HookRule.Do is a JSON string), but
// the dispatcher operates on this type so typo'd actions surface as
// "unknown" rather than silently dispatching the default branch.
type HookAction string

const (
	ActionLog       HookAction = "log"
	ActionExit      HookAction = "exit"
	ActionDumpPProf HookAction = "dump_pprof"
)

// KnownActions returns the supported action set. Order is the
// dispatcher's canonical order (log → exit → dump_pprof) so callers
// rendering documentation get a stable list.
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

// ParseHooksConfig parses raw JSON bytes into a HooksConfig. Errors are
// wrapped so the call site can prefix them ("hooks-config: ..."). The
// returned config is always usable — empty Rules is valid and means
// "config loaded but nothing to subscribe."
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

// ActionRunner is the callback invoked when a subscribed event fires.
// In production the runner dispatches "log" / "exit" / "dump_pprof";
// tests inject a recorder instead so the dispatcher's switch can be
// exercised without side effects.
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

// SubscribeRules registers each rule with the given hooks bus. Returns a
// cleanup function that unsubscribes every registration; safe to call
// multiple times.
//
// The runner callback is invoked synchronously from each subscriber, so
// long-running actions should hand off to a goroutine themselves (the
// bus already runs subscribers on a worker pool, but actions like
// dump_pprof want to free the worker).
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
				log.Infof("[HOOKS] %s → %s payload=%v", e.Kind, action, e.Payload)
			}
			if runner != nil {
				runner(action, e)
			}
		})
		unsubs = append(unsubs, unsub)
		if log != nil {
			log.Infof("[HOOKS] subscribed: on=%s do=%s", rule.On, rule.Do)
		}
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}
