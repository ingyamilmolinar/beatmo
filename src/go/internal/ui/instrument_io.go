package ui

import (
	"encoding/json"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// On the UI goroutine, so the synchronous saveJSON is safe.
func (dv *DrumView) exportActiveInstrument() {
	b, err := dv.BuildInstrumentBundle(dv.selRow)
	if err != nil {
		dv.logger.Errorf("[drumview] export instrument failed: %v", err)
		return
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		dv.logger.Errorf("[drumview] export instrument marshal failed: %v", err)
		return
	}
	name := "instrument"
	if dv.selRow >= 0 && dv.selRow < len(dv.Rows) {
		name = dv.Rows[dv.selRow].Name
	}
	if err := saveJSON(instrumentFileName(name), raw); err != nil {
		dv.logger.Errorf("[drumview] export instrument save failed: %v", err)
	}
}

// Stamped so a re-export never collides (browsers spell a collision differently).
func instrumentFileName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "instrument"
	}
	return stampedFileName(base, ".beatmo-inst.json")
}

// A second tap while the picker is open is ignored, as on project import.
func (dv *DrumView) importInstrumentFromFile() {
	if dv.instImporting {
		return
	}
	dv.instImporting = true
	selectJSONAsyncFn(func(data []byte, name string, err error) {
		dv.instImportCh <- importResult{data: data, name: name, err: err}
	})
}

// A collision with a USER instrument defers to the Replace/Keep-both dialog.
func (dv *DrumView) drainInstrumentImport() {
	if !dv.instImporting {
		return
	}
	select {
	case res := <-dv.instImportCh:
		dv.instImporting = false
		if res.err != nil {
			dv.logger.Errorf("[drumview] instrument import failed: %v", res.err)
			return
		}
		if len(res.data) == 0 {
			return // canceled
		}
		var b audio.InstrumentBundle
		if err := json.Unmarshal(res.data, &b); err != nil {
			dv.logger.Errorf("[drumview] instrument import parse failed: %v", err)
			return
		}
		if cols := bundleUserCollisions(&b); len(cols) > 0 {
			// Defer to the user: Replace (overwrite) or Keep both (re-namespace).
			dv.openImportCollisionDialog(cols, func(choice CollisionChoice) {
				dv.applyInstrumentBundleWithChoice(&b, choice)
			})
			return
		}
		dv.applyInstrumentBundleWithChoice(&b, CollisionKeepBoth)
	default:
	}
}

func (dv *DrumView) applyInstrumentBundleWithChoice(b *audio.InstrumentBundle, choice CollisionChoice) {
	resolver := func(ImportCollision) CollisionChoice { return choice }
	if _, err := dv.ApplyInstrumentBundle(b, resolver, dv.selRow, dv.wavImportLocators()); err != nil {
		dv.logger.Errorf("[drumview] instrument import apply failed: %v", err)
	}
}

// Bytes, not PCM: the pin hashes the file and is checked before decode. localstorage re-encodes the stored PCM;
// file and url are desktop only.
func (dv *DrumView) wavImportLocators() map[audio.WavSourceKind]audio.WavLocator {
	return map[audio.WavSourceKind]audio.WavLocator{
		audio.WavSourceLocalStorage: func(ref string) ([]byte, bool) {
			id, err := instrument.ParseID(ref)
			if err != nil {
				return nil, false
			}
			if rec, ok := audio.UserSamplePCM(id); ok {
				if data, err := wavgen.EncodeWAV(rec.PCM, rec.SampleRate); err == nil {
					return data, true
				}
			}
			return nil, false
		},
		audio.WavSourceFile: func(ref string) ([]byte, bool) { return readWavFileBytes(ref) },
		audio.WavSourceURL:  func(ref string) ([]byte, bool) { return fetchWavURLBytes(ref) },
	}
}
