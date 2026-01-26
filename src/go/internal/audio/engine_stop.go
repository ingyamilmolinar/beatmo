//go:build !test && !js

package audio

func (m *mixer) Stop(id string) {
	m.mu.Lock()
	// Use swap-and-truncate to avoid allocations
	for i := 0; i < len(m.voices); {
		if m.voices[i].id == id {
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices = m.voices[:len(m.voices)-1]
			continue
		}
		i++
	}
	m.mu.Unlock()
}

// Read implements io.Reader for oto.Player.
// Uses block-based processing to minimize lock overhead.
func (m *mixer) Read(p []byte) (int, error) {
	samples := len(p) / 2

	// Drain pending voices with quick lock
	m.pendingMu.Lock()
	if len(m.pendingAdd) > 0 {
		m.mu.Lock()
		m.voices = append(m.voices, m.pendingAdd...)
		m.mu.Unlock()
		m.pendingAdd = m.pendingAdd[:0]
	}
	m.pendingMu.Unlock()

	// Process in blocks
	offset := 0
	for offset < samples {
		blockLen := blockSize
		if offset+blockLen > samples {
			blockLen = samples - offset
		}

		// Lock ONCE for entire block
		m.mu.Lock()
		m.processBlock(offset, blockLen, p)
		m.mu.Unlock()

		offset += blockLen
	}
	return len(p), nil
}

// processBlock processes blockLen samples starting at offset.
// Caller must hold m.mu.
func (m *mixer) processBlock(offset, blockLen int, p []byte) {
	// Lazily initialize work buffers (needed for tests that create mixer{} directly)
	if m.workBuf == nil {
		m.workBuf = make([]float64, blockSize)
		m.voiceTemp = make([]float64, blockSize)
	}

	// Zero work buffer
	for i := 0; i < blockLen; i++ {
		m.workBuf[i] = 0
	}

	// Process voices - use swap-and-truncate to avoid allocations
	i := 0
	for i < len(m.voices) {
		vs := m.voices[i]
		startPos := m.pos + offset

		if startPos < vs.start {
			i++
			continue
		}

		done := m.renderVoiceBlock(vs, blockLen)
		if done {
			// Swap with last and truncate (no allocation)
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices = m.voices[:len(m.voices)-1]
			// Don't increment i - check swapped element
		} else {
			i++
		}
	}

	// Convert to int16 output
	for i := 0; i < blockLen; i++ {
		sum := m.workBuf[i]
		if sum > 1 {
			sum = 1
		} else if sum < -1 {
			sum = -1
		}
		v := int16(sum * 32767)
		outIdx := (offset + i) * 2
		p[outIdx] = byte(v)
		p[outIdx+1] = byte(v >> 8)
	}

	m.pos += blockLen
}

// renderVoiceBlock renders up to blockLen samples from a voice into workBuf.
// Returns true if the voice is done and should be removed.
// Caller must hold m.mu.
func (m *mixer) renderVoiceBlock(vs *voiceState, blockLen int) bool {
	// Ensure channel is set
	if vs.ch == nil {
		vs.ch = channelForInstrument(vs.id)
	}

	// Render samples from voice
	for i := 0; i < blockLen; i++ {
		val, done := vs.v.Sample()
		if done {
			// Process partial block
			if i > 0 {
				vs.ch.ProcessBlock(m.voiceTemp[:i], m.workBuf[:i])
			}
			return true
		}
		m.voiceTemp[i] = val
	}
	vs.ch.ProcessBlock(m.voiceTemp[:blockLen], m.workBuf[:blockLen])
	return false
}
