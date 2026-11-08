package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRuntime stands in for podman: it records each container's mounts and
// runs `exec` natively, rewriting container paths to the host paths mounted
// there, so the whole job lifecycle runs without a container engine.
const fakeRuntime = `#!/usr/bin/env bash
set -e
state="$FAKE_RT_STATE"
cmd="$1"; shift
case "$cmd" in
  info) echo "[cpu memory pids]";;
  image|pull|build) exit 0;;
  run)
    args=("$@"); name=""
    for ((i=0; i<${#args[@]}; i++)); do [ "${args[i]}" = --name ] && name="${args[i+1]}"; done
    : > "$state/$name.mounts"
    while [ $# -gt 0 ]; do case "$1" in
      -v) echo "$2" >> "$state/$name.mounts"; shift 2;;
      --entrypoint) echo "$3" > "$state/$name.image"; shift 3;;
      *) shift;; esac; done;;
  exec)
    wd=""; envs=()
    while [ $# -gt 0 ]; do case "$1" in
      -w) wd="$2"; shift 2;;
      -e) envs+=("$2"); shift 2;;
      *) break;; esac; done
    name="$1"; shift
    map() { local s="$1"; while IFS=: read -r h c; do s="${s//$c/$h}"; done < "$state/$name.mounts"; printf '%s' "$s"; }
    mapped=(); for e in "${envs[@]}"; do mapped+=("$(map "$e")"); done
    cd "$(map "$wd")"
    shift 3
    exec env -i PATH="$PATH" FAKE_IMAGE="$(cat "$state/$name.image")" "${mapped[@]}" bash -e -c "$(map "$1")";;
  rm) rm -f "$state/$2.mounts";;
esac
`

const fixtureWorkflow = `env:
  TOP: top
jobs:
  images:
    runs-on: ubuntu-latest
    steps:
      - run: make image-publish
  produce:
    name: Produce ${{ matrix.shard }}/2
    needs: [images]
    container: ${{ needs.images.outputs.build }}
    strategy:
      matrix:
        shard: [1, 2]
    steps:
      - uses: actions/checkout@v4
      - run: mkdir -p out && echo "shard ${{ matrix.shard }} $TOP $FAKE_IMAGE $(cat tracked.txt)" > out/s${{ matrix.shard }}.txt
      - uses: actions/upload-artifact@v4
        with:
          name: part-${{ matrix.shard }}
          path: out
  matrix:
    runs-on: ubuntu-latest
    outputs:
      m: ${{ steps.set.outputs.m }}
    steps:
      - id: set
        run: echo 'm={"include":[{"index":1,"word":"a"},{"index":2,"word":"b"}]}' >> "$GITHUB_OUTPUT"
  consume:
    name: "Consume ${{ matrix.index }}"
    needs: [images, produce, matrix]
    container:
      image: ${{ needs.images.outputs.browser }}
      options: --init
    env:
      HOME: /root
    strategy:
      matrix: ${{ fromJson(needs.matrix.outputs.m) }}
    steps:
      - uses: actions/download-artifact@v4
        with:
          pattern: part-*
          path: got
          merge-multiple: true
      - run: cat got/s1.txt got/s2.txt > merged-${{ matrix.word }}.txt && echo "$HOME $CI $GITHUB_ACTIONS" >> merged-${{ matrix.word }}.txt && test -z "$LEAKED"
  broken:
    needs: [images]
    container: ${{ needs.images.outputs.build }}
    steps:
      - run: exit 3
      - run: touch never
      - if: always()
        run: touch always
  verdict:
    if: always()
    needs: [produce, consume, broken]
    runs-on: ubuntu-latest
    steps:
      - env:
          NEEDS: ${{ toJSON(needs) }}
        run: echo "$NEEDS" > "$CHECK_DIR/needs.json"
`

