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

// Package mcptools exposes Marshal to agents over MCP. Every tool calls the
// same board API a human's click calls, under the token the server was
// started with, so refusals and attribution are identical (M1.11, M5.2,
// M5.3). Nothing here reads a credential from a file in the repository.
package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/engine"
	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/marshal/mcp"
	"code.vikunja.io/veans/internal/marshal/refs"
	"code.vikunja.io/veans/internal/output"
)

// New builds the MCP server with every tool registered.
func New(e *engine.Engine, version string) *mcp.Server {
	s := mcp.NewServer("marshal", version)
	t := &tools{e: e}
	s.Register(mcp.Tool{
		Name:        "list_startable_tasks",
		Description: "Tasks in the queue bucket that can be started now: not done, unassigned, no open blocker, no owned path leased by another task. Ordered as the board orders them.",
		InputSchema: mcp.Schema(map[string]any{}),
		Handler:     t.listStartable,
	})
	s.Register(mcp.Tool{
		Name:        "claim_task",
		Description: "Atomically claim a task: assign yourself, move it to In Progress and lease its paths_owned. Refused (with who holds what) when another user has it, a blocker is open, or a path is leased elsewhere.",
		InputSchema: mcp.Schema(map[string]any{"task": mcp.Prop("string", "Task reference: PROJ-12, #12, 12 or id:123")}, "task"),
		Handler:     t.claim,
	})
	s.Register(mcp.Tool{
		Name:        "report_status",
		Description: "Move a task to a status (todo, in-progress, in-review, scrapped) and optionally comment. 'done' is refused for workers: it needs CI's merged receipt and is closed on merge.",
		InputSchema: mcp.Schema(map[string]any{
			"task":    mcp.Prop("string", "Task reference"),
			"status":  mcp.Prop("string", "todo | in-progress | in-review | scrapped"),
			"comment": mcp.Prop("string", "Optional HTML comment to post"),
		}, "task", "status"),
		Handler: t.reportStatus,
	})
	s.Register(mcp.Tool{
		Name:        "attach_receipt",
		Description: "Post a gate receipt as CI (only the project's receipt bot token is accepted). gates is a list of {name, status: passed|failed|skipped, duration_ms}.",
		InputSchema: mcp.Schema(map[string]any{
			"task":                 mcp.Prop("string", "Task reference"),
			"commit_sha":           mcp.Prop("string", "Commit the gates ran on"),
			"branch":               mcp.Prop("string", "Branch name"),
			"gates":                map[string]any{"type": "array", "items": map[string]any{"type": "object"}, "description": "Gate results"},
			"merged":               mcp.Prop("boolean", "The commit is on the merged branch"),
			"ci_run_url":           mcp.Prop("string", "Link to the run"),
			"docs_api_required":    mcp.Prop("boolean", "The diff touched the contract"),
			"docs_api_regenerated": mcp.Prop("boolean", "The API docs were regenerated"),
		}, "task", "commit_sha", "gates"),
		Handler: t.attachReceipt,
	})
	s.Register(mcp.Tool{
		Name:        "release_task",
		Description: "Release a task's path leases without changing its status (when you stop working on it).",
		InputSchema: mcp.Schema(map[string]any{"task": mcp.Prop("string", "Task reference")}, "task"),
		Handler:     t.release,
	})
	s.Register(mcp.Tool{
		Name:        "open_on_path",
		Description: "Is anything open on this path? Lists open tasks whose declared scope or live lease covers it, with assignee and branch.",
		InputSchema: mcp.Schema(map[string]any{"path": mcp.Prop("string", "Repository-relative path or pattern")}, "path"),
		Handler:     t.openOnPath,
	})
	s.Register(mcp.Tool{
		Name:        "resolve_references",
		Description: "Resolve FR-/AD-/NFR-/D- references in a task (or in given text) to their spec text with provenance, and report verbatim spec pastes in the task.",
		InputSchema: mcp.Schema(map[string]any{
			"task": mcp.Prop("string", "Task reference (either task or text)"),
			"text": mcp.Prop("string", "Free text containing references"),
		}),
		Handler: t.resolveRefs,
	})
	s.Register(mcp.Tool{
		Name:        "check_scope",
		Description: "Judge changed files against your tasks' scopes and everyone else's leases before committing or opening a PR.",
		InputSchema: mcp.Schema(map[string]any{
			"files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Changed files, repo-relative"},
			"tasks": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Task references the change implements"},
		}, "files", "tasks"),
		Handler: t.checkScope,
	})
	s.Register(mcp.Tool{
		Name:        "worktree_plan",
		Description: "The exact git worktree commands for a story, with a database and port allocated to you from the pool.",
		InputSchema: mcp.Schema(map[string]any{"task": mcp.Prop("string", "Task reference")}, "task"),
		Handler:     t.worktreePlan,
	})
	s.Register(mcp.Tool{
		Name:        "board_health",
		Description: "The graph invariants right now: every story claims files, no unordered overlap, no blocked cycle, and the unblocked roots.",
		InputSchema: mcp.Schema(map[string]any{}),
		Handler:     t.health,
	})
	return s
}

