package engine

import "time"

// StartBackground spawns a low-priority goroutine that keeps predictions
// ahead of the UI's needs. target returns the desired horizon; the
// predictor converges towards it on an 8ms cadence.
//
// Idempotent: concurrent or repeat calls update the target function but
// do not spawn a second worker. After StopBackground returns, the
// predictor is ready for a fresh StartBackground call.
func (p *Predictor) StartBackground(target func() int) {
	p.mu.Lock()
	p.targetFn = target
	p.mu.Unlock()

	if !p.bgRunning.CompareAndSwap(false, true) {
		return // worker already running; targetFn was just updated above
	}

	p.mu.Lock()
	p.bgStopped = false
	p.bgQuit = make(chan struct{})
	p.bgDone = make(chan struct{})
	quit, done := p.bgQuit, p.bgDone
	p.mu.Unlock()

	go p.runBackground(quit, done)
}

// runBackground is the precompute loop. Exits when quit is closed.
func (p *Predictor) runBackground(quit, done chan struct{}) {
	ticker := time.NewTicker(8 * time.Millisecond)
	defer func() {
		ticker.Stop()
		if done != nil {
			close(done)
		}
	}()
	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			p.mu.RLock()
			stopped := p.bgStopped
			tf := p.targetFn
			cur := p.horizon
			p.mu.RUnlock()
			if stopped {
				return
			}
			if tf == nil {
				continue
			}
			want := tf()
			if want > cur {
				step := min(cur+256, want)
				p.Ensure(step)
			}
		}
	}
}

// StopBackground stops the background precompute loop and clears the
// running gate so a future StartBackground call spawns a fresh worker.
// Safe to call when no worker is running (no-op).
func (p *Predictor) StopBackground() {
	p.mu.Lock()
	p.bgStopped = true
	quit := p.bgQuit
	done := p.bgDone
	p.mu.Unlock()
	if quit != nil {
		select {
		case <-quit:
		default:
			close(quit)
		}
	}
	if done != nil {
		<-done
	}
	// Clear the gate AFTER the worker has acknowledged exit so a racing
	// Start cannot spawn a second worker before this one is fully gone.
	p.bgRunning.Store(false)
}

// SetBackgroundTarget assigns the background target function without starting
// the goroutine. This is used by tests that want to evaluate the target logic
// without spinning a worker.
func (p *Predictor) SetBackgroundTarget(target func() int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targetFn = target
}

// BackgroundTargetForTest returns the current background target function so
// tests can evaluate the requested horizon without invoking the goroutine.
func (p *Predictor) BackgroundTargetForTest() func() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targetFn
}
