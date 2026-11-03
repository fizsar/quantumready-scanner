package cipolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// workflow is the part of a workflow file the rules look at.
type workflow struct {
	file        string
	On          map[string]any `yaml:"on"`
	Permissions any            `yaml:"permissions"`
	Jobs        map[string]job `yaml:"jobs"`
	raw         string
}

type job struct {
	If          string `yaml:"if"`
	Permissions any    `yaml:"permissions"`
	Steps       []step `yaml:"steps"`
}

type step struct {
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func load(t *testing.T) []workflow {
	t.Helper()
	files, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows: %v", err)
	}
	var out []workflow
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var w workflow
		if err := yaml.Unmarshal(b, &w); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		w.file, w.raw = filepath.Base(f), string(b)
		out = append(out, w)
	}
	return out
}

// writes lists the permissions a block grants beyond read ("write-all" too).
func writes(p any) []string {
	switch v := p.(type) {
	case string:
		if v == "write-all" {
			return []string{"write-all"}
		}
	case map[string]any:
		var w []string
		for k, a := range v {
			if a == "write" {
				w = append(w, k)
			}
		}
		slices.Sort(w)
		return w
	}
	return nil
}

func (w workflow) triggers() []string {
	var t []string
	for k := range w.On {
		t = append(t, k)
	}
	slices.Sort(t)
	return t
}

var pinned = regexp.MustCompile(`^[\w.-]+/[\w./-]+@[0-9a-f]{40}$`)

func TestWorkflowPolicy(t *testing.T) {
	for _, w := range load(t) {
		trig := w.triggers()
		fromPRs := slices.Contains(trig, "pull_request")
		// Never pull_request_target, nor anything that runs a fork's code with this repository's token.
		if slices.Contains(trig, "pull_request_target") || strings.Contains(w.raw, "pull_request_target") {
			t.Errorf("%s: pull_request_target", w.file)
		}
		// Read-only by default: nothing written at the workflow level.
		if w.Permissions == nil {
			t.Errorf("%s: no top-level permissions (the default token may write)", w.file)
		} else if wr := writes(w.Permissions); len(wr) > 0 {
			t.Errorf("%s: top-level write permissions %v", w.file, wr)
		}
		// Secrets: only the job's own token, and none at all where a pull request runs.
		for _, m := range regexp.MustCompile(`secrets\.(\w+)`).FindAllStringSubmatch(w.raw, -1) {
			if m[1] != "GITHUB_TOKEN" || fromPRs {
				t.Errorf("%s: secrets.%s", w.file, m[1])
			}
		}
		for name, j := range w.Jobs {
			wr := writes(j.Permissions)
			// Writes only where no pull request runs, except code scanning's
			// upload (read-only for forks, which GitHub enforces).
			if fromPRs && slices.ContainsFunc(wr, func(p string) bool { return p != "security-events" }) {
				t.Errorf("%s/%s: writes %v and runs on pull requests", w.file, name, wr)
			}
			for i, s := range j.Steps {
				where := fmt.Sprintf("%s/%s/step %d", w.file, name, i)
				if s.Uses != "" && !strings.HasPrefix(s.Uses, "./") && !pinned.MatchString(s.Uses) {
					t.Errorf("%s: %q not pinned to a commit SHA", where, s.Uses)
				}
				if strings.HasPrefix(s.Uses, "actions/checkout@") && s.With["persist-credentials"] != false {
					t.Errorf("%s: checkout keeps the token in .git/config (persist-credentials: false)", where)
				}
				// Script injection: event fields and inputs go through env, never into a script.
				if regexp.MustCompile(`\$\{\{\s*(github\.event|github\.head_ref|inputs\.)`).MatchString(s.Run) {
					t.Errorf("%s: an event field or input expanded inside run:", where)
				}
			}
		}
	}
}

// Manual workflows that write: dispatched by hand, and run for the owner only.
func TestWritingWorkflowsAreManualOrGuarded(t *testing.T) {
	for _, w := range load(t) {
		for name, j := range w.Jobs {
			wr := writes(j.Permissions)
			if len(wr) == 0 || slices.Equal(wr, []string{"security-events"}) || (w.file == "scorecard.yml" && slices.Equal(wr, []string{"id-token", "security-events"})) {
				continue
			}
			switch w.file {
			case "release.yml": // automatic, guarded below (TestReleaseOnlyFromOwnPushes)
			case "ghcr-cleanup.yml":
				if !slices.Equal(w.triggers(), []string{"workflow_dispatch"}) {
					t.Errorf("%s: triggers %v, want workflow_dispatch only", w.file, w.triggers())
				}
			default:
				t.Errorf("%s/%s writes %v: add it to this test with its guard", w.file, name, wr)
			}
		}
	}
}