type tools struct {
	e *engine.Engine
}

func (t *tools) resolve(ctx context.Context, ref string) (*client.Task, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, output.New(output.CodeValidation, "task is required")
	}
	if strings.HasPrefix(ref, "id:") {
		id, err := strconv.ParseInt(ref[3:], 10, 64)
		if err != nil {
			return nil, output.Wrap(output.CodeValidation, err, "invalid task id %q", ref)
		}
		return t.e.Board.Client.GetTask(ctx, id)
	}
	r := strings.TrimPrefix(ref, "#")
	if i := strings.LastIndex(r, "-"); i >= 0 {
		r = r[i+1:]
	}
	index, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		return nil, output.Wrap(output.CodeValidation, err, "invalid task reference %q", ref)
	}
	var byIndex client.Task
	if err := t.e.Board.Client.Do(ctx, "GET", fmt.Sprintf("/projects/%d/tasks/by-index/%d", t.e.Board.Cfg.ProjectID, index), nil, nil, &byIndex); err != nil {
		return nil, err
	}
	return t.e.Board.Client.GetTask(ctx, byIndex.ID)
}

func result(v any) (mcp.Result, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Result{Text: string(b)}, nil
}

// refusal renders a board refusal as a tool error result so the agent sees
// the same envelope veans prints, including who holds what.
func refusal(err error) (mcp.Result, error) {
	var oe *output.Error
	if errors.As(err, &oe) {
		if b, mErr := json.MarshalIndent(oe, "", "  "); mErr == nil {
			return mcp.Result{Text: string(b), IsError: true}, nil
		}
	}
	return mcp.Result{Text: err.Error(), IsError: true}, nil
}

func (t *tools) listStartable(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
	rows, err := t.e.Board.Client.ProjectReadiness(ctx, t.e.Board.Cfg.ProjectID, t.e.Board.Cfg.ViewID, 0)
	if err != nil {
		return refusal(err)
	}
	type startable struct {
		ID         int64    `json:"id"`
		Identifier string   `json:"identifier"`
		Title      string   `json:"title"`
		Paths      []string `json:"paths_owned"`
		Rank       int      `json:"rank"`
	}
	out := []startable{}
	for _, r := range rows {
		if !r.Ready || r.Task == nil {
			continue
		}
		s := startable{ID: r.Task.ID, Identifier: r.Task.Identifier, Title: r.Task.Title, Rank: len(out) + 1, Paths: []string{}}
		if r.Task.Scope != nil {
			s.Paths = r.Task.Scope.PathsOwned
		}
		out = append(out, s)
	}
	return result(out)
}

func (t *tools) claim(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	task, err := t.resolve(ctx, in.Task)
	if err != nil {
		return refusal(err)
	}
	cfg := t.e.Board.Cfg
	claimed, err := t.e.Board.Client.ClaimTask(ctx, task.ID, cfg.ViewID, cfg.Buckets.InProgress, cfg.Buckets.Todo)
	if err != nil {
		t.e.Log(ledger.Entry{Action: "acquire", TaskID: task.ID, Subject: task.Identifier, Outcome: "refused", Reason: err.Error()})
		return refusal(err)
	}
	t.e.Log(ledger.Entry{Action: "acquire", TaskID: task.ID, Subject: task.Identifier, Outcome: "ok"})
	return result(claimed)
}

func (t *tools) reportStatus(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task    string `json:"task"`
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	task, err := t.resolve(ctx, in.Task)
	if err != nil {
		return refusal(err)
	}
	cfg := t.e.Board.Cfg
	var bucket int64
	switch strings.ToLower(strings.TrimSpace(in.Status)) {
	case "todo":
		bucket = cfg.Buckets.Todo
	case "in-progress", "in_progress", "inprogress":
		bucket = cfg.Buckets.InProgress
	case "in-review", "in_review", "inreview", "review":
		bucket = cfg.Buckets.InReview
	case "scrapped":
		bucket = cfg.Buckets.Scrapped
	case "done":
		return refusal(output.New(output.CodeValidation, "done is not a worker's call: CI posts the receipt and the merge closes the task"))
	default:
		return refusal(output.New(output.CodeValidation, "unknown status %q", in.Status))
	}
	if bucket == 0 {
		return refusal(output.New(output.CodeNotConfigured, "no bucket configured for %s in .veans.yml", in.Status))
	}
	if in.Comment != "" {
		if _, err := t.e.Board.Client.AddTaskComment(ctx, task.ID, in.Comment); err != nil {
			return refusal(err)
		}
	}
	if err := t.e.Board.Client.MoveTaskToBucket(ctx, cfg.ProjectID, cfg.ViewID, bucket, task.ID); err != nil {
		t.e.Log(ledger.Entry{Action: "status", TaskID: task.ID, Subject: in.Status, Outcome: "refused", Reason: err.Error()})
		return refusal(err)
	}
	t.e.Log(ledger.Entry{Action: "status", TaskID: task.ID, Subject: in.Status, Outcome: "ok"})
	updated, err := t.e.Board.Client.GetTask(ctx, task.ID)
	if err != nil {
		return refusal(err)
	}
	return result(updated)
}

