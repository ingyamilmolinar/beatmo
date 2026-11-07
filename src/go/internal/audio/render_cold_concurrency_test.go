//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Together they use every lazily built oscillator table.
var coldRaceIDs = []instrument.ID{roster.LeadHoover, roster.BassWobble, roster.PwmPad, roster.LeadChip, roster.EnsembleLead, roster.Violin, roster.FmBass, roster.FmBell}

func coldRenderHash(t *testing.T, id string) string {
	t.Helper()
	buf, err := RenderInstrumentWithFields(instrument.ID(id), 22050, 22050, nil)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// The prewarm pool renders on several workers from the moment a circuit loads, so a session's first renders ARE
// concurrent, and a table two cold renders built at once froze corrupted for the whole process. Each child starts
// COLD and releases eight renders at once; every one must equal the serial reference.
func TestColdConcurrentRendersMatchSerial(t *testing.T) {
	if os.Getenv("BEATMO_COLD_RENDER_CHILD") == "1" {
		t.Skip("child process")
	}
	want := map[string]string{}
	for _, id := range coldRaceIDs {
		want[string(id)] = coldRenderHash(t, string(id))
	}
	const launches = 10
	bad := 0
	for run := 0; run < launches; run++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestColdConcurrentRenderChild$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "BEATMO_COLD_RENDER_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("child %d: %v\n%s", run, err, out)
		}
		seen := 0
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) != 3 || f[0] != "COLDHASH" {
				continue
			}
			seen++
			if f[2] != want[f[1]] {
				bad++
				t.Errorf("launch %d: a cold concurrent render of %s differs from the serial render (%s vs %s)", run, f[1], f[2], want[f[1]])
			}
		}
		if seen != len(coldRaceIDs) {
			t.Fatalf("child %d reported %d hashes, want %d:\n%s", run, seen, len(coldRaceIDs), out)
		}
	}
	if bad > 0 {
		t.Logf("%d of %d cold concurrent renders were corrupted", bad, launches*len(coldRaceIDs))
	}
}

// No render has happened in this process, so all eight race to build the tables.
func TestColdConcurrentRenderChild(t *testing.T) {
	if os.Getenv("BEATMO_COLD_RENDER_CHILD") != "1" {
		t.Skip("run by TestColdConcurrentRendersMatchSerial")
	}
	start := make(chan struct{})
	got := make([]string, len(coldRaceIDs))
	var wg sync.WaitGroup
	for i, id := range coldRaceIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			got[i] = coldRenderHash(t, id)
		}(i, string(id))
	}
	close(start)
	wg.Wait()
	for i, id := range coldRaceIDs {
		fmt.Printf("COLDHASH %s %s\n", id, got[i])
	}
}
