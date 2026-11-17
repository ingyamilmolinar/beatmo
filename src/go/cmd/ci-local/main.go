// Command ci-local runs .github/workflows/ci.yml on this machine: every job in
// the image CI runs it in (the same content-hash tag), at CI's workspace path,
// with a GitHub runner's 4 CPUs / 16 GB, its steps exec'd into one container
// per job as GitHub does, artifacts and job outputs passed between jobs. It
// reads the workflow itself, so a job added to ci.yml runs here unchanged.
//
//	make ci-local                        # every job
//	make ci-local JOBS="lint test-go/2"  # a job, or one matrix instance
//	make ci-local-list                   # jobs and their instances
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

func main() { os.Exit(realMain(os.Args[1:])) }

func realMain(argv []string) int {
	flag := flag.NewFlagSet("ci-local", flag.ContinueOnError)
	list := flag.Bool("list", false, "print the jobs and their matrix instances, run nothing")
	jobs := flag.Int("j", max(1, runtime.NumCPU()/4), "job instances run at once (each gets a runner's 4 CPUs)")
	cpus := flag.String("cpus", "4", "CPUs per job container (GitHub ubuntu-latest: 4); empty = no limit")
	memory := flag.String("memory", "16g", "memory per job container (GitHub ubuntu-latest: 16 GB); empty = no limit")
	rt := flag.String("runtime", os.Getenv("CONTAINER"), "podman or docker (default: whichever is installed, podman first)")
	keep := flag.Bool("keep", false, "keep each job's checkout after it succeeds (a failed one is always kept)")
	committed := flag.Bool("committed", false, "run HEAD exactly, without the tracked working-tree changes")
	dry := flag.Bool("n", false, "print what would run")
	repo := flag.String("C", ".", "the repository to run (its .github/workflows/ci.yml)")
	stateDir := flag.String("state", "", "scratch directory (default $XDG_CACHE_HOME/beatmo-ci)")
	if err := flag.Parse(argv); err != nil {
		return 2
	}

	top := exec.Command("git", "rev-parse", "--show-toplevel")
	top.Dir = *repo
	root, err := top.Output()
	if err != nil {
		return fail("not inside the repository: %v", err)
	}
	r := &runner{root: strings.TrimSpace(string(root)), keep: *keep, dry: *dry, jobs: *jobs,
		vars: map[string]string{"BROWSER_BATCH_SIZE": os.Getenv("BROWSER_BATCH_SIZE")}}
	r.wf, err = loadWorkflow(filepath.Join(r.root, ".github/workflows/ci.yml"))
	if err != nil {
		return fail("%v", err)
	}
	sel, filter := parseSelection(flag.Args())
	order, err := r.wf.order(sel)
	if err != nil {
		return fail("%v", err)
	}
	if *list {
		printJobs(order)
		return 0
	}

	if r.rt = *rt; r.rt == "" {
		for _, c := range []string{"podman", "docker"} {
			if p, err := exec.LookPath(c); err == nil {
				r.rt = p
				break
			}
		}
	}
	if r.rt == "" && !r.dry {
		return fail("ci-local needs podman or docker: run `make dependencies` (installs podman)")
	}
	r.rootful = !r.dry && filepath.Base(r.rt) == "docker" && os.Getuid() != 0 && !dockerRootless(r.rt)
	if *cpus != "" || *memory != "" {
		if limitsSupported(r.rt) || r.dry {
			if *cpus != "" {
				r.limits = append(r.limits, "--cpus", *cpus)
			}
			if *memory != "" {
				r.limits = append(r.limits, "--memory", *memory)
			}
		} else {
			fmt.Println("ci-local: WARNING: this container runtime cannot limit CPU/memory (rootless cgroup " +
				"controllers not delegated; `make dependencies` sets that up) — jobs see the whole machine, unlike a CI runner")
		}
	}
	if r.state = *stateDir; r.state == "" {
		cache := os.Getenv("XDG_CACHE_HOME")
		if cache == "" {
			home, _ := os.UserHomeDir()
			cache = filepath.Join(home, ".cache")
		}
		r.state = filepath.Join(cache, "beatmo-ci")
	}
	r.base = filepath.Join(r.state, "base.git")
	_ = os.RemoveAll(filepath.Join(r.state, "artifacts"))
	_ = os.RemoveAll(filepath.Join(r.state, "logs"))
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		return fail("%v", err)
	}
	commit, err := r.snapshot(*committed)
	if err != nil {
		return fail("%v", err)
	}
	r.stream = r.jobs == 1

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nci-local: interrupted — stopping job containers")
		cancel()
		r.stopAll()
		os.Exit(130)
	}()

	start := time.Now()
	fmt.Printf("ci-local: commit %s, %d job(s) at a time, runtime %s, state %s\n", commit[:12], r.jobs, r.rt, r.state)
	ok := r.run(ctx, order, filter)
	fmt.Printf("\n%-55s %-8s %s\n", "JOB", "RESULT", "TIME")
	sort.SliceStable(r.done, func(a, b int) bool { return r.done[a].key < r.done[b].key })
	for _, d := range r.done {
		fmt.Printf("%-55s %-8s %s\n", d.name, d.result, d.dur.Round(time.Second))
	}
	fmt.Printf("total %s; logs and step summaries in %s\n", time.Since(start).Round(time.Second), filepath.Join(r.state, "logs"))
	signal.Stop(sig)
	if !ok {
		return 1
	}
	return 0
}

func printJobs(order []*job) {
	c := &ctx{needs: map[string]*result{}, vars: map[string]string{}}
	for _, j := range order {
		image, _ := j.container()
		where := "host"
		if image != "" {
			where = strings.TrimSpace(exprRE.ReplaceAllString(image, "$1"))
		}
		fmt.Printf("%-16s %-34s needs %v\n", j.ID, where, j.needs())
		if j.Strategy.Matrix.Kind != 0 {
			c.jobID = j.ID
			if insts, err := j.instances(c); err == nil {
				for _, in := range insts {
					fmt.Printf("    %s  (%s)\n", in.key, in.name)
				}
			} else {
				fmt.Printf("    matrix computed at run time (%s)\n", strings.TrimSpace(j.Strategy.Matrix.Value))
			}
		}
	}
}

func dockerRootless(rt string) bool {
	out, err := exec.Command(rt, "info", "--format", "{{.SecurityOptions}}").Output()
	return err == nil && strings.Contains(string(out), "rootless")
}

// limitsSupported: rootless podman can only apply --cpus/--memory when systemd
// delegates those cgroup controllers to the user.
func limitsSupported(rt string) bool {
	if filepath.Base(rt) != "podman" {
		return true
	}
	out, err := exec.Command(rt, "info", "--format", "{{.Host.CgroupControllers}}").Output()
	if err != nil {
		return false
	}
	s := string(out)
	return strings.Contains(s, "cpu") && strings.Contains(s, "memory")
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "ci-local: "+format+"\n", a...)
	return 2
}