func (t *tools) attachReceipt(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task               string              `json:"task"`
		CommitSHA          string              `json:"commit_sha"`
		Branch             string              `json:"branch"`
		Gates              []client.GateResult `json:"gates"`
		Merged             bool                `json:"merged"`
		CIRunURL           string              `json:"ci_run_url"`
		DocsAPIRequired    bool                `json:"docs_api_required"`
		DocsAPIRegenerated bool                `json:"docs_api_regenerated"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	task, err := t.resolve(ctx, in.Task)
	if err != nil {
		return refusal(err)
	}
	r, err := t.e.Board.Client.PostTaskReceipt(ctx, task.ID, &client.TaskReceipt{
		CommitSHA: in.CommitSHA, Branch: in.Branch, Gates: in.Gates, Merged: in.Merged, CIRunURL: in.CIRunURL,
		DocsAPIRequired: in.DocsAPIRequired, DocsAPIRegenerated: in.DocsAPIRegenerated,
	})
	if err != nil {
		t.e.Log(ledger.Entry{Action: "receipt", TaskID: task.ID, Subject: in.CommitSHA, Outcome: "refused", Reason: err.Error()})
		return refusal(err)
	}
	t.e.Log(ledger.Entry{Action: "receipt", TaskID: task.ID, Subject: in.CommitSHA, Outcome: map[bool]string{true: "passed", false: "failed"}[r.Passed]})
	return result(r)
}

func (t *tools) release(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	task, err := t.resolve(ctx, in.Task)
	if err != nil {
		return refusal(err)
	}
	if err := t.e.Board.Client.ReleaseTaskLeases(ctx, task.ID); err != nil {
		t.e.Log(ledger.Entry{Action: "release", TaskID: task.ID, Subject: task.Identifier, Outcome: "refused", Reason: err.Error()})
		return refusal(err)
	}
	t.e.Log(ledger.Entry{Action: "release", TaskID: task.ID, Subject: task.Identifier, Outcome: "ok"})
	return result(map[string]any{"task_id": task.ID, "released": true})
}

func (t *tools) openOnPath(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	snap, err := t.e.Board.Snapshot(ctx)
	if err != nil {
		return refusal(err)
	}
	holders, err := t.e.OpenOnPath(snap, in.Path)
	if err != nil {
		return refusal(err)
	}
	return result(map[string]any{"path": in.Path, "holders": holders})
}

func (t *tools) resolveRefs(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task string `json:"task"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	if in.Task != "" {
		task, err := t.resolve(ctx, in.Task)
		if err != nil {
			return refusal(err)
		}
		res, err := t.e.ResolveTask(ctx, task)
		if err != nil {
			return refusal(err)
		}
		return result(res)
	}
	if in.Text == "" {
		return refusal(output.New(output.CodeValidation, "pass task or text"))
	}
	ix, _, err := t.e.Index(ctx)
	if err != nil {
		return refusal(err)
	}
	return result(map[string]any{"rev": ix.Rev, "resolutions": ix.Resolve(refs.Extract(in.Text, t.e.Prefixes()))})
}

func (t *tools) checkScope(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Files []string `json:"files"`
		Tasks []string `json:"tasks"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	ids := []int64{}
	for _, ref := range in.Tasks {
		task, err := t.resolve(ctx, ref)
		if err != nil {
			return refusal(err)
		}
		ids = append(ids, task.ID)
	}
	res, err := t.e.Board.Client.CheckScope(ctx, t.e.Board.Cfg.ProjectID, &client.ScopeCheckRequest{TaskIDs: ids, Files: in.Files, Repository: t.e.Board.Cfg.Repository})
	if err != nil {
		return refusal(err)
	}
	out, err := result(res)
	if err != nil {
		return out, err
	}
	out.IsError = !res.OK
	return out, nil
}

func (t *tools) worktreePlan(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
	var in struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return refusal(err)
	}
	task, err := t.resolve(ctx, in.Task)
	if err != nil {
		return refusal(err)
	}
	plan, err := t.e.PlanWorktree(task, t.e.Board.Identity)
	if err != nil {
		return refusal(err)
	}
	return result(plan)
}

func (t *tools) health(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
	snap, err := t.e.Board.Snapshot(ctx)
	if err != nil {
		return refusal(err)
	}
	return result(t.e.Health(snap))
}
