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
	"strings"
	"testing"

	"code.vikunja.io/veans/internal/client"
)

func createTask(t *testing.T, h *Harness, ws *Workspace, args ...string) *client.Task {
	t.Helper()
	out, errOut, code := h.Run(t, ws, append([]string{"create"}, args...)...)
	if code != 0 {
		t.Fatalf("create exit %d\n%s", code, errOut)
	}
	var created client.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	return &created
}

func readiness(t *testing.T, h *Harness, ws *Workspace, taskID int64) *client.TaskReadiness {
	t.Helper()
	out, errOut, code := h.Run(t, ws, "ready")
	if code != 0 {
		t.Fatalf("ready exit %d\n%s", code, errOut)
	}
	var rows []*client.TaskReadiness
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Task != nil && r.Task.ID == taskID {
			return r
		}
	}
	t.Fatalf("task %d not in the ready queue: %s", taskID, out)
	return nil
}

// TestScope_LeasesBlockOverlappingClaims is the whole point of scopes: two
// tasks that own overlapping paths cannot both be in progress.
func TestScope_LeasesBlockOverlappingClaims(t *testing.T) {
	ws, h := provisionWorkspace(t)

	owner := createTask(t, h, ws, "owns models", "--paths-owned", "pkg/models/**", "--endpoint", "POST /api/v2/tasks/{id}/claim")
	other := createTask(t, h, ws, "touches tasks.go", "--paths-owned", "pkg/models/tasks.go")
	ownerRef := fmt.Sprintf("%d", owner.Index)
	otherRef := fmt.Sprintf("%d", other.Index)

	out, _, code := h.Run(t, ws, "scope", ownerRef)
	if code != 0 {
		t.Fatalf("scope exit %d\n%s", code, out)
	}
	var sc client.TaskScope
	if err := json.Unmarshal([]byte(out), &sc); err != nil {
		t.Fatal(err)
	}
	if len(sc.PathsOwned) != 1 || sc.PathsOwned[0] != "pkg/models/**" || len(sc.Endpoints) != 1 {
		t.Fatalf("scope not stored from create flags: %+v", sc)
	}

	// Nothing is leased yet, so both are ready.
	if r := readiness(t, h, ws, other.ID); !r.Ready {
		t.Fatalf("other must be ready before any claim: %+v", r)
	}

	if _, errOut, code := h.Run(t, ws, "claim", ownerRef); code != 0 {
		t.Fatalf("claim owner exit %d\n%s", code, errOut)
	}

	out, _, code = h.Run(t, ws, "leases")
	if code != 0 {
		t.Fatalf("leases exit %d\n%s", code, out)
	}
	var leases []*client.TaskPathLease
	if err := json.Unmarshal([]byte(out), &leases); err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].TaskID != owner.ID || leases[0].Pattern != "pkg/models/**" || leases[0].Task == nil {
		t.Fatalf("expected one lease held by the owner with the task embedded, got %s", out)
	}

	r := readiness(t, h, ws, other.ID)
	if r.Ready || len(r.LeaseConflicts) != 1 || r.LeaseConflicts[0].HeldByTaskID != owner.ID {
		t.Fatalf("other must report a lease conflict with the owner: %+v", r)
	}
	if !contains(r.Reasons, client.ReadinessReasonLeaseConflict) {
		t.Fatalf("reasons = %v", r.Reasons)
	}

	// list --ready is the same query reduced to what can be claimed.
	out, _, code = h.Run(t, ws, "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready exit %d\n%s", code, out)
	}
	if strings.Contains(out, fmt.Sprintf(`"id":%d,`, other.ID)) {
		t.Fatalf("list --ready must hide the lease-conflicted task: %s", out)
	}

	if _, errOut, code := h.Run(t, ws, "claim", otherRef); code == 0 || !strings.Contains(errOut, `"CONFLICT"`) {
		t.Fatalf("overlapping claim must fail with CONFLICT; exit %d stderr %s", code, errOut)
	}

	if _, errOut, code := h.Run(t, ws, "release", ownerRef); code != 0 {
		t.Fatalf("release exit %d\n%s", code, errOut)
	}
	if _, errOut, code := h.Run(t, ws, "claim", otherRef); code != 0 {
		t.Fatalf("claim after release exit %d\n%s", code, errOut)
	}

	// The releasing task is still in progress and assigned; only the lease went.
	server := h.GetTask(t, owner.ID)
	cfg := loadConfig(t, ws)
	if server.CurrentBucketID(cfg.ViewID) != cfg.Buckets.InProgress || len(server.Assignees) != 1 {
		t.Fatalf("release must not change status or assignees: %+v", server)
	}
}