// eval evaluates a job's if: as GitHub would for the expressions used here:
// clauses joined by &&, each "a == b" with a context path or a quoted string.
func eval(t *testing.T, cond string, ctx map[string]any) bool {
	t.Helper()
	cond = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(cond), "${{"), "}}")
	value := func(s string) string {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
			return strings.Trim(s, "'")
		}
		var cur any = ctx
		for _, k := range strings.Split(s, ".") {
			m, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur = m[k]
		}
		v, _ := cur.(string)
		return v
	}
	for _, clause := range strings.Split(cond, "&&") {
		a, b, ok := strings.Cut(clause, "==")
		if !ok || strings.ContainsAny(clause, "|!") {
			t.Fatalf("clause %q: only a == b joined by && is understood here", clause)
		}
		if value(a) != value(b) {
			return false
		}
	}
	return true
}

func find(t *testing.T, file, jobName string) job {
	t.Helper()
	for _, w := range load(t) {
		if w.file == file {
			if j, ok := w.Jobs[jobName]; ok {
				return j
			}
		}
	}
	t.Fatalf("%s: no job %s", file, jobName)
	return job{}
}

// The Release (images to GHCR, statuses on commits) runs after CI only for a
// push to this repository by its owner: a pull request from a fork never
// publishes, whatever its branch is called.
func TestReleaseOnlyFromOwnPushes(t *testing.T) {
	cond := find(t, "release.yml", "publish").If
	run := func(event, conclusion, headRepo, branch, actor string) map[string]any {
		return map[string]any{"github": map[string]any{
			"repository": "fizsar/quantumready-saas", "repository_owner": "fizsar",
			"event": map[string]any{"workflow_run": map[string]any{
				"event": event, "conclusion": conclusion, "head_branch": branch,
				"head_repository": map[string]any{"full_name": headRepo},
				"actor":           map[string]any{"login": actor},
			}},
		}}
	}
	for _, tc := range []struct {
		name string
		ctx  map[string]any
		want bool
	}{
		{"push to main by the owner", run("push", "success", "fizsar/quantumready-saas", "main", "fizsar"), true},
		{"push to showcase by the owner", run("push", "success", "fizsar/quantumready-saas", "showcase", "fizsar"), true},
		{"CI failed", run("push", "failure", "fizsar/quantumready-saas", "main", "fizsar"), false},
		{"pull request from a fork, branch named main", run("pull_request", "success", "mallory/quantumready-saas", "main", "mallory"), false},
		{"pull request from this repository", run("pull_request", "success", "fizsar/quantumready-saas", "fix/x", "fizsar"), false},
		{"a push event whose code is a fork's", run("push", "success", "mallory/quantumready-saas", "main", "fizsar"), false},
		{"a push by someone else", run("push", "success", "fizsar/quantumready-saas", "main", "collaborator"), false},
	} {
		if got := eval(t, cond, tc.ctx); got != tc.want {
			t.Errorf("%s: runs=%v, want %v (if: %s)", tc.name, got, tc.want, cond)
		}
	}
}

// The GHCR cleanup deletes packages: only when the owner dispatches it.
func TestCleanupOnlyForTheOwner(t *testing.T) {
	cond := find(t, "ghcr-cleanup.yml", "cleanup").If
	ctx := func(actor, triggering string) map[string]any {
		return map[string]any{"github": map[string]any{"repository_owner": "fizsar", "actor": actor, "triggering_actor": triggering}}
	}
	if !eval(t, cond, ctx("fizsar", "fizsar")) {
		t.Error("the owner cannot run it")
	}
	for _, c := range [][2]string{{"collaborator", "collaborator"}, {"fizsar", "collaborator"}, {"collaborator", "fizsar"}} {
		if eval(t, cond, ctx(c[0], c[1])) {
			t.Errorf("runs for actor %s, triggering actor %s", c[0], c[1])
		}
	}
}
