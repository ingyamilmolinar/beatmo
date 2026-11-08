package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type runner struct {
	root    string // the developer's checkout
	state   string // scratch: worktrees, caches, logs, artifacts
	base    string // bare repo holding the one commit every job checks out
	rt      string // podman or docker
	rootful bool   // docker as root: hand files back to the user afterwards
	limits  []string
	vars    map[string]string
	keep    bool
	dry     bool
	jobs    int
	wf      *workflow
	stream  bool

	mu         sync.Mutex
	results    map[string]*result
	done       []doneInst
	containers map[string]bool
}

type doneInst struct {
	key, name, result string
	dur               time.Duration
	log               string
}

func (r *runner) git(env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// snapshot commits HEAD plus the tracked working-tree changes (never the real
// index, stash or working tree) and fetches that one commit into the bare
// scratch repo: a shallow checkout, as actions/checkout makes.
func (r *runner) snapshot(committedOnly bool) (string, error) {
	commit, err := r.git(nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if !committedOnly {
		idx := filepath.Join(r.state, "snapshot.index")
		_ = os.Remove(idx)
		env := []string{"GIT_INDEX_FILE=" + idx}
		if _, err := r.git(env, "read-tree", "HEAD"); err != nil {
			return "", err
		}
		if _, err := r.git(env, "add", "-u"); err != nil {
			return "", err
		}
		added, err := r.git(nil, "diff", "--cached", "--name-only", "--diff-filter=A")
		if err != nil {
			return "", err
		}
		if added != "" {
			if _, err := r.git(env, append([]string{"add", "--"}, strings.Split(added, "\n")...)...); err != nil {
				return "", err
			}
		}
		tree, err := r.git(env, "write-tree")
		if err != nil {
			return "", err
		}
		headTree, _ := r.git(nil, "rev-parse", "HEAD^{tree}")
		if tree != headTree {
			if commit, err = r.git(nil, "commit-tree", tree, "-p", "HEAD", "-m", "ci-local: working tree"); err != nil {
				return "", err
			}
			fmt.Printf("ci-local: running HEAD + tracked working-tree changes (%s); untracked files are not part of it, as in CI\n", commit[:12])
		}
		_ = os.Remove(idx)
	}
	if _, err := os.Stat(r.base); err != nil {
		if _, err := r.git(nil, "init", "-q", "--bare", r.base); err != nil {
			return "", err
		}
	}
	const ref = "refs/beatmo-ci/snapshot"
	if _, err := r.git(nil, "update-ref", ref, commit); err != nil {
		return "", err
	}
	defer func() { _, _ = r.git(nil, "update-ref", "-d", ref) }()
	if _, err := r.git(nil, "-C", r.base, "fetch", "-q", "--depth", "1", "--no-tags", "file://"+r.root, "+"+ref+":refs/heads/snapshot"); err != nil {
		return "", err
	}
	return commit, nil
}

func (r *runner) worktree(key string) (string, error) {
	dir := filepath.Join(r.state, "w", key)
	_, _ = r.git(nil, "-C", r.base, "worktree", "remove", "--force", dir)
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clearing %s: %w", dir, err)
	}
	_, _ = r.git(nil, "-C", r.base, "worktree", "prune")
	if _, err := r.git(nil, "-C", r.base, "worktree", "add", "-q", "--detach", dir, "snapshot"); err != nil {
		return "", err
	}
	return dir, nil
}

func (r *runner) imageRef(kind string) (string, error) {
	out, err := exec.Command(filepath.Join(r.root, "scripts/ci/image-tag.sh"), kind).Output()
	return strings.TrimSpace(string(out)), err
}

// images stands in for the `images` job: the same content-hash tags, pulled
// from GHCR or, when ci/ changed locally and nothing is published, built here
// from the same Dockerfiles and pins.
func (r *runner) images() (map[string]string, error) {
	refs := map[string]string{}
	for _, kind := range []string{"build", "browser"} {
		ref, err := r.imageRef(kind)
		if err != nil {
			return nil, err
		}
		refs[kind] = ref
		if r.dry || exec.Command(r.rt, "image", "inspect", ref).Run() == nil {
			continue
		}
		fmt.Printf("ci-local: pulling %s\n", ref)
		pull := exec.Command(r.rt, "pull", ref)
		pull.Stdout, pull.Stderr = os.Stdout, os.Stderr
		if pull.Run() == nil {
			continue
		}
		fmt.Printf("ci-local: %s is not published (ci/ changed locally): building it from ci/images/%s.Dockerfile\n", ref, kind)
		args := []string{"build", "-f", filepath.Join(r.root, "ci/images", kind+".Dockerfile"), "-t", ref}
		pins, err := os.ReadFile(filepath.Join(r.root, "ci/versions.env"))
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(pins), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				args = append(args, "--build-arg", line)
			}
		}
		if kind == "browser" {
			args = append(args, "--build-arg", "BUILD_IMAGE="+refs["build"])
		}
		build := exec.Command(r.rt, append(args, filepath.Join(r.root, "ci/images"))...)
		build.Stdout, build.Stderr = os.Stdout, os.Stderr
		if err := build.Run(); err != nil {
			return nil, fmt.Errorf("building %s: %w", ref, err)
		}
	}
	return refs, nil
}

