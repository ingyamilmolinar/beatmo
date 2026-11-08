package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// workspace is where GitHub mounts the checkout in a container job; jobs run
// there locally too, so a path a target derives from it is the same path.
const workspace = "/__w/beatmo/beatmo"

type workflow struct {
	Env  map[string]string `yaml:"env"`
	Jobs map[string]*job   `yaml:"jobs"`
}

type job struct {
	ID        string            `yaml:"-"`
	Name      string            `yaml:"name"`
	Needs     yaml.Node         `yaml:"needs"`
	If        string            `yaml:"if"`
	Container yaml.Node         `yaml:"container"`
	Env       map[string]string `yaml:"env"`
	Outputs   map[string]string `yaml:"outputs"`
	Strategy  struct {
		Matrix yaml.Node `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []step `yaml:"steps"`
}

type step struct {
	ID   string            `yaml:"id"`
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func loadWorkflow(path string) (*workflow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for id, j := range w.Jobs {
		j.ID = id
	}
	return &w, nil
}

func (j *job) needs() []string {
	switch j.Needs.Kind {
	case yaml.ScalarNode:
		return []string{j.Needs.Value}
	case yaml.SequenceNode:
		var out []string
		for _, n := range j.Needs.Content {
			out = append(out, n.Value)
		}
		return out
	}
	return nil
}

// container returns the job's image expression and docker options; an empty
// image means the job runs on the host.
func (j *job) container() (image, options string) {
	switch j.Container.Kind {
	case yaml.ScalarNode:
		return j.Container.Value, ""
	case yaml.MappingNode:
		for i := 0; i+1 < len(j.Container.Content); i += 2 {
			switch j.Container.Content[i].Value {
			case "image":
				image = j.Container.Content[i+1].Value
			case "options":
				options = j.Container.Content[i+1].Value
			}
		}
	}
	return image, options
}

// ctx is what an expression can read.
type ctx struct {
	jobID  string
	matrix map[string]string
	needs  map[string]*result
	steps  map[string]map[string]string
	failed bool
	vars   map[string]string
}

type result struct {
	Result  string            `json:"result"`
	Outputs map[string]string `json:"outputs"`
}

var exprRE = regexp.MustCompile(`\$\{\{\s*(.*?)\s*\}\}`)

// expand substitutes every ${{ }} in s. An expression this runner does not
// know is an error, so a workflow edit cannot silently run differently here.
func (c *ctx) expand(s string) (string, error) {
	var firstErr error
	out := exprRE.ReplaceAllStringFunc(s, func(m string) string {
		v, err := c.eval(exprRE.FindStringSubmatch(m)[1])
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return v
	})
	return out, firstErr
}

var (
	orDefaultRE = regexp.MustCompile(`^vars\.(\w+)\s*\|\|\s*'([^']*)'$`)
	needsOutRE  = regexp.MustCompile(`^needs\.([\w-]+)\.outputs\.([\w-]+)$`)
	stepsOutRE  = regexp.MustCompile(`^steps\.([\w-]+)\.outputs\.([\w-]+)$`)
	matrixRE    = regexp.MustCompile(`^matrix\.([\w-]+)$`)
)

func (c *ctx) eval(e string) (string, error) {
	switch {
	case e == "github.workspace":
		return workspace, nil
	case e == "github.job":
		return c.jobID, nil
	case e == "toJSON(needs)":
		b, err := json.Marshal(c.needs)
		return string(b), err
	case strings.HasPrefix(e, "secrets."):
		return "", nil
	case strings.HasPrefix(e, "hashFiles("):
		return "local", nil
	}
	if m := orDefaultRE.FindStringSubmatch(e); m != nil {
		if v := c.vars[m[1]]; v != "" {
			return v, nil
		}
		return m[2], nil
	}
	if m := matrixRE.FindStringSubmatch(e); m != nil {
		if v, ok := c.matrix[m[1]]; ok {
			return v, nil
		}
		return "", fmt.Errorf("matrix.%s is not set for %s", m[1], c.jobID)
	}
	if m := needsOutRE.FindStringSubmatch(e); m != nil {
		r := c.needs[m[1]]
		if r == nil {
			return "", fmt.Errorf("%s reads needs.%s, which has not run", c.jobID, m[1])
		}
		return r.Outputs[m[2]], nil
	}
	if m := stepsOutRE.FindStringSubmatch(e); m != nil {
		return c.steps[m[1]][m[2]], nil
	}
	return "", fmt.Errorf("expression ${{ %s }} is not supported by ci-local", e)
}

// cond evaluates a job or step `if:`; empty means success().
func (c *ctx) cond(e string) (bool, error) {
	e = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(e), "${{"), "}}"))
	switch e {
	case "", "success()":
		return !c.failed, nil
	case "always()", "!cancelled()", "${{ !cancelled() }}":
		return true, nil
	case "failure()":
		return c.failed, nil
	}
	return false, fmt.Errorf("condition %q is not supported by ci-local", e)
}

