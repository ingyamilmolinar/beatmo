//go:build !js && !test

package audio

import (
    "os"
    "path/filepath"
    "strings"
    "runtime"
    "sync"

    "github.com/ingyamilmolinar/tunkul/internal/assets"
)

// AutoLoadEmbeddedWAVs registers all embedded WAVs as instruments.
// On desktop, we write each asset to a temporary file and decode via miniaudio.
func AutoLoadEmbeddedWAVs() {
    wavs, err := assets.ListEmbeddedWAVs()
    if err != nil {
        return
    }
    // Decode concurrently to speed up startup without blocking the main game.
    // Limit concurrency to a modest worker count to avoid saturating I/O/CPU.
    workers := runtime.NumCPU()
    if workers > 4 {
        workers = 4
    }
    if workers < 1 {
        workers = 1
    }
    type job struct{ id string; data []byte }
    jobs := make(chan job)
    var wg sync.WaitGroup
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := range jobs {
                base := strings.ReplaceAll(j.id, "sample-", "")
                tmp, err := os.CreateTemp("", "tunkul-"+base+"-*.wav")
                if err != nil {
                    continue
                }
                _, _ = tmp.Write(j.data)
                _ = tmp.Close()
                _ = RegisterWAV(j.id, filepath.ToSlash(tmp.Name()))
            }
        }()
    }
    for _, w := range wavs {
        jobs <- job{id: w.ID, data: w.Data}
    }
    close(jobs)
    wg.Wait()
}