// run executes the selected jobs in dependency order, up to r.jobs instances
// at a time, and returns whether every instance that ran succeeded.
func (r *runner) run(ctx context.Context, order []*job, filter map[string]string) bool {
	r.results = map[string]*result{}
	r.containers = map[string]bool{}
	sem := make(chan struct{}, r.jobs)
	finished := map[string]chan struct{}{}
	for _, j := range order {
		finished[j.ID] = make(chan struct{})
	}
	var wg sync.WaitGroup
	for _, j := range order {
		wg.Add(1)
		go func(j *job) {
			defer wg.Done()
			defer close(finished[j.ID])
			for _, n := range j.needs() {
				<-finished[n]
			}
			r.runJob(ctx, j, filter[j.ID], sem)
		}(j)
	}
	wg.Wait()
	ok := true
	for _, d := range r.done {
		ok = ok && (d.result == "success" || d.result == "skipped")
	}
	return ok
}

func (r *runner) snapshotNeeds(j *job) map[string]*result {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]*result{}
	for _, n := range j.needs() {
		if res := r.results[n]; res != nil {
			out[n] = res
		} else {
			out[n] = &result{Result: "skipped", Outputs: map[string]string{}}
		}
	}
	return out
}

func (r *runner) record(j *job, res *result, d ...doneInst) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[j.ID] = res
	r.done = append(r.done, d...)
}

func (r *runner) runJob(ctx context.Context, j *job, filter string, sem chan struct{}) {
	needs := r.snapshotNeeds(j)
	jc := newCtx(j.ID, nil, needs, r.vars)
	anyFailed := false
	for _, n := range needs {
		anyFailed = anyFailed || n.Result != "success"
	}
	jc.failed = anyFailed
	if j.ID == "images" {
		refs, err := r.images()
		if err != nil {
			fmt.Printf("✖ CI images: %v\n", err)
			r.record(j, &result{Result: "failure", Outputs: map[string]string{}}, doneInst{key: j.ID, name: "CI images", result: "failure"})
			return
		}
		r.record(j, &result{Result: "success", Outputs: refs}, doneInst{key: j.ID, name: "CI images", result: "success"})
		return
	}
	run, err := jc.cond(j.If)
	if err != nil {
		fmt.Printf("✖ %s: %v\n", j.ID, err)
		r.record(j, &result{Result: "failure", Outputs: map[string]string{}}, doneInst{key: j.ID, name: j.ID, result: "failure"})
		return
	}
	if !run {
		r.record(j, &result{Result: "skipped", Outputs: map[string]string{}}, doneInst{key: j.ID, name: j.ID, result: "skipped"})
		return
	}
	insts, err := j.instances(jc)
	if err != nil {
		fmt.Printf("✖ %s: %v\n", j.ID, err)
		r.record(j, &result{Result: "failure", Outputs: map[string]string{}}, doneInst{key: j.ID, name: j.ID, result: "failure"})
		return
	}
	var sel []instance
	for _, in := range insts {
		if in.matches(filter) {
			sel = append(sel, in)
		}
	}
	if len(sel) == 0 {
		fmt.Printf("✖ %s: no matrix instance matches %q\n", j.ID, filter)
		r.record(j, &result{Result: "failure", Outputs: map[string]string{}}, doneInst{key: j.ID, name: j.ID, result: "failure"})
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	agg := &result{Result: "success", Outputs: map[string]string{}}
	var dones []doneInst
	for _, in := range sel {
		wg.Add(1)
		go func(in instance) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, outs := r.runInstance(ctx, in, needs)
			mu.Lock()
			defer mu.Unlock()
			dones = append(dones, d)
			if d.result != "success" {
				agg.Result = "failure"
			}
			for k, v := range outs {
				agg.Outputs[k] = v
			}
		}(in)
	}
	wg.Wait()
	sort.Slice(dones, func(a, b int) bool { return dones[a].key < dones[b].key })
	r.record(j, agg, dones...)
}