func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	write := func(rel, body string, mode os.FileMode) {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(".github/workflows/ci.yml", fixtureWorkflow, 0o644)
	write("scripts/ci/image-tag.sh", "#!/bin/sh\necho ref-$1\n", 0o755)
	write("tracked.txt", "committed\n", 0o644)
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func TestRunsTheWorkflowLikeGitHub(t *testing.T) {
	repo := fixtureRepo(t)
	// An uncommitted edit to a tracked file is part of the run; the real
	// index and working tree stay as they were.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, check := t.TempDir(), t.TempDir()
	rt := filepath.Join(t.TempDir(), "fake-rt")
	if err := os.WriteFile(rt, []byte(fakeRuntime), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_RT_STATE", t.TempDir())
	t.Setenv("CHECK_DIR", check)
	t.Setenv("LEAKED", "the host environment must not reach a job container")

	code := realMain([]string{"-C", repo, "-state", state, "-runtime", rt, "-j", "2", "-keep"})
	if code != 1 {
		t.Fatalf("exit %d, want 1 (the broken job fails, every other job passes)", code)
	}

	merged, err := os.ReadFile(filepath.Join(state, "w", "consume-1", "merged-a.txt"))
	if err != nil {
		t.Fatalf("the computed matrix did not run consume-1: %v", err)
	}
	want := "shard 1 top ref-build edited\nshard 2 top ref-build edited\n/root true true\n"
	if string(merged) != want {
		t.Errorf("consume-1 saw\n%s\nwant\n%s", merged, want)
	}
	if _, err := os.Stat(filepath.Join(state, "w", "consume-2", "merged-b.txt")); err != nil {
		t.Errorf("consume-2 did not run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "w", "broken", "never")); err == nil {
		t.Error("a step after a failed one ran")
	}
	if _, err := os.Stat(filepath.Join(state, "w", "broken", "always")); err != nil {
		t.Error("an `if: always()` step did not run after a failure")
	}

	var needs map[string]result
	b, err := os.ReadFile(filepath.Join(check, "needs.json"))
	if err != nil {
		t.Fatalf("verdict (if: always()) did not run after a failed need: %v", err)
	}
	if err := json.Unmarshal(b, &needs); err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string]string{"produce": "success", "consume": "success", "broken": "failure"} {
		if needs[job].Result != want {
			t.Errorf("toJSON(needs).%s.result = %q, want %q", job, needs[job].Result, want)
		}
	}

	st, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(st)) != "M tracked.txt" {
		t.Errorf("the developer's checkout changed: git status %q", st)
	}
	if refs, _ := exec.Command("git", "-C", repo, "for-each-ref").Output(); strings.Contains(string(refs), "beatmo-ci") {
		t.Errorf("a scratch ref was left in the developer's repo:\n%s", refs)
	}
}

// Every expression, action and matrix in the real ci.yml is one this runner
// knows, so a workflow edit fails here rather than on a developer's machine.
func TestTheRealWorkflowIsSupported(t *testing.T) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	wf, err := loadWorkflow(filepath.Join(strings.TrimSpace(string(out)), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	order, err := wf.order(nil)
	if err != nil {
		t.Fatal(err)
	}
	needs := map[string]*result{}
	for _, j := range order {
		needs[j.ID] = &result{Result: "success", Outputs: map[string]string{
			"build": "b", "browser": "c", "matrix": `{"include":[{"index":1,"tests":"x.browser.test.js"}]}`}}
	}
	for _, j := range order {
		c := newCtx(j.ID, nil, needs, map[string]string{})
		if _, err := c.cond(j.If); err != nil {
			t.Errorf("job %s: %v", j.ID, err)
		}
		insts, err := j.instances(c)
		if err != nil {
			t.Errorf("job %s: %v", j.ID, err)
			continue
		}
		if j.ID == "images" {
			continue // ci-local resolves the images itself
		}
		for _, in := range insts {
			sc := newCtx(j.ID, in.matrix, needs, map[string]string{})
			image, _ := j.container()
			if _, err := sc.expand(image); err != nil {
				t.Errorf("job %s container: %v", j.ID, err)
			}
			for k, v := range j.Env {
				if _, err := sc.expand(v); err != nil {
					t.Errorf("job %s env %s: %v", j.ID, k, err)
				}
			}
			for _, st := range j.Steps {
				if _, err := sc.cond(st.If); err != nil {
					t.Errorf("job %s step: %v", j.ID, err)
				}
				uses, _, _ := strings.Cut(st.Uses, "@")
				switch uses {
				case "", "actions/checkout", "actions/cache", "actions/upload-artifact", "actions/download-artifact":
				default:
					t.Errorf("job %s uses %s, which ci-local cannot run", j.ID, st.Uses)
				}
				for _, s := range append([]string{st.Run}, mapValues(st.With, st.Env)...) {
					if _, err := sc.expand(s); err != nil {
						t.Errorf("job %s step %q: %v", j.ID, st.Run+st.Uses, err)
					}
				}
			}
		}
	}
}

func mapValues(ms ...map[string]string) []string {
	var out []string
	for _, m := range ms {
		for k, v := range m {
			if k == "key" || k == "restore-keys" {
				continue // cache keys: ci-local's caches are plain volumes
			}
			out = append(out, v)
		}
	}
	return out
}
