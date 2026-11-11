# MusicXML → Circuit Translator + Asturias — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Build a reusable offline MusicXML→circuit translator and use it to replace the `irish-air-guitar` template with a faithful Albéniz "Asturias" opening (E minor, two voices).

**Architecture:** A pure-Go `internal/musicxml` parser produces a neutral `Score`; a `package main` converter in `cmd/gen-showcase` maps `Score`→ the existing `Showcase` struct, so the existing `build()` emits the circuit JSON (reusing layout/edges/colors/drift). Asturias is a committed MusicXML the gen step loads + converts.

**Tech Stack:** Go 1.23 (`.tools/go/bin/go`), `encoding/xml` (no new deps), `-tags test -modfile=go.test.mod`.

## Global Constraints
- **Never `git add`/`git commit`/`git stash`/`git add -N`.** Leave work uncommitted; each task ends at a green checkpoint.
- Bundled Go only: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkg>`. gofmt via `../../.tools/go/bin/gofmt -w`.
- No new go.mod dependencies (pure `encoding/xml`).
- Pitch convention: semitone offset from **A3 = 0** (A3 = MIDI 57). `pc = ((pitch%12)+12)%12`; A=0,B=2,C=3,D=5,E=7,F=8,F#=9,G=10.
- E minor (template key) pitch classes = `{0,2,3,5,7,9,10}` (A,B,C,D,E,F#,G).
- `gen-showcase` `Showcase`/`Hit`/`RowSpec`/`InstSpec` are `package main` (not importable) — the reusable `Score` lives in `internal/musicxml`; the `Score→Showcase` glue stays in `package main`.

---

## File Structure
- `src/go/internal/musicxml/musicxml.go` — types (`Score`,`Part`,`Note`) + `Parse`.
- `src/go/internal/musicxml/musicxml_test.go` — parser unit tests.
- `src/go/cmd/gen-showcase/score_import.go` — `scoreToShowcase` + `asturiasShowcase()` loader.
- `src/go/cmd/gen-showcase/score_import_test.go` — converter + fidelity tests.
- `src/go/cmd/gen-showcase/scores/asturias-opening.musicxml` — committed PD transcription.
- `src/go/cmd/gen-showcase/specs.go` / `main.go` — showcase registry: add `asturias`, remove `irish-air-guitar`.
- `src/go/internal/assets/templates/asturias.json` — generated; `irish-air-guitar.json` — deleted.

---

## Task 1: `internal/musicxml` parser

**Files:** Create `src/go/internal/musicxml/musicxml.go`, `src/go/internal/musicxml/musicxml_test.go`.

**Interfaces — Produces:**
```go
package musicxml
type Note struct { Semitone, OnsetDiv, DurationDiv, Voice int; Rest, Chord bool }
type Part struct { ID string; Notes []Note }
type Score struct { Divisions, KeyFifths int; Mode string; TempoBPM float64; TimeBeats, TimeBeatType int; Parts []Part }
func Parse(data []byte) (Score, error)
```
`Semitone` for non-rests: `midi = (octave+1)*12 + stepSem + alter; Semitone = midi - 57`, where `stepSem` = {C:0,D:2,E:4,F:5,G:7,A:9,B:11}. Timing per `<part>`: maintain a divisions cursor `pos`; a `<note>` without `<chord/>` sets `OnsetDiv=pos` then `pos+=DurationDiv`; a `<note>` with `<chord/>` sets `OnsetDiv` to the previous non-chord note's onset and does NOT advance `pos`; `<rest>` notes advance `pos` like notes and carry `Rest=true`; `<backup><duration>d` → `pos-=d`; `<forward><duration>d` → `pos+=d`. `Divisions`/`KeyFifths`/`Mode`/`TimeBeats`/`TimeBeatType` come from the first `<attributes>`; `TempoBPM` from `<sound tempo="...">` or `<direction><metronome>` (fallback 120).

- [ ] **Step 1: Write failing tests.**
```go
package musicxml
import "testing"
const tinyXML = `<?xml version="1.0"?><score-partwise><part-list><score-part id="P1"/></part-list>
<part id="P1"><measure number="1">
<attributes><divisions>2</divisions><key><fifths>1</fifths><mode>minor</mode></key><time><beats>3</beats><beat-type>4</beat-type></time></attributes>
<sound tempo="120"/>
<note><pitch><step>E</step><octave>3</octave></pitch><duration>2</duration><voice>1</voice></note>
<note><rest/><duration>2</duration><voice>1</voice></note>
<note><pitch><step>B</step><octave>3</octave></pitch><duration>2</duration><voice>1</voice></note>
</measure></part></score-partwise>`
func TestParse_BasicScore(t *testing.T) {
	sc, err := Parse([]byte(tinyXML))
	if err != nil { t.Fatal(err) }
	if sc.Divisions != 2 || sc.KeyFifths != 1 || sc.Mode != "minor" { t.Fatalf("attrs: %+v", sc) }
	if sc.TempoBPM != 120 || sc.TimeBeats != 3 || sc.TimeBeatType != 4 { t.Fatalf("time/tempo: %+v", sc) }
	if len(sc.Parts) != 1 || len(sc.Parts[0].Notes) != 3 { t.Fatalf("notes: %+v", sc.Parts) }
	n := sc.Parts[0].Notes
	if n[0].Semitone != -5 || n[0].OnsetDiv != 0 || n[0].Rest { t.Fatalf("note0 (E3=-5,onset0): %+v", n[0]) }
	if !n[1].Rest || n[1].OnsetDiv != 2 { t.Fatalf("rest1 onset2: %+v", n[1]) }
	if n[2].Semitone != 2 || n[2].OnsetDiv != 4 { t.Fatalf("note2 (B3=+2,onset4): %+v", n[2]) }
}
func TestParse_ChordSharesOnset(t *testing.T) {
	x := `<score-partwise><part id="P1"><measure number="1"><attributes><divisions>1</divisions></attributes>
	<note><pitch><step>E</step><octave>3</octave></pitch><duration>1</duration></note>
	<note><chord/><pitch><step>G</step><octave>3</octave></pitch><duration>1</duration></note>
	<note><pitch><step>A</step><octave>3</octave></pitch><duration>1</duration></note></measure></part></score-partwise>`
	sc, err := Parse([]byte(x)); if err != nil { t.Fatal(err) }
	n := sc.Parts[0].Notes
	if n[0].OnsetDiv != 0 || n[1].OnsetDiv != 0 || !n[1].Chord { t.Fatalf("chord onset: %+v", n) }
	if n[2].OnsetDiv != 1 { t.Fatalf("after-chord onset should be 1: %+v", n[2]) }
	if n[2].Semitone != 0 { t.Fatalf("A3 semitone 0: %+v", n[2]) } // A3=0
}
func TestParse_AlterAndOctave(t *testing.T) {
	x := `<score-partwise><part id="P1"><measure number="1"><attributes><divisions>1</divisions></attributes>
	<note><pitch><step>F</step><alter>1</alter><octave>3</octave></pitch><duration>1</duration></note>
	<note><pitch><step>A</step><octave>4</octave></pitch><duration>1</duration></note></measure></part></score-partwise>`
	sc,_ := Parse([]byte(x)); n := sc.Parts[0].Notes
	if n[0].Semitone != -3 { t.Fatalf("F#3 = -3: %+v", n[0]) }   // F#3=MIDI54-57=-3
	if n[1].Semitone != 12 { t.Fatalf("A4 = +12: %+v", n[1]) }
}
```
- [ ] **Step 2: Run, confirm FAIL** (`-run TestParse` → undefined `Parse`).
- [ ] **Step 3: Implement** `musicxml.go` with `encoding/xml` structs mirroring the elements above and a two-pass approach: unmarshal into raw element structs (capturing element order within a measure requires `xml.Unmarshal` of a `[]struct` with `,any` or per-measure ordered decoding — use a streaming `xml.Decoder` walking tokens per `<part>/<measure>` to preserve note/backup/forward order). Compute onsets per the cursor rules above. Map step→semitone. Set Score attrs from first `<attributes>`/`<sound>`/`<metronome>`.
- [ ] **Step 4: Run, confirm PASS**; run the package.
- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 2: `scoreToShowcase` converter

**Files:** Create `src/go/cmd/gen-showcase/score_import.go`, `src/go/cmd/gen-showcase/score_import_test.go`.

**Interfaces — Consumes** `musicxml.Score` (Task 1). **Produces:**
```go
func scoreToShowcase(sc musicxml.Score, stem, instID, instName string, subdiv int) Showcase
```
Read the existing `Showcase`/`Hit`/`RowSpec`/`InstSpec` definitions in `main.go`/`specs.go` first. Rules: `stepsPerQuarter = subdiv/4`; for each note `step = int(math.Round(float64(n.OnsetDiv)/float64(sc.Divisions)*float64(stepsPerQuarter)))`, `durBeats = float64(n.DurationDiv)/float64(sc.Divisions)` → `Hit.Dur`, `Hit.Pitch = float64(n.Semitone)`. Skip `Rest` notes. Group notes by `(PartIndex, Voice)` → one `RowSpec` per group (stable order), each with `Inst=instID`. One `InstSpec{ID:instID, Name:instName}` per row (rows 1:1 insts). Collision: if two kept notes in one row map to the same `step`, push the later one to the next free step and `log.Printf("score: collision row %d step %d -> %d", ...)`. `Bars = ceil(maxStep+1 over 16)`; set `Showcase.BPM = int(round(sc.TempoBPM))`, `Subdiv = subdiv`.

- [ ] **Step 1: Write failing tests.**
```go
func TestScoreToShowcase_PitchAndStep(t *testing.T) {
	sc := musicxml.Score{Divisions:2, TempoBPM:120, TimeBeats:3, TimeBeatType:4, Parts:[]musicxml.Part{{ID:"P1", Notes:[]musicxml.Note{
		{Semitone:-5, OnsetDiv:0, DurationDiv:2, Voice:1},  // E3 beat1
		{Semitone:2, OnsetDiv:2, DurationDiv:2, Voice:1},   // B3 beat2
		{Semitone:2, OnsetDiv:0, DurationDiv:6, Voice:2},   // pedal in voice2
	}}}}
	sh := scoreToShowcase(sc, "asturias", "guitar-nylon", "Nylon Guitar", 16)
	if sh.BPM != 120 || sh.Subdiv != 16 { t.Fatalf("bpm/subdiv: %+v", sh) }
	if len(sh.Rows) != 2 { t.Fatalf("want 2 rows (voices), got %d", len(sh.Rows)) }
	r1 := sh.Rows[0].Hits
	if r1[0].Pitch != -5 || r1[0].Step != 0 { t.Fatalf("v1 note0 E3 step0: %+v", r1[0]) }
	if r1[1].Step != 4 { t.Fatalf("v1 note1 beat2 = step4: %+v", r1[1]) } // div2/quarter, 4 steps/quarter
	if sh.Rows[0].Inst != "guitar-nylon" { t.Fatalf("inst: %+v", sh.Rows[0]) }
}
func TestScoreToShowcase_SkipsRests(t *testing.T) {
	sc := musicxml.Score{Divisions:1, Parts:[]musicxml.Part{{Notes:[]musicxml.Note{
		{Rest:true, OnsetDiv:0, DurationDiv:1, Voice:1}, {Semitone:7, OnsetDiv:1, DurationDiv:1, Voice:1}}}}}
	sh := scoreToShowcase(sc, "x", "guitar-nylon", "G", 16)
	if len(sh.Rows[0].Hits) != 1 || sh.Rows[0].Hits[0].Pitch != 7 { t.Fatalf("rest not skipped: %+v", sh.Rows) }
}
```
- [ ] **Step 2: Run, confirm FAIL.**
- [ ] **Step 3: Implement** per the rules. Use `math`, `log`, and `sort` for stable voice grouping.
- [ ] **Step 4: Run, confirm PASS.**
- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 3: Asturias MusicXML transcription + loader

**Files:** Create `src/go/cmd/gen-showcase/scores/asturias-opening.musicxml`; add `asturiasShowcase()` to `score_import.go`.

**Interfaces — Produces:** `func asturiasShowcase() Showcase` — reads the embedded/relative MusicXML, `musicxml.Parse`, `scoreToShowcase(sc, "asturias", "guitar-nylon", "Nylon Guitar", 16)`.

- [ ] **Step 1:** Create the MusicXML (key E minor `fifths=1 mode=minor`, `3/4`, tempo ~112, divisions=4). TWO voices in one part: Voice 1 = continuous open-B pedal `B3` (eighth notes); Voice 2 = the bass Phrygian/E-minor descending melody (`E3 D3 C3 B2` … repeated), aligned under the pedal. (The controller will supply the exact note list; encode each `<note>` with `<pitch>`/`<duration>`/`<voice>` and `<backup>` between the two voices per measure. All pitches must be E-minor pitch classes `{A,B,C,D,E,F#,G}`.)
- [ ] **Step 2: Write the loader** `asturiasShowcase()`. Resolve the MusicXML path relative to the gen-showcase working dir (`scores/asturias-opening.musicxml`) OR `//go:embed scores/asturias-opening.musicxml`. Prefer `//go:embed` (robust to cwd).
- [ ] **Step 3: Smoke test** (append to `score_import_test.go`):
```go
func TestAsturiasShowcase_Loads(t *testing.T) {
	sh := asturiasShowcase()
	if sh.Stem != "asturias" || len(sh.Rows) < 2 { t.Fatalf("asturias: %+v", sh.Stem) }
	total := 0
	for _, r := range sh.Rows { total += len(r.Hits) }
	if total < 16 { t.Fatalf("too few notes: %d", total) }
}
```
- [ ] **Step 4: Run, confirm PASS.**
- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 4: Register asturias, remove irish-air, regenerate

**Files:** Modify `src/go/cmd/gen-showcase/specs.go`/`main.go` (showcase registry); delete `src/go/internal/assets/templates/irish-air-guitar.json`; generate `asturias.json`.

- [ ] **Step 1:** Read how showcases are registered (the slice/list `main.go` iterates). Remove the `irish-air-guitar` entry. Add `asturias` by appending `asturiasShowcase()` to the list (or a registry hook that calls it).
- [ ] **Step 2:** Run the generator: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase`. Confirm `internal/assets/templates/asturias.json` is created and `irish-air-guitar.json` removed (delete the file if the generator doesn't prune: `rm src/go/internal/assets/templates/irish-air-guitar.json`).
- [ ] **Step 3:** Verify the JSON: `python3 -c "import json;d=json.load(open('src/go/internal/assets/templates/asturias.json'));print(d['bpm'],d['subdiv'],[i['id'] for i in d['instruments']],len(d['nodes']))"` → bpm ~112, subdiv 16, two `guitar-nylon` instruments, >16 nodes.
- [ ] **Step 4:** Check the template embed list — if templates are embedded via a manifest (`templates.go`), ensure `asturias.json` is included and `irish-air-guitar.json` removed; run `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/...` (templates_test).
- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 5: Fidelity validation + drift

**Files:** `src/go/cmd/gen-showcase/score_import_test.go` (append); run drift.

- [ ] **Step 1: Write fidelity tests.**
```go
func TestAsturias_DiatonicToEMinor(t *testing.T) {
	eMinor := map[int]bool{0:true,2:true,3:true,5:true,7:true,9:true,10:true} // A,B,C,D,E,F#,G
	for _, r := range asturiasShowcase().Rows {
		for _, h := range r.Hits {
			pc := ((int(h.Pitch)%12)+12)%12
			if !eMinor[pc] { t.Errorf("non-E-minor pitch class %d (pitch %v)", pc, h.Pitch) }
		}
	}
}
func TestAsturias_HasPedalVoice(t *testing.T) {
	// One voice should be a repeated single pitch (the B3 pedal).
	for _, r := range asturiasShowcase().Rows {
		if len(r.Hits) < 4 { continue }
		same := true
		for _, h := range r.Hits { if h.Pitch != r.Hits[0].Pitch { same = false; break } }
		if same && r.Hits[0].Pitch == 2 { return } // B3 pedal found
	}
	t.Fatal("no B3 (+2) pedal voice found")
}
```
- [ ] **Step 2: Run, confirm PASS** (fix the MusicXML in Task 3 if a pitch is out of key — do NOT loosen the test).
- [ ] **Step 3: Run drift + the full gen-showcase suite:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/gen-showcase/ ./internal/musicxml/`. Drift must pass (asturias.json matches `build()`); the removed irish-air must not be referenced by any remaining test.
- [ ] **Step 4 (optional key-detect):** Render and confirm tonality if a quick path exists — otherwise the diatonic test suffices.
- [ ] **Step 5: gofmt + checkpoint.**

---

## Final verification
- [ ] `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/musicxml/ ./cmd/gen-showcase/ ./internal/assets/...` → all PASS.
- [ ] `asturias.json` present, `irish-air-guitar.json` gone, no dangling references. gofmt-clean. Uncommitted.

## Self-review
- Spec components 1–4 → Tasks 1–4; fidelity validation → Tasks 1(parse),5; drift → Task 5. Error handling (collisions/rests/onset cursor) → Tasks 1,2.
- Types consistent: `Score`/`Note`/`Part` (Task 1) consumed verbatim by Task 2; `scoreToShowcase` signature identical in Tasks 2/3; `asturiasShowcase()` in Tasks 3/4/5.
- The exact Asturias note list is supplied by the controller at Task 3 (content-critical), encoded as E-minor pitch classes so Task 5's diatonic test passes by construction.