func newCtx(jobID string, matrix map[string]string, needs map[string]*result, vars map[string]string) *ctx {
	return &ctx{jobID: jobID, matrix: matrix, needs: needs, steps: map[string]map[string]string{}, vars: vars}
}

func (r *runner) runInstance(ctx context.Context, in instance, needs map[string]*result) (doneInst, map[string]string) {
	start := time.Now()
	d := doneInst{key: in.key, name: in.name, result: "failure"}
	logPath := filepath.Join(r.state, "logs", in.key+".log")
	d.log = logPath
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logf, err := os.Create(logPath)
	if err != nil {
		fmt.Printf("✖ %s: %v\n", in.name, err)
		return d, nil
	}
	defer logf.Close()
	var out io.Writer = logf
	if r.stream {
		out = io.MultiWriter(logf, prefixed(os.Stdout, in.key))
	}
	fmt.Printf("▶ %s\n", in.name)
	sc := newCtx(in.job.ID, in.matrix, needs, r.vars)
	res, outputs := r.execInstance(ctx, in, sc, out)
	d.result, d.dur = res, time.Since(start)
	mark := "✔"
	if res != "success" {
		mark = "✖"
	}
	fmt.Printf("%s %s (%s) — log %s\n", mark, in.name, d.dur.Round(time.Second), logPath)
	if res != "success" && !r.stream {
		printTail(logPath, 40)
	}
	return d, outputs
}

func (r *runner) execInstance(ctx context.Context, in instance, sc *ctx, out io.Writer) (string, map[string]string) {
	j := in.job
	imageExpr, options := j.container()
	image, err := sc.expand(imageExpr)
	if err != nil {
		fmt.Fprintln(out, "ci-local:", err)
		return "failure", nil
	}
	dir := filepath.Join(r.state, "w", in.key)
	if !r.dry {
		if dir, err = r.worktree(in.key); err != nil {
			fmt.Fprintln(out, "ci-local:", err)
			return "failure", nil
		}
	}
	tmp := filepath.Join(r.state, "tmp", in.key)
	home := filepath.Join(r.state, "home", in.key)
	for _, p := range []string{tmp, home} {
		_ = os.RemoveAll(p)
		_ = os.MkdirAll(p, 0o777)
	}
	inContainer := image != ""
	wsPath, tmpPath, homePath := dir, tmp, home
	if inContainer {
		wsPath, tmpPath, homePath = workspace, "/__w/_temp", "/github/home"
	}
	env := map[string]string{
		"CI": "true", "GITHUB_ACTIONS": "true", "GITHUB_JOB": j.ID,
		"GITHUB_WORKSPACE": wsPath, "RUNNER_TEMP": tmpPath,
		"GITHUB_REPOSITORY": "ingyamilmolinar/beatmo",
	}
	if inContainer {
		env["HOME"] = homePath
	}
	for _, layer := range []map[string]string{r.wf.Env, j.Env} {
		for k, v := range layer {
			ev, err := sc.expand(v)
			if err != nil {
				fmt.Fprintln(out, "ci-local:", err)
				return "failure", nil
			}
			env[k] = ev
		}
	}
	name := ""
	if inContainer {
		name = fmt.Sprintf("beatmo-ci-%s-%d", in.key, os.Getpid())
		args := []string{"run", "-d", "--name", name, "--workdir", workspace,
			"-v", dir + ":" + workspace, "-v", r.base + ":" + r.base,
			"-v", tmp + ":/__w/_temp", "-v", home + ":/github/home"}
		for _, cdir := range []string{"go-build", "gomod", "npm", "golangci-lint"} {
			host := filepath.Join(r.state, "cache", cdir)
			_ = os.MkdirAll(host, 0o777)
			args = append(args, "-v", host+":/cache/"+cdir)
		}
		args = append(args, r.limits...)
		args = append(args, strings.Fields(options)...)
		args = append(args, "--entrypoint", "tail", image, "-f", "/dev/null")
		fmt.Fprintf(out, "$ %s %s\n", r.rt, strings.Join(args, " "))
		if !r.dry {
			start := exec.CommandContext(ctx, r.rt, args...)
			start.Stdout, start.Stderr = out, out
			if err := start.Run(); err != nil {
				fmt.Fprintln(out, "ci-local: starting the job container:", err)
				return "failure", nil
			}
			r.mu.Lock()
			r.containers[name] = true
			r.mu.Unlock()
			defer r.stopContainer(name, out)
		}
	}
	failed := false
	for i, st := range j.Steps {
		ok, err := sc.cond(st.If)
		if err != nil {
			fmt.Fprintln(out, "ci-local:", err)
			failed, sc.failed = true, true
			continue
		}
		if !ok {
			continue
		}
		label := st.Name
		if label == "" {
			label = st.Uses
		}
		if label == "" {
			label = st.Run
		}
		fmt.Fprintf(out, "\n── step %d: %s\n", i+1, label)
		if err := r.step(ctx, sc, st, i, env, name, dir, tmp, tmpPath, out); err != nil {
			fmt.Fprintf(out, "ci-local: step failed: %v\n", err)
			failed, sc.failed = true, true
		}
	}
	outputs := map[string]string{}
	for k, v := range j.Outputs {
		if ev, err := sc.expand(v); err == nil {
			outputs[k] = ev
		}
	}
	if summary := collectSummaries(tmp); summary != "" {
		_ = os.WriteFile(filepath.Join(r.state, "logs", in.key+".summary.md"), []byte(summary), 0o644)
	}
	if failed {
		return "failure", outputs
	}
	if !r.keep && !r.dry {
		_, _ = r.git(nil, "-C", r.base, "worktree", "remove", "--force", dir)
	}
	return "success", outputs
}

