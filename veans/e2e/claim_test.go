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

// TestClaim_AssignsBotMovesToInProgressTagsBranch exercises the full claim
// flow: assignment, bucket transition, and branch label application.
func TestClaim_AssignsBotMovesToInProgressTagsBranch(t *testing.T) {
	ws, h := provisionWorkspace(t)

	out, _, code := h.Run(t, ws, "create", "claim me")
	if code != 0 {
		t.Fatalf("create exit %d\n%s", code, out)
	}
	var created client.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("%d", created.Index)

	// Switch the workspace's git branch so claim has something to label with.
	gitInWorkspace(t, ws, "checkout", "-q", "-b", "feat-claim-test")

	_, errOut, code := h.Run(t, ws, "claim", id)
	if code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}

	server := h.GetTask(t, created.ID)

	// Verify bucket transition by reading the workspace's .veans.yml — the
	// bot's expected In Progress bucket is stored there.
	cfg := loadConfig(t, ws)
	bucket := server.CurrentBucketID(cfg.ViewID)
	if bucket != cfg.Buckets.InProgress {
		t.Fatalf("task not in In Progress bucket: got %d, want %d", bucket, cfg.Buckets.InProgress)
	}

	// Bot assigned.
	assigned := false
	for _, a := range server.Assignees {
		if a != nil && a.ID == cfg.Bot.UserID {
			assigned = true
			break
		}
	}
	if !assigned {
		t.Fatalf("bot %d not in assignees: %+v", cfg.Bot.UserID, server.Assignees)
	}

	// Branch label attached.
	branchLabel := "veans:branch:feat-claim-test"
	if !taskHasLabelTitle(server, branchLabel) {
		t.Fatalf("expected label %q on task; got %+v", branchLabel, server.Labels)
	}
}

// TestClaim_LosesRaceToAnotherUser proves the claim is exclusive: once a
// different user holds the task, the bot's claim is refused with CONFLICT and
// the task is left exactly as the winner put it.
func TestClaim_LosesRaceToAnotherUser(t *testing.T) {
	ws, h := provisionWorkspace(t)
	cfg := loadConfig(t, ws)

	out, _, code := h.Run(t, ws, "create", "contested task")
	if code != 0 {
		t.Fatalf("create exit %d\n%s", code, out)
	}
	var created client.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}

	// The admin (project owner, a different user from the bot) claims first,
	// straight against the API.
	if _, err := h.AdminClient.ClaimTask(t.Context(), created.ID, cfg.ViewID, cfg.Buckets.InProgress, cfg.Buckets.Todo); err != nil {
		t.Fatalf("admin claim: %v", err)
	}

	_, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", created.Index))
	if code == 0 {
		t.Fatalf("bot claim must fail once another user holds the task; stderr: %s", errOut)
	}
	if !strings.Contains(errOut, `"CONFLICT"`) {
		t.Fatalf("expected CONFLICT error code, got: %s", errOut)
	}

	server := h.GetTask(t, created.ID)
	for _, a := range server.Assignees {
		if a != nil && a.ID == cfg.Bot.UserID {
			t.Fatalf("losing bot must not be added as a second assignee: %+v", server.Assignees)
		}
	}
}

// TestClaim_RepeatIsIdempotent: an agent claims on every start of work, so a
// second claim of a task it already holds must succeed and change nothing.
func TestClaim_RepeatIsIdempotent(t *testing.T) {
	ws, h := provisionWorkspace(t)
	cfg := loadConfig(t, ws)

	out, _, code := h.Run(t, ws, "create", "claim twice")
	if code != 0 {
		t.Fatalf("create exit %d\n%s", code, out)
	}
	var created client.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("%d", created.Index)

	for i := 0; i < 2; i++ {
		if _, errOut, code := h.Run(t, ws, "claim", id); code != 0 {
			t.Fatalf("claim #%d exit %d\n%s", i+1, code, errOut)
		}
	}

	server := h.GetTask(t, created.ID)
	if len(server.Assignees) != 1 || server.Assignees[0].ID != cfg.Bot.UserID {
		t.Fatalf("expected exactly the bot assigned, got %+v", server.Assignees)
	}
	if got := server.CurrentBucketID(cfg.ViewID); got != cfg.Buckets.InProgress {
		t.Fatalf("task not in In Progress: got %d, want %d", got, cfg.Buckets.InProgress)
	}
}

// TestClaim_RefusesTaskNotInTodo: `list --ready` can be stale; a task someone
// already moved on is refused unless --force is passed.
func TestClaim_RefusesTaskNotInTodo(t *testing.T) {
	ws, h := provisionWorkspace(t)

	out, _, code := h.Run(t, ws, "create", "already in review", "-s", "in-review")
	if code != 0 {
		t.Fatalf("create exit %d\n%s", code, out)
	}
	var created client.Task
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("%d", created.Index)

	if _, errOut, code := h.Run(t, ws, "claim", id); code == 0 || !strings.Contains(errOut, `"CONFLICT"`) {
		t.Fatalf("claim of a non-Todo task must fail with CONFLICT; exit %d stderr %s", code, errOut)
	}
	if _, errOut, code := h.Run(t, ws, "claim", "--force", id); code != 0 {
		t.Fatalf("claim --force exit %d\n%s", code, errOut)
	}
}
