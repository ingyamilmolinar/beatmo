package ui

import (
	"encoding/json"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// exportActiveInstrument builds a portable bundle for the selected row's
// instrument and downloads it as a *.beatmo-inst.json file. Runs on the UI
// goroutine (menu OnSelect), so the synchronous saveJSON is safe.
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

// instrumentFileName turns an instrument display name into a safe, timestamped
// download filename with the portable-instrument extension. The stamp is what
// keeps a re-export from colliding in the Downloads folder — see
// download_name.go for why a colliding name renders differently per browser.
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

// importInstrumentFromFile opens the file picker and queues the picked bytes for
// application on the UI goroutine (drainInstrumentImport). Mirrors the project
// import guard so a second tap while the picker is open is ignored.
func (dv *DrumView) importInstrumentFromFile() {
	if dv.instImporting {
		return
	}
	dv.instImporting = true
	selectJSONAsyncFn(func(data []byte, name string, err error) {
		dv.instImportCh <- importResult{data: data, name: name, err: err}
	})
}

// drainInstrumentImport applies a picked instrument bundle. Called every frame
// from Update while an import is in flight; a no-op otherwise. When the bundle
// collides with an existing USER instrument it defers the apply to a
// Replace/Keep-both confirm dialog; otherwise it applies immediately.
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

// applyInstrumentBundleWithChoice applies a parsed bundle, resolving every
// collision with the given choice, binding the result to the selected row.
func (dv *DrumView) applyInstrumentBundleWithChoice(b *audio.InstrumentBundle, choice CollisionChoice) {
	resolver := func(ImportCollision) CollisionChoice { return choice }
	if _, err := dv.ApplyInstrumentBundle(b, resolver, dv.selRow, dv.wavImportLocators()); err != nil {
		dv.logger.Errorf("[drumview] instrument import apply failed: %v", err)
	}
}

// wavImportLocators supplies the per-kind WAV-byte fetchers used to resolve a
// referenced WAV on import (bytes, not decoded PCM — the pin is a content hash
// of the file, verified before decode):
//   - localstorage: re-encode the sample store's PCM to the canonical WAV bytes
//     (deterministic; covers same-machine round-trips and cached samples);
//   - file: read the WAV file from a local path (desktop; no-op in the browser);
//   - url: fetch the WAV over HTTP (desktop; async fetch on browser is a follow-up).
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
