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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"code.vikunja.io/veans/internal/client"
)

var (
	marshalOnce sync.Once
	marshalBin  string
	marshalErr  error
)

func marshalBinary(t *testing.T) string {
	t.Helper()
	marshalOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "marshal-bin-*")
		if err != nil {
			marshalErr = err
			return
		}
		bin := filepath.Join(tmp, "marshal")
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, "./cmd/marshal")
		cmd.Dir = repoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			marshalErr = fmt.Errorf("build marshal: %w (%s)", err, out)
			return
		}
		marshalBin = bin
	})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	return marshalBin
}

// runMarshal runs the marshal binary in the workspace with the test HOME so
// it finds the bot tokens veans init stored. extraEnv adds MARSHAL_* knobs.
func runMarshal(t *testing.T, ws *Workspace, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), marshalBinary(t), args...)
	cmd.Dir = ws.Dir
	cmd.Env = append(filterEnv(os.Environ(), "VEANS_"), envSlice(ws.envOverrides)...)
	cmd.Env = append(cmd.Env, extraEnv...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return so.String(), se.String(), ee.ExitCode()
		}
		t.Fatalf("run marshal %v: %v", args, err)
	}
	return so.String(), se.String(), 0
}

func writeWorkspaceFile(t *testing.T, ws *Workspace, rel, content string) {
	t.Helper()
	p := filepath.Join(ws.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const prdBody = "The storefront shows machinery only to accounts that belong to a network with at least one shared location."

// TestMarshal_EndToEnd walks the whole layer against a live board: init and
// setup, reference resolution and the paste detector, the invariants, the
// worktree plan, the receipt gate on done, the ledger, MCP and Discord.
func TestMarshal_EndToEnd(t *testing.T) {
	ws, h := provisionWorkspace(t)
	cfg := loadConfig(t, ws)

	writeWorkspaceFile(t, ws, "docs/prd.md", "# PRD\n\n**FR-1 — Absence for single-location accounts**\n\n"+prdBody+"\n\n**FR-2 — Something else**\n\nMore text.\n")
	writeWorkspaceFile(t, ws, ".github/CODEOWNERS", "# chokepoint\n/src/lib/contract/   @owner\n")
	gitInWorkspace(t, ws, "add", "-A")
	gitInWorkspace(t, ws, "commit", "-q", "-m", "chore: spec and codeowners")

	// --- init ---
	_, errOut, code := runMarshal(t, ws, nil, "init", "--prd", "docs/prd.md", "--ports", "3100-3101", "--database", "mongodb://127.0.0.1:27101/one")
	if code != 0 {
		t.Fatalf("init exit %d\n%s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".marshal.yml")); err != nil {
		t.Fatal(err)
	}
	if ignore, _ := os.ReadFile(filepath.Join(ws.Dir, ".gitignore")); !strings.Contains(string(ignore), ".marshal/") {
		t.Fatalf(".marshal/ must be ignored: %q", ignore)
	}

	// --- setup: bots, receipt bot, claim bucket ---
	out, errOut, code := runMarshal(t, ws, nil, "setup", "--token", h.AdminToken, "--skip-webhook")
	if code != 0 {
		t.Fatalf("setup exit %d\n%s", code, errOut)
	}
	var setup struct {
		CIBotID        int64  `json:"ci_bot_id"`
		CIToken        string `json:"ci_token"`
		ReceiptBotSet  bool   `json:"receipt_bot_set"`
		ClaimBucketSet bool   `json:"claim_bucket_set"`
	}
	if err := json.Unmarshal([]byte(out), &setup); err != nil {
		t.Fatal(err)
	}
	if setup.CIBotID == 0 || setup.CIToken == "" || !setup.ReceiptBotSet || !setup.ClaimBucketSet {
		t.Fatalf("setup incomplete: %s", out)
	}
	project, err := h.AdminClient.GetProject(t.Context(), cfg.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if project.ReceiptBotID != setup.CIBotID {
		t.Fatalf("receipt bot not set on the project: %+v", project)
	}
	views, err := h.AdminClient.ListProjectViews(t.Context(), cfg.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	claimSet := false
	for _, v := range views {
		if v.ID == cfg.ViewID && v.ClaimBucketID == cfg.Buckets.InProgress {
			claimSet = true
		}
	}
	if !claimSet {
		t.Fatalf("claim bucket not set on view %d", cfg.ViewID)
	}

	// --- tasks: one clean, one that pastes the spec and cites a ghost ---
	clean := createTask(t, h, ws, "E1.1 — Absence checklist. Implements FR-1.", "--paths-owned", "src/lib/contract/hire.ts")
	pasted := createTask(t, h, ws, "E1.2 — Copies the spec", "--description", "<p>"+prdBody+" Also see FR-9.</p>")

	// --- refs ---
	out, errOut, code = runMarshal(t, ws, nil, "refs", "resolve", "FR-1")
	if code != 0 {
		t.Fatalf("refs resolve exit %d\n%s", code, errOut)
	}
	var resolved struct {
		Resolutions []struct {
			Found      bool   `json:"Found"`
			Provenance string `json:"Provenance"`
			Anchor     struct {
				Title string `json:"Title"`
			} `json:"Anchor"`
		} `json:"resolutions"`
	}
	if err := json.Unmarshal([]byte(out), &resolved); err != nil {
		t.Fatal(err)
	}
	if len(resolved.Resolutions) != 1 || !resolved.Resolutions[0].Found || !strings.HasPrefix(resolved.Resolutions[0].Provenance, "docs/prd.md@") {
		t.Fatalf("FR-1 must resolve with provenance: %s", out)
	}
	if _, errOut, code = runMarshal(t, ws, nil, "refs", "resolve", "FR-9"); code == 0 || !strings.Contains(errOut, "NOT_FOUND") {
		t.Fatalf("a ghost reference must be NOT_FOUND; exit %d %s", code, errOut)
	}

	out, errOut, code = runMarshal(t, ws, nil, "refs", "check")
	if code == 0 || !strings.Contains(errOut, "CONFLICT") {
		t.Fatalf("refs check must fail on the broken ref and the paste; exit %d\n%s", code, errOut)
	}
	var report struct {
		Broken []struct {
			TaskID int64  `json:"task_id"`
			Ref    string `json:"ref"`
		} `json:"broken"`
		Pastes []struct {
			TaskID int64 `json:"task_id"`
			Match  struct {
				Words int `json:"Words"`
			} `json:"match"`
		} `json:"pastes"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Broken) != 1 || report.Broken[0].TaskID != pasted.ID || report.Broken[0].Ref != "FR-9" {
		t.Fatalf("expected FR-9 broken on task %d: %s", pasted.ID, out)
	}
	if len(report.Pastes) != 1 || report.Pastes[0].TaskID != pasted.ID || report.Pastes[0].Match.Words < 12 {
		t.Fatalf("expected the pasted paragraph flagged on task %d: %s", pasted.ID, out)
	}

	// --- health: the pasted task claims nothing ---
	out, errOut, code = runMarshal(t, ws, nil, "health")
	if code == 0 || !strings.Contains(errOut, "CONFLICT") {
		t.Fatalf("health must report the unclaimed story; exit %d\n%s", code, errOut)
	}
	var health struct {
		OK       bool `json:"OK"`
		Findings []struct {
			Code    string  `json:"Code"`
			TaskIDs []int64 `json:"TaskIDs"`
		} `json:"Findings"`
		UnblockedRoots []int64 `json:"UnblockedRoots"`
	}
	if err := json.Unmarshal([]byte(out), &health); err != nil {
		t.Fatal(err)
	}
	noClaim := false
	for _, f := range health.Findings {
		if f.Code == "no_claim" && len(f.TaskIDs) == 1 && f.TaskIDs[0] == pasted.ID {
			noClaim = true
		}
	}
	if health.OK || !noClaim || len(health.UnblockedRoots) != 2 {
		t.Fatalf("unexpected health: %s", out)
	}

	// --- chokepoints ---
	out, errOut, code = runMarshal(t, ws, nil, "chokepoints")
	if code != 0 || !strings.Contains(out, "src/lib/contract/**") || !strings.Contains(out, fmt.Sprintf(`"TaskID": %d`, clean.ID)) {
		t.Fatalf("the contract chokepoint must queue the clean task; exit %d\n%s\n%s", code, out, errOut)
	}

	// --- worktree plan with pool allocation ---
	out, errOut, code = runMarshal(t, ws, nil, "worktree", fmt.Sprintf("%d", clean.Index))
	if code != 0 {
		t.Fatalf("worktree exit %d\n%s", code, errOut)
	}
	var plan struct {
		Plan struct {
			Branch   string   `json:"Branch"`
			Commands []string `json:"Commands"`
		} `json:"plan"`
		Allocation struct {
			Database string `json:"database"`
			Port     int    `json:"port"`
		} `json:"allocation"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.Plan.Branch, "e1.1-") || plan.Allocation.Port != 3100 || plan.Allocation.Database != "mongodb://127.0.0.1:27101/one" {
		t.Fatalf("unexpected plan: %s", out)
	}
	if len(plan.Plan.Commands) == 0 || !strings.HasPrefix(plan.Plan.Commands[0], "git worktree add ") {
		t.Fatalf("expected a git worktree command first: %s", out)
	}

	// --- the receipt gate on done ---
	ref := fmt.Sprintf("%d", clean.Index)
	if _, errOut, code := h.Run(t, ws, "claim", ref); code != 0 {
		t.Fatalf("claim exit %d\n%s", code, errOut)
	}
	if _, errOut, code := h.Run(t, ws, "update", ref, "--status", "in-review"); code != 0 {
		t.Fatalf("to review exit %d\n%s", code, errOut)
	}
	if _, errOut, code := h.Run(t, ws, "update", ref, "--status", "completed"); code == 0 || !strings.Contains(errOut, "gate receipt") {
		t.Fatalf("done without a receipt must be refused (4039); exit %d %s", code, errOut)
	}

	ciEnv := []string{"MARSHAL_TOKEN=" + setup.CIToken}
	out, errOut, code = runMarshal(t, ws, ciEnv, "receipt", ref, "--commit", "abcdef1234", "--branch", plan.Plan.Branch,
		"--gate", "typecheck=passed:1200", "--gate", "lint=passed:800", "--gate", "test=passed:30000", "--gate", "build=passed:45000", "--merged", "--merge-sha", "0123abcd")
	if code != 0 {
		t.Fatalf("receipt exit %d\n%s", code, errOut)
	}
	if !strings.Contains(out, `"passed": true`) {
		t.Fatalf("receipt should pass: %s", out)
	}
	// A worker's token is not CI.
	if _, errOut, code := runMarshal(t, ws, nil, "receipt", ref, "--commit", "abcdef1234", "--gate", "test=passed"); code == 0 || !strings.Contains(errOut, "AUTH_ERROR") && !strings.Contains(errOut, "403") {
		t.Fatalf("a worker posting a receipt must be refused; exit %d %s", code, errOut)
	}
	// The bot moved the task to review, so the bot cannot close it even now.
	if _, errOut, code := h.Run(t, ws, "update", ref, "--status", "completed"); code == 0 || !strings.Contains(errOut, "submitted this task for review") {
		t.Fatalf("the submitter must not close (4040); exit %d %s", code, errOut)
	}
	// CI can.
	ci := client.New(h.APIURL, setup.CIToken)
	done := true
	closed, err := ci.UpdateTask(t.Context(), clean.ID, &client.TaskPatch{Done: &done})
	if err != nil {
		t.Fatalf("CI closing after a merged receipt: %v", err)
	}
	if !closed.Done {
		t.Fatalf("task should be done: %+v", closed)
	}
	receipts, err := h.AdminClient.ListTaskReceipts(t.Context(), clean.ID)
	if err != nil || len(receipts) != 1 || !receipts[0].Merged {
		t.Fatalf("expected one merged receipt: %v %+v", err, receipts)
	}

	// --- ledger ---
	out, errOut, code = runMarshal(t, ws, nil, "ledger", "verify")
	if code != 0 || !strings.Contains(out, `"intact": true`) {
		t.Fatalf("ledger verify exit %d\n%s\n%s", code, out, errOut)
	}
	out, _, _ = runMarshal(t, ws, nil, "ledger", "tail", "100")
	if !strings.Contains(out, `"action": "receipt"`) || !strings.Contains(out, `"action": "allocate"`) {
		t.Fatalf("ledger should record the receipt and the allocation: %s", out)
	}

	// --- Discord: replay a claimed event into a fake channel ---
	var got []map[string]any
	var mu sync.Mutex
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discord.Close()
	payload := fmt.Sprintf(`{"event_name":"task.claimed","time":"2026-09-02T12:00:00Z","data":{"task":{"id":%d,"title":"%s","identifier":"%s","leases":[{"pattern":"src/lib/contract/hire.ts"}]},"doer":{"username":"%s"}}}`, clean.ID, clean.Title, clean.Identifier, ws.BotUsername)
	writeWorkspaceFile(t, ws, "payload.json", payload)
	discordEnv := []string{"MARSHAL_DISCORD_WEBHOOK=" + discord.URL}
	if out, errOut, code := runMarshal(t, ws, discordEnv, "notify", "replay", "payload.json"); code != 0 || !strings.Contains(out, `"sent": true`) {
		t.Fatalf("notify replay exit %d\n%s\n%s", code, out, errOut)
	}
	mu.Lock()
	n := len(got)
	first := got[0]
	mu.Unlock()
	if n != 1 || !strings.Contains(fmt.Sprint(first["embeds"]), "Claimed") {
		t.Fatalf("expected one Claimed card: %v", got)
	}

	// --- one watcher pass posts the findings ---
	out, errOut, code = runMarshal(t, ws, discordEnv, "serve", "--once", "--spec-rev", "")
	if code != 0 {
		t.Fatalf("serve --once exit %d\n%s", code, errOut)
	}
	var tick struct {
		Broken   int  `json:"broken"`
		Pastes   int  `json:"pastes"`
		HealthOK bool `json:"health_ok"`
		Notified int  `json:"notified"`
	}
	if err := json.Unmarshal([]byte(out), &tick); err != nil {
		t.Fatal(err)
	}
	if tick.Broken != 1 || tick.Pastes != 1 || tick.HealthOK || tick.Notified < 2 {
		t.Fatalf("unexpected tick: %s", out)
	}
	pastedNow := h.GetTask(t, pasted.ID)
	labels := []string{}
	for _, l := range pastedNow.Labels {
		labels = append(labels, l.Title)
	}
	if !hasLabel(labels, "marshal:paste") || !hasLabel(labels, "marshal:broken-ref") {
		t.Fatalf("the flagged task must carry the labels: %v", labels)
	}
	// A second pass says nothing new.
	out, _, _ = runMarshal(t, ws, discordEnv, "serve", "--once", "--spec-rev", "")
	if err := json.Unmarshal([]byte(out), &tick); err != nil || tick.Notified != 0 {
		t.Fatalf("second pass must be quiet: %s (%v)", out, err)
	}

	// --- MCP handshake and a tool call (on an open task: clean is done now) ---
	probe := createTask(t, h, ws, "E1.3 — MCP probe", "--paths-owned", "src/lib/contract/hire.ts")
	testMCP(t, ws, probe)
}

func hasLabel(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func testMCP(t *testing.T, ws *Workspace, task *client.Task) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, marshalBinary(t), "mcp")
	cmd.Dir = ws.Dir
	cmd.Env = append(filterEnv(os.Environ(), "VEANS_"), envSlice(ws.envOverrides)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	send := func(line string) map[string]any {
		if _, err := io.WriteString(stdin, line+"\n"); err != nil {
			t.Fatal(err)
		}
		resp, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read mcp response: %v (stderr %s)", err, stderr.String())
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(resp), &m); err != nil {
			t.Fatalf("bad mcp json %q: %v", resp, err)
		}
		return m
	}
	init := send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}`)
	if res, ok := init["result"].(map[string]any); !ok || res["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %v", init)
	}
	if _, err := io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	list := send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	names := fmt.Sprint(list["result"])
	for _, want := range []string{"list_startable_tasks", "claim_task", "report_status", "attach_receipt", "release_task", "open_on_path", "resolve_references", "worktree_plan", "board_health", "check_scope"} {
		if !strings.Contains(names, want) {
			t.Fatalf("tool %s missing: %s", want, names)
		}
	}
	call := send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"open_on_path","arguments":{"path":"src/lib/contract/hire.ts"}}}`)
	res, ok := call["result"].(map[string]any)
	if !ok || res["isError"] == true || !strings.Contains(fmt.Sprint(res["content"]), fmt.Sprintf(`"task_id": %d`, task.ID)) {
		t.Fatalf("open_on_path: %v", call)
	}
	_ = stdin.Close()
	_ = cmd.Wait()
}
