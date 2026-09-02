// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/veans/internal/client"
)

// TestPlan_LintsThenCreates: a plan with a cycle creates nothing; a clean
// plan creates tasks, relations and scopes in one call.
func TestPlan_LintsThenCreates(t *testing.T) {
	ws, h := provisionWorkspace(t)

	bad := filepath.Join(ws.Dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"tasks":[{"key":"a","title":"A","blocked_by":["b"]},{"key":"b","title":"B","blocked_by":["a"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := h.Run(t, ws, "plan", bad)
	if code == 0 || !strings.Contains(errOut, `"CONFLICT"`) || !strings.Contains(out, "dependency_cycle") {
		t.Fatalf("a cyclic plan must fail with CONFLICT and print the finding; exit %d stdout %s stderr %s", code, out, errOut)
	}

	good := filepath.Join(ws.Dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"tasks":[
		{"key":"epic","title":"claiming"},
		{"key":"api","title":"claim endpoint","parent_key":"epic","priority":3,"scope":{"paths_owned":["pkg/models/**"],"paths_affected":[],"endpoints":["POST /api/v2/tasks/{id}/claim"],"notes":""}},
		{"key":"ui","title":"board badges","parent_key":"epic","blocked_by":["api"],"scope":{"paths_owned":["frontend/**"],"paths_affected":[],"endpoints":[],"notes":""}}
	]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = h.Run(t, ws, "plan", "--dry-run", good)
	if code != 0 {
		t.Fatalf("dry run exit %d\n%s", code, errOut)
	}
	var res client.PlanResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Created || len(res.Tasks) != 0 {
		t.Fatalf("dry run must lint only: %+v", res)
	}

	out, errOut, code = h.Run(t, ws, "plan", good)
	if code != 0 {
		t.Fatalf("plan exit %d\n%s", code, errOut)
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Created || len(res.Tasks) != 3 {
		t.Fatalf("expected three created tasks: %+v", res)
	}
	var api, ui client.PlannedTaskResult
	for _, r := range res.Tasks {
		switch r.Key {
		case "api":
			api = r
		case "ui":
			ui = r
		}
	}
	server := h.GetTask(t, ui.ID)
	if server.Scope == nil || len(server.Scope.PathsOwned) != 1 || server.Scope.PathsOwned[0] != "frontend/**" {
		t.Fatalf("scope not stored from the plan: %+v", server.Scope)
	}
	blocked := server.RelatedTasks["blocked"]
	if len(blocked) != 1 || blocked[0].ID != api.ID {
		t.Fatalf("ui must be blocked by api: %+v", server.RelatedTasks)
	}
	// ui is not claimable until api is done.
	if r := readiness(t, h, ws, ui.ID); r.Ready || !contains(r.Reasons, client.ReadinessReasonBlocked) {
		t.Fatalf("ui must be blocked: %+v", r)
	}
}

// TestCheck_FlagsStraysAndCollisions drives veans check through real
// commits: inside the scope passes, outside fails, another task's lease fails.
func TestCheck_FlagsStraysAndCollisions(t *testing.T) {
	ws, h := provisionWorkspace(t)

	mine := createTask(t, h, ws, "models work", "--paths-owned", "pkg/models/**")
	other := createTask(t, h, ws, "routes work", "--paths-owned", "pkg/routes/**")
	if _, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", other.Index)); code != 0 {
		t.Fatalf("claim other exit %d\n%s", code, errOut)
	}

	// provisionWorkspace leaves .veans.yml and the hook settings untracked;
	// they belong to main, not to the change under test.
	gitInWorkspace(t, ws, "add", "-A")
	gitInWorkspace(t, ws, "commit", "-q", "-m", "chore: veans init")
	gitInWorkspace(t, ws, "checkout", "-q", "-b", "feat-check")
	writeFile := func(rel string) {
		t.Helper()
		p := filepath.Join(ws.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("pkg/models/new.go")
	gitInWorkspace(t, ws, "add", ".")
	gitInWorkspace(t, ws, "commit", "-q", "-m", fmt.Sprintf("feat: inside\n\nRefs: %d", mine.Index))

	out, errOut, code := h.Run(t, ws, "check", "--base", "main")
	if code != 0 {
		t.Fatalf("an in-scope change must pass; exit %d stdout %s stderr %s", code, out, errOut)
	}

	writeFile("docs/README.md")
	gitInWorkspace(t, ws, "add", ".")
	gitInWorkspace(t, ws, "commit", "-q", "-m", "docs: stray")
	out, errOut, code = h.Run(t, ws, "check", "--base", "main")
	if code == 0 || !strings.Contains(errOut, `"CONFLICT"`) {
		t.Fatalf("a stray file must fail with CONFLICT; exit %d stderr %s", code, errOut)
	}
	var res client.ScopeCheckResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Strays != 1 || res.OK {
		t.Fatalf("expected one stray: %+v", res)
	}

	writeFile("pkg/routes/x.go")
	gitInWorkspace(t, ws, "add", ".")
	gitInWorkspace(t, ws, "commit", "-q", "-m", "feat: collision")
	out, _, code = h.Run(t, ws, "check", "--base", "main")
	if code == 0 {
		t.Fatalf("a file leased by another task must fail; stdout %s", out)
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Collisions != 1 {
		t.Fatalf("expected one collision with task %d: %+v", other.ID, res)
	}
}

// TestHeartbeat_RefreshesLeases: heartbeat returns the task's leases, fresh.
func TestHeartbeat_RefreshesLeases(t *testing.T) {
	ws, h := provisionWorkspace(t)
	task := createTask(t, h, ws, "long job", "--paths-owned", "pkg/**")
	ref := fmt.Sprintf("%d", task.Index)
	if _, errOut, code := h.Run(t, ws, "claim", ref); code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}
	out, errOut, code := h.Run(t, ws, "heartbeat", ref)
	if code != 0 {
		t.Fatalf("heartbeat exit %d\n%s", code, errOut)
	}
	var leases []*client.TaskPathLease
	if err := json.Unmarshal([]byte(out), &leases); err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].Stale || leases[0].LastActive.IsZero() {
		t.Fatalf("expected one fresh lease: %s", out)
	}
}