func (r *runner) step(ctx context.Context, sc *ctx, st step, i int, jobEnv map[string]string,
	container, dir, tmp, tmpPath string, out io.Writer) error {
	uses := st.Uses
	if at := strings.Index(uses, "@"); at >= 0 {
		uses = uses[:at]
	}
	with := map[string]string{}
	for k, v := range st.With {
		ev, err := sc.expand(v)
		if err != nil {
			return err
		}
		with[k] = ev
	}
	switch uses {
	case "actions/checkout":
		fmt.Fprintln(out, "checked out the snapshot commit at", dir)
		return nil
	case "actions/cache":
		fmt.Fprintln(out, "caches are local volumes under", filepath.Join(r.state, "cache"))
		return nil
	case "actions/upload-artifact":
		return r.upload(dir, with, out)
	case "actions/download-artifact":
		return r.download(dir, with, out)
	case "":
	default:
		return fmt.Errorf("action %s is not supported by ci-local", st.Uses)
	}
	script, err := sc.expand(st.Run)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for k, v := range jobEnv {
		env[k] = v
	}
	for k, v := range st.Env {
		ev, err := sc.expand(v)
		if err != nil {
			return err
		}
		env[k] = ev
	}
	outFile := fmt.Sprintf("set_output_%d", i)
	sumFile := fmt.Sprintf("step_summary_%d", i)
	env["GITHUB_OUTPUT"] = path.Join(tmpPath, outFile)
	env["GITHUB_STEP_SUMMARY"] = path.Join(tmpPath, sumFile)
	_ = os.WriteFile(filepath.Join(tmp, outFile), nil, 0o666)
	_ = os.WriteFile(filepath.Join(tmp, sumFile), nil, 0o666)
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var cmd *exec.Cmd
	if container != "" {
		args := []string{"exec", "-w", workspace}
		for _, k := range keys {
			args = append(args, "-e", k+"="+env[k])
		}
		args = append(args, container, "bash", "-e", "-c", script)
		cmd = exec.CommandContext(ctx, r.rt, args...)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-e", "-c", script)
		cmd.Dir = dir
		cmd.Env = os.Environ()
		for _, k := range keys {
			cmd.Env = append(cmd.Env, k+"="+env[k])
		}
	}
	fmt.Fprintf(out, "$ %s\n", script)
	if r.dry {
		return nil
	}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Run()
	if st.ID != "" {
		sc.steps[st.ID] = readOutputs(filepath.Join(tmp, outFile))
	}
	return err
}

