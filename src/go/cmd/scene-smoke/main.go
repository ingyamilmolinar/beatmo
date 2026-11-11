//go:build !js

// Command scene-smoke runs every catalog scene in the real desktop binary, in
// the desktop and the mobile pass, and fails on a crash, a missing capture or
// a blank one. The -tags test suite draws scenes through the Ebiten stub; only
// the real binary shows that Ebiten renders them.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A capture with fewer distinct colours than this drew nothing but a fill.
const minDistinctColors = 16

type job struct {
	pass, name, out string
	args            []string
}

type result struct {
	job
	err error
}

func main() {
	bin := flag.String("bin", "", "the desktop beatmo binary (real Ebiten build)")
	out := flag.String("out", "build/scene-smoke", "directory for the captures")
	jobs := flag.Int("jobs", 4, "scenes captured at once")
	timeout := flag.Duration("timeout", 90*time.Second, "per-scene time limit")
	only := flag.String("scenes", "", "comma-separated scene names (default: all)")
	flag.Parse()
	if *bin == "" {
		fail("-bin is required")
	}
	work, err := plan(*bin, *out, *only)
	if err != nil {
		fail(err.Error())
	}
	if len(work) == 0 {
		fail("no scenes to capture")
	}
	results := runAll(*bin, work, *jobs, *timeout)
	failed := report(results)
	if failed > 0 {
		os.Exit(1)
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "scene-smoke:", msg)
	os.Exit(2)
}

func listScenes(bin, flagName string) ([]string, error) {
	cmd := exec.Command(bin, flagName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %v\n%s", bin, flagName, err, stderr.String())
	}
	return strings.Fields(string(b)), nil
}

func plan(bin, out, only string) ([]job, error) {
	desktop, err := listScenes(bin, "-list-scenes")
	if err != nil {
		return nil, err
	}
	mobile, err := listScenes(bin, "-list-mobile-scenes")
	if err != nil {
		return nil, err
	}
	keep := func(string) bool { return true }
	if only != "" {
		want := map[string]bool{}
		for _, n := range strings.Split(only, ",") {
			want[strings.TrimSpace(n)] = true
		}
		keep = func(n string) bool { return want[n] }
	}
	var work []job
	for _, n := range desktop {
		if strings.HasPrefix(n, "mobile_") || !keep(n) {
			continue
		}
		work = append(work, job{pass: "desktop", name: n, out: filepath.Join(out, "desktop", n+".png")})
	}
	for _, n := range mobile {
		if !keep(n) {
			continue
		}
		work = append(work, job{pass: "mobile", name: n, out: filepath.Join(out, "mobile", n+".png"),
			args: []string{"-mobile", "-scene-pass", "mobile", "-win-w", "390", "-win-h", "844"}})
	}
	for _, d := range []string{"desktop", "mobile"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0o755); err != nil {
			return nil, err
		}
	}
	return work, nil
}

func runAll(bin string, work []job, parallel int, timeout time.Duration) []result {
	results := make([]result, len(work))
	sem := make(chan struct{}, max(1, parallel))
	var wg sync.WaitGroup
	for i, j := range work {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = result{job: j, err: capture(bin, j, timeout)}
			mark := "ok  "
			if results[i].err != nil {
				mark = "FAIL"
			}
			fmt.Printf("%s %-7s %s\n", mark, j.pass, j.name)
		}()
	}
	wg.Wait()
	return results
}

func capture(bin string, j job, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// A scene that persists user state (the Theme editor) must not touch the
	// runner's profile or another scene's.
	cfg, err := os.MkdirTemp("", "scene-smoke-cfg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cfg)
	_ = os.Remove(j.out)
	args := append([]string{"-log", "ERROR"}, j.args...)
	args = append(args, "-scene", j.name, "-screenshot", j.out)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+cfg)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("timed out after %v", timeout)
		}
		// A Go crash report leads with the signal and the stacks; the register
		// dump that ends it says little. The whole report stays beside the PNG.
		logPath := strings.TrimSuffix(j.out, ".png") + ".log"
		_ = os.WriteFile(logPath, output.Bytes(), 0o644)
		return fmt.Errorf("binary: %v (full output: %s)\n%s", err, logPath, head(output.String(), 40))
	}
	return checkCapture(j.out)
}

// checkCapture fails when the PNG is missing, undecodable or blank.
func checkCapture(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("no capture: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("capture does not decode: %v", err)
	}
	if n := distinctColors(img, minDistinctColors); n < minDistinctColors {
		b := img.Bounds()
		return fmt.Errorf("blank capture: %d distinct colours in %dx%d", n, b.Dx(), b.Dy())
	}
	return nil
}

// distinctColors counts the image's colours, stopping once it reaches limit.
func distinctColors(img image.Image, limit int) int {
	seen := map[[4]uint32]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			seen[[4]uint32{r, g, bl, a}] = true
			if len(seen) >= limit {
				return len(seen)
			}
		}
	}
	return len(seen)
}

func head(s string, lines int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) > lines {
		all = all[:lines]
	}
	return strings.Join(all, "\n")
}

func report(results []result) int {
	var bad []result
	count := map[string]int{}
	for _, r := range results {
		count[r.pass]++
		if r.err != nil {
			bad = append(bad, r)
		}
	}
	sort.Slice(bad, func(i, k int) bool { return bad[i].pass+bad[i].name < bad[k].pass+bad[k].name })
	fmt.Printf("\nscene-smoke: %d desktop + %d mobile captures, %d failed\n", count["desktop"], count["mobile"], len(bad))
	for _, r := range bad {
		fmt.Printf("\n--- %s %s\n%v\n", r.pass, r.name, r.err)
	}
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "## Scene smoke\n\n%d desktop + %d mobile scenes captured in the real binary, %d failed.\n",
			count["desktop"], count["mobile"], len(bad))
		for _, r := range bad {
			first, _, _ := strings.Cut(r.err.Error(), "\n")
			fmt.Fprintf(&b, "- `%s` %s: %s\n", r.name, r.pass, first)
		}
		if err := appendFile(path, b.String()); err != nil {
			fmt.Fprintln(os.Stderr, "scene-smoke: step summary:", err)
		}
	}
	return len(bad)
}

func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(s)
	return errors.Join(werr, f.Close())
}
