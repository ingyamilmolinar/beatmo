package engine

import "time"

// StartBackground spawns a low-priority goroutine to keep predictions ahead.
// target returns a desired horizon; the predictor will gradually converge.
func (p *Predictor) StartBackground(target func() int) {
	p.mu.Lock()
	p.targetFn = target
	// Reset stopped flag and done channel for new worker
	p.bgStopped = false
	// Recreate quit channel if it was closed.
	select {
	case <-p.bgQuit:
		p.bgQuit = make(chan struct{})
	default:
		if p.bgQuit == nil {
			p.bgQuit = make(chan struct{})
		}
	}
	p.bgDone = make(chan struct{})
	quit := p.bgQuit
	done := p.bgDone
	p.mu.Unlock()
	go func() {
		// Low priority loop; ~8ms cadence keeps ahead without impacting UI.
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
					step := cur + 256
					if step > want {
						step = want
					}
					p.Ensure(step)
				}
			}
		}
	}()
}

// StopBackground stops the background precompute loop.
func (p *Predictor) StopBackground() {
	p.mu.Lock()
	// Mark stopped so the worker can bail even if a ticker tick raced with quit.
	p.bgStopped = true
	quit := p.bgQuit
	done := p.bgDone
	p.mu.Unlock()
	// Close quit channel once.
	if quit != nil {
		select {
		case <-quit:
		default:
			close(quit)
		}
	}
	// Wait for worker to acknowledge exit to avoid post-close work.
	if done != nil {
		<-done
	}
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