func (r *runner) stopContainer(name string, out io.Writer) {
	if r.rootful {
		chown := exec.Command(r.rt, "exec", name, "chown", "-R", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			workspace, "/__w/_temp", "/github/home", "/cache")
		chown.Stdout, chown.Stderr = out, out
		_ = chown.Run()
	}
	_ = exec.Command(r.rt, "rm", "-f", name).Run()
	r.mu.Lock()
	delete(r.containers, name)
	r.mu.Unlock()
}

func (r *runner) stopAll() {
	r.mu.Lock()
	names := make([]string, 0, len(r.containers))
	for n := range r.containers {
		names = append(names, n)
	}
	r.mu.Unlock()
	for _, n := range names {
		r.stopContainer(n, io.Discard)
	}
}

func (r *runner) artifactsDir() string { return filepath.Join(r.state, "artifacts") }

// upload mirrors upload-artifact@v4: the files under the listed paths, stored
// relative to their least common ancestor.
func (r *runner) upload(dir string, with map[string]string, out io.Writer) error {
	var paths []string
	for _, p := range strings.Split(with["path"], "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, filepath.Join(dir, p))
		}
	}
	dst := filepath.Join(r.artifactsDir(), with["name"])
	_ = os.RemoveAll(dst)
	base := commonDir(paths)
	n := 0
	for _, p := range paths {
		_ = filepath.Walk(p, func(f string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(base, f)
			if copyFile(f, filepath.Join(dst, rel)) == nil {
				n++
			}
			return nil
		})
	}
	fmt.Fprintf(out, "artifact %s: %d files\n", with["name"], n)
	return nil
}

func (r *runner) download(dir string, with map[string]string, out io.Writer) error {
	entries, err := os.ReadDir(r.artifactsDir())
	if err != nil {
		return fmt.Errorf("no artifacts: %w", err)
	}
	target := filepath.Join(dir, with["path"])
	merge := with["merge-multiple"] == "true"
	n := 0
	for _, e := range entries {
		name := e.Name()
		if want := with["name"]; want != "" && name != want {
			continue
		}
		if pat := with["pattern"]; pat != "" {
			if ok, _ := path.Match(pat, name); !ok {
				continue
			}
		}
		src := filepath.Join(r.artifactsDir(), name)
		dst := target
		if !merge && with["name"] == "" {
			dst = filepath.Join(target, name)
		}
		_ = filepath.Walk(src, func(f string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(src, f)
			if copyFile(f, filepath.Join(dst, rel)) == nil {
				n++
			}
			return nil
		})
	}
	fmt.Fprintf(out, "downloaded %d files into %s\n", n, with["path"])
	return nil
}

func commonDir(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	dirOf := func(p string) string {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		return filepath.Dir(p)
	}
	base := dirOf(paths[0])
	for _, p := range paths[1:] {
		d := dirOf(p)
		for !strings.HasPrefix(d+string(filepath.Separator), base+string(filepath.Separator)) {
			base = filepath.Dir(base)
		}
	}
	return base
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

// readOutputs parses a GITHUB_OUTPUT file: name=value lines and
// name<<DELIM heredocs.
func readOutputs(file string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(file)
	if err != nil {
		return out
	}
	lines := strings.Split(string(b), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if k, delim, ok := strings.Cut(l, "<<"); ok && !strings.Contains(k, "=") {
			var val []string
			for i++; i < len(lines) && lines[i] != delim; i++ {
				val = append(val, lines[i])
			}
			out[k] = strings.Join(val, "\n")
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			out[k] = v
		}
	}
	return out
}

func collectSummaries(tmp string) string {
	files, _ := filepath.Glob(filepath.Join(tmp, "step_summary_*"))
	sort.Slice(files, func(a, b int) bool {
		na, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(files[a]), "step_summary_"))
		nb, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(files[b]), "step_summary_"))
		return na < nb
	})
	var sb strings.Builder
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			sb.Write(b)
		}
	}
	return sb.String()
}

func printTail(file string, n int) {
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		fmt.Println("    " + l)
	}
}

func prefixed(w io.Writer, tag string) io.Writer {
	pr, pw := io.Pipe()
	go func() {
		s := bufio.NewScanner(pr)
		s.Buffer(make([]byte, 1<<20), 1<<20)
		for s.Scan() {
			fmt.Fprintf(w, "[%s] %s\n", tag, s.Text())
		}
	}()
	return pw
}