// instance is one run of a job: a matrix combination, or the job itself.
type instance struct {
	job    *job
	matrix map[string]string
	key    string // unique, filesystem-safe
	name   string
}

// instances expands a job's matrix; a matrix computed by another job
// (fromJson(needs.X.outputs.Y)) is read from that job's result.
func (j *job) instances(c *ctx) ([]instance, error) {
	var combos []map[string]string
	m := j.Strategy.Matrix
	switch m.Kind {
	case 0:
		combos = []map[string]string{nil}
	case yaml.ScalarNode:
		inner := strings.TrimSpace(exprRE.ReplaceAllString(m.Value, "$1"))
		if !strings.HasPrefix(inner, "fromJson(") || !strings.HasSuffix(inner, ")") {
			return nil, fmt.Errorf("%s: matrix %q is not supported by ci-local", j.ID, m.Value)
		}
		raw, err := c.eval(strings.TrimSuffix(strings.TrimPrefix(inner, "fromJson("), ")"))
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Include []map[string]any `json:"include"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("%s: matrix JSON: %w", j.ID, err)
		}
		for _, inc := range parsed.Include {
			combo := map[string]string{}
			for k, v := range inc {
				combo[k] = fmt.Sprint(v)
			}
			combos = append(combos, combo)
		}
	case yaml.MappingNode:
		var axes map[string][]string
		if err := m.Decode(&axes); err != nil {
			return nil, fmt.Errorf("%s: matrix: %w", j.ID, err)
		}
		keys := make([]string, 0, len(axes))
		for k := range axes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		combos = []map[string]string{{}}
		for _, k := range keys {
			var next []map[string]string
			for _, base := range combos {
				for _, v := range axes[k] {
					c := map[string]string{}
					for bk, bv := range base {
						c[bk] = bv
					}
					c[k] = v
					next = append(next, c)
				}
			}
			combos = next
		}
	}
	var out []instance
	for _, combo := range combos {
		in := instance{job: j, matrix: combo, key: j.ID}
		if len(combo) > 0 {
			in.key += "-" + instanceSuffix(combo)
		}
		cc := *c
		cc.jobID, cc.matrix = j.ID, combo
		name := j.Name
		if name == "" {
			name = j.ID
		}
		n, err := cc.expand(name)
		if err != nil {
			return nil, err
		}
		in.name = strings.Trim(n, `"`)
		out = append(out, in)
	}
	return out, nil
}

var unsafeRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func instanceSuffix(combo map[string]string) string {
	if v, ok := combo["shard"]; ok {
		return v
	}
	if v, ok := combo["index"]; ok {
		return v
	}
	keys := make([]string, 0, len(combo))
	for k := range combo {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, unsafeRE.ReplaceAllString(combo[k], "_"))
	}
	s := strings.Join(parts, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// order returns the jobs to run for the selection, needs first; nil selects
// every job.
func (w *workflow) order(selected []string) ([]*job, error) {
	want := map[string]bool{}
	var add func(id string) error
	add = func(id string) error {
		j := w.Jobs[id]
		if j == nil {
			return fmt.Errorf("ci.yml has no job %q (try -list)", id)
		}
		if want[id] {
			return nil
		}
		want[id] = true
		for _, n := range j.needs() {
			if err := add(n); err != nil {
				return err
			}
		}
		return nil
	}
	if len(selected) == 0 {
		for id := range w.Jobs {
			selected = append(selected, id)
		}
	}
	for _, id := range selected {
		if err := add(id); err != nil {
			return nil, err
		}
	}
	var out []*job
	done := map[string]bool{}
	for len(out) < len(want) {
		var ready []string
		for id := range want {
			if done[id] {
				continue
			}
			ok := true
			for _, n := range w.Jobs[id].needs() {
				ok = ok && done[n]
			}
			if ok {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("ci.yml needs form a cycle")
		}
		sort.Strings(ready)
		for _, id := range ready {
			done[id] = true
			out = append(out, w.Jobs[id])
		}
	}
	return out, nil
}

// parseSelection splits "test-go/2" into the job and a matrix filter.
func parseSelection(args []string) (jobs []string, filter map[string]string) {
	filter = map[string]string{}
	for _, a := range args {
		id, f, _ := strings.Cut(a, "/")
		jobs = append(jobs, id)
		if f != "" {
			filter[id] = f
		}
	}
	return jobs, filter
}

func (in instance) matches(filter string) bool {
	if filter == "" {
		return true
	}
	for _, v := range in.matrix {
		if v == filter {
			return true
		}
	}
	if n, err := strconv.Atoi(filter); err == nil {
		return in.matrix["shard"] == strconv.Itoa(n) || in.matrix["index"] == strconv.Itoa(n)
	}
	return false
}
