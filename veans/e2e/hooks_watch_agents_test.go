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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/veans/internal/client"
)

// gitWithHooks runs git the way a developer's shell would after
// `veans hooks install`: the veans binary on PATH and the test's HOME, so
// the pre-commit hook finds the bot's credentials. Returns combined output
// and the exit code.
func gitWithHooks(t *testing.T, h *Harness, ws *Workspace, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = ws.Dir
	env := filterEnv(os.Environ(), "VEANS_")
	env = append(env, envSlice(ws.envOverrides)...)
	env = append(env, "PATH="+filepath.Dir(h.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	env = append(env, extraEnv...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), ee.ExitCode()
		}
		t.Fatalf("git %v: %v", args, err)
	}
	return out.String(), 0
}

// TestHooks_PreCommitRefusesCollisions: the installed hook runs the scope
// check on the index — against the bot's claimed task before any commit
// carries a Refs: trailer — and aborts a commit that strays or touches
// another user's lease.
func TestHooks_PreCommitRefusesCollisions(t *testing.T) {
	ws, h := provisionWorkspace(t)
	cfg := loadConfig(t, ws)
	mine := createTask(t, h, ws, "models work", "--paths-owned", "pkg/models/**")
	other := createTask(t, h, ws, "routes work", "--paths-owned", "pkg/routes/**")
	if _, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", mine.Index)); code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}
	if _, err := h.AdminClient.ClaimTask(t.Context(), other.ID, cfg.ViewID, cfg.Buckets.InProgress, cfg.Buckets.Todo); err != nil {
		t.Fatalf("admin claim: %v", err)
	}
	gitInWorkspace(t, ws, "add", "-A")
	gitInWorkspace(t, ws, "commit", "-q", "-m", "chore: veans init")

	out, errOut, code := h.Run(t, ws, "hooks", "install")
	if code != 0 {
		t.Fatalf("hooks install exit %d\n%s", code, errOut)
	}
	var installed map[string]string
	if err := json.Unmarshal([]byte(out), &installed); err != nil {
		t.Fatal(err)
	}
	if installed["action"] != "Wrote" {
		t.Fatalf("expected Wrote: %s", out)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".git", "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	if out, _, code := h.Run(t, ws, "hooks", "install"); code != 0 || !strings.Contains(out, "Already installed") {
		t.Fatalf("second install: exit %d %s", code, out)
	}

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

	writeFile("pkg/routes/x.go")
	gitInWorkspace(t, ws, "add", ".")
	if out, code := gitWithHooks(t, h, ws, nil, "commit", "-q", "-m", "feat: collision"); code == 0 || !strings.Contains(out, "leased_by_other") {
		t.Fatalf("the hook must refuse a file leased by another task; exit %d\n%s", code, out)
	}
	if out, code := gitWithHooks(t, h, ws, nil, "log", "--oneline"); code != 0 || strings.Contains(out, "collision") {
		t.Fatalf("the refused commit must not exist:\n%s", out)
	}

	if out, code := gitWithHooks(t, h, ws, []string{"VEANS_SKIP_CHECK=1"}, "commit", "-q", "-m", "feat: bypassed"); code != 0 {
		t.Fatalf("VEANS_SKIP_CHECK must bypass the hook; exit %d\n%s", code, out)
	}

	writeFile("docs/notes.md")
	gitInWorkspace(t, ws, "add", ".")
	if out, code := gitWithHooks(t, h, ws, nil, "commit", "-q", "-m", "docs: stray"); code == 0 || !strings.Contains(out, "unscoped") {
		t.Fatalf("a file outside the claimed task's paths_owned must be refused; exit %d\n%s", code, out)
	}
	gitInWorkspace(t, ws, "reset", "-q", "docs/notes.md")

	writeFile("pkg/models/new.go")
	gitInWorkspace(t, ws, "add", "pkg/models/new.go")
	if out, code := gitWithHooks(t, h, ws, nil, "commit", "-q", "-m", "feat: inside"); code != 0 {
		t.Fatalf("a file inside the claimed task's scope must commit; exit %d\n%s", code, out)
	}

	if out, _, code := h.Run(t, ws, "hooks", "uninstall"); code != 0 || !strings.Contains(out, "Removed") {
		t.Fatalf("uninstall: exit %d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".git", "hooks", "pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("hook should be gone: %v", err)
	}
}

// TestWatch_OnceEmitsReadyTasks: --once prints a task.ready line for every
// claimable task and nothing for blocked ones.
func TestWatch_OnceEmitsReadyTasks(t *testing.T) {
	ws, h := provisionWorkspace(t)
	ready := createTask(t, h, ws, "free")
	blocked := createTask(t, h, ws, "waits", "--blocked-by", fmt.Sprintf("%d", ready.Index))

	out, errOut, code := h.Run(t, ws, "watch", "--once")
	if code != 0 {
		t.Fatalf("watch --once exit %d\n%s", code, errOut)
	}
	seen := map[int64]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev struct {
			Event string       `json:"event"`
			Task  *client.Task `json:"task"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("not a JSON line: %q: %v", line, err)
		}
		if ev.Task != nil {
			seen[ev.Task.ID] = ev.Event
		}
	}
	if seen[ready.ID] != "task.ready" {
		t.Fatalf("expected task.ready for %d: %s", ready.ID, out)
	}
	if _, ok := seen[blocked.ID]; ok {
		t.Fatalf("a blocked task must not be emitted: %s", out)
	}

	if _, errOut, code := h.Run(t, ws, "watch", "--once", "--interval", "1s"); code == 0 || !strings.Contains(errOut, "VALIDATION_ERROR") {
		t.Fatalf("a sub-5s interval must be refused; exit %d %s", code, errOut)
	}
}

// TestAgents_ListsBotWithOwner: the bot that claimed a task shows up with
// the human who ran init behind it.
func TestAgents_ListsBotWithOwner(t *testing.T) {
	ws, h := provisionWorkspace(t)
	task := createTask(t, h, ws, "mine", "--paths-owned", "pkg/**")
	if _, errOut, code := h.Run(t, ws, "claim", fmt.Sprintf("%d", task.Index)); code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}

	out, errOut, code := h.Run(t, ws, "agents")
	if code != 0 {
		t.Fatalf("agents exit %d\n%s", code, errOut)
	}
	var agents []*client.ProjectAgent
	if err := json.Unmarshal([]byte(out), &agents); err != nil {
		t.Fatal(err)
	}
	var bot *client.ProjectAgent
	for _, a := range agents {
		if a.User != nil && a.User.Username == ws.BotUsername {
			bot = a
		}
	}
	if bot == nil {
		t.Fatalf("bot %s missing from agents: %s", ws.BotUsername, out)
	}
	if bot.Owner == nil || bot.Owner.Username != seedAdminUsername {
		t.Fatalf("bot must carry its owner: %s", out)
	}
	if bot.OpenTasks != 1 || bot.Leases != 1 {
		t.Fatalf("expected 1 open task and 1 lease: %+v", bot)
	}
	if agents[0] != bot {
		t.Fatalf("bots come first: %s", out)
	}
}