// TestScope_WideningAClaimedTaskIsChecked: an agent cannot sidestep leases
// by claiming with a narrow scope and widening it afterwards.
func TestScope_WideningAClaimedTaskIsChecked(t *testing.T) {
	ws, h := provisionWorkspace(t)

	holder := createTask(t, h, ws, "holds routes", "--paths-owned", "pkg/routes/**")
	late := createTask(t, h, ws, "starts narrow", "--paths-owned", "frontend/**")
	for _, ref := range []int64{holder.Index, late.Index} {
		if _, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", ref)); code != 0 {
			t.Fatalf("claim %d exit %d\n%s", ref, code, errOut)
		}
	}

	lateRef := fmt.Sprintf("%d", late.Index)
	_, errOut, code := h.Run(t, ws, "scope", lateRef, "--paths-owned", "frontend/**", "--paths-owned", "pkg/routes/api/v2/tasks.go")
	if code == 0 || !strings.Contains(errOut, `"CONFLICT"`) {
		t.Fatalf("widening onto a leased path must fail with CONFLICT; exit %d stderr %s", code, errOut)
	}

	// Unrelated fields still update in place, and the lists not passed stay.
	out, errOut, code := h.Run(t, ws, "scope", lateRef, "--scope-notes", "<p>no backend</p>")
	if code != 0 {
		t.Fatalf("scope notes exit %d\n%s", code, errOut)
	}
	var sc client.TaskScope
	if err := json.Unmarshal([]byte(out), &sc); err != nil {
		t.Fatal(err)
	}
	if sc.Notes != "<p>no backend</p>" || len(sc.PathsOwned) != 1 || sc.PathsOwned[0] != "frontend/**" {
		t.Fatalf("partial scope update clobbered fields: %+v", sc)
	}

	if _, errOut, code := h.Run(t, ws, "scope", lateRef, "--clear"); code != 0 {
		t.Fatalf("scope --clear exit %d\n%s", code, errOut)
	}
	out, _, _ = h.Run(t, ws, "leases")
	if strings.Contains(out, "frontend/**") {
		t.Fatalf("clearing the scope must drop its leases: %s", out)
	}
}

// TestReady_ReportsBlockedAndAssigned covers the two non-lease reasons the
// server reports, and that finishing the blocker frees the dependent.
func TestReady_ReportsBlockedAndAssigned(t *testing.T) {
	ws, h := provisionWorkspace(t)

	blocker := createTask(t, h, ws, "first")
	dependent := createTask(t, h, ws, "second", "--blocked-by", fmt.Sprintf("%d", blocker.Index))

	r := readiness(t, h, ws, dependent.ID)
	if r.Ready || !contains(r.Reasons, client.ReadinessReasonBlocked) || len(r.BlockedBy) != 1 || r.BlockedBy[0].ID != blocker.ID {
		t.Fatalf("dependent must be blocked by the blocker: %+v", r)
	}

	if _, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", blocker.Index)); code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}
	// A claimed task leaves Todo, so it is no longer in the queue at all;
	// park it back in Todo to see "assigned" reported.
	if _, errOut, code := h.Run(t, ws, "update", fmt.Sprintf("%d", blocker.Index), "-s", "todo"); code != 0 {
		t.Fatalf("update exit %d\n%s", code, errOut)
	}
	if r := readiness(t, h, ws, blocker.ID); r.Ready || !contains(r.Reasons, client.ReadinessReasonAssigned) {
		t.Fatalf("assigned task must not be ready: %+v", r)
	}

	if _, errOut, code := h.Run(t, ws, "update", fmt.Sprintf("%d", blocker.Index), "-s", "completed"); code != 0 {
		t.Fatalf("complete exit %d\n%s", code, errOut)
	}
	if r := readiness(t, h, ws, dependent.ID); !r.Ready {
		t.Fatalf("dependent must be ready once its blocker is done: %+v", r)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
