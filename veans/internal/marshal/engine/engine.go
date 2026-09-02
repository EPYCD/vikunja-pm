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

// Package engine is Marshal's brain: every check the CLI, the HTTP service
// and the MCP tools expose is a method here, built on the board snapshot,
// the repository and the checkers.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/board"
	"code.vikunja.io/veans/internal/marshal/codeowners"
	"code.vikunja.io/veans/internal/marshal/config"
	"code.vikunja.io/veans/internal/marshal/discord"
	"code.vikunja.io/veans/internal/marshal/invariants"
	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/marshal/notify"
	"code.vikunja.io/veans/internal/marshal/pathpattern"
	"code.vikunja.io/veans/internal/marshal/pool"
	"code.vikunja.io/veans/internal/marshal/refs"
	"code.vikunja.io/veans/internal/marshal/worktree"
	"code.vikunja.io/veans/internal/output"
)

// Engine holds the configuration, the board handle and the state files.
type Engine struct {
	Cfg     *config.Config
	Board   *board.Board
	Ledger  *ledger.Ledger
	Pool    *pool.Store
	Discord *discord.Notifier
	Format  notify.Formatter
	// RepoRoot is the checkout Marshal reads specs and git history from.
	RepoRoot string
	// SpecRev is the revision references resolve against; "" reads the
	// working tree. serve sets it to origin/<default> after fetching.
	SpecRev string

	flags *flagStore

	mu       sync.Mutex
	index    *refs.Index
	corpus   *refs.SpecCorpus
	health   *invariants.Report
	healthAt time.Time
}

// Load finds .marshal.yml from dir upward, opens the board and the state.
func Load(dir string) (*Engine, error) {
	path, err := config.Find(dir)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, output.Wrap(output.CodeNotConfigured, err, "no .marshal.yml found — run `marshal init` in the repository")
		}
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	b, err := board.Open(cfg.Dir())
	if err != nil {
		return nil, err
	}
	l, err := ledger.Open(cfg.LedgerPath())
	if err != nil {
		return nil, err
	}
	flags, err := openFlags(filepath.Join(cfg.StateDir, "flags.json"))
	if err != nil {
		return nil, err
	}
	e := &Engine{
		Cfg:      cfg,
		Board:    b,
		Ledger:   l,
		Pool:     pool.Open(cfg.PoolPath()),
		Discord:  discord.New(cfg.Discord.WebhookURL, cfg.Discord.Username, cfg.Discord.AvatarURL),
		RepoRoot: cfg.Dir(),
		flags:    flags,
	}
	e.Format = notify.Formatter{TaskURL: b.TaskURL}
	return e, nil
}

// Prefixes lists the configured reference prefixes.
func (e *Engine) Prefixes() []string {
	out := make([]string, 0, len(e.Cfg.References))
	for _, s := range e.Cfg.References {
		out = append(out, s.Prefix)
	}
	return out
}

func (e *Engine) reader() refs.Reader {
	if e.SpecRev == "" {
		return refs.WorktreeReader(e.RepoRoot)
	}
	return refs.GitReader{Root: e.RepoRoot, Rev: e.SpecRev}
}

// Index builds (and caches) the anchor index at the current spec revision.
func (e *Engine) Index(ctx context.Context) (*refs.Index, []string, error) {
	ix, warnings, err := refs.Build(ctx, e.reader(), e.Cfg.References)
	if err != nil {
		return nil, nil, err
	}
	e.mu.Lock()
	e.index = ix
	e.mu.Unlock()
	return ix, warnings, nil
}

// Corpus builds (and caches) the paste-detection corpus.
func (e *Engine) Corpus(ctx context.Context) (*refs.SpecCorpus, error) {
	e.mu.Lock()
	c := e.corpus
	e.mu.Unlock()
	if c != nil {
		return c, nil
	}
	c, err := refs.NewSpecCorpus(ctx, e.reader(), e.Cfg.SpecFiles, e.Cfg.PasteMinWords)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.corpus = c
	e.mu.Unlock()
	return c, nil
}

// InvalidateCorpus drops the cached corpus, e.g. after the spec revision moved.
func (e *Engine) InvalidateCorpus() {
	e.mu.Lock()
	e.corpus = nil
	e.mu.Unlock()
}

// TaskReferences is what a task's references render as.
type TaskReferences struct {
	TaskID      int64             `json:"task_id"`
	Identifier  string            `json:"identifier"`
	Resolutions []refs.Resolution `json:"resolutions"`
	Pastes      []refs.PasteMatch `json:"pastes"`
	Broken      int               `json:"broken"`
}

// TaskText is the free text Marshal scans on a task.
func TaskText(t *client.Task) string {
	return t.Title + "\n" + t.Description
}

// ResolveTask resolves the references in a task's title and description at
// read time and runs the paste detector over the same text.
func (e *Engine) ResolveTask(ctx context.Context, t *client.Task) (*TaskReferences, error) {
	ix, _, err := e.Index(ctx)
	if err != nil {
		return nil, err
	}
	found := refs.Extract(TaskText(t), e.Prefixes())
	out := &TaskReferences{TaskID: t.ID, Identifier: t.Identifier, Resolutions: ix.Resolve(found), Pastes: []refs.PasteMatch{}}
	for _, r := range out.Resolutions {
		if !r.Found {
			out.Broken++
		}
	}
	if corpus, err := e.Corpus(ctx); err == nil {
		out.Pastes = corpus.FindPastes(TaskText(t))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return out, nil
}

// ReferenceReport is the board-level view of M3.3 and M3.4.
type ReferenceReport struct {
	Rev        string            `json:"rev"`
	Tasks      int               `json:"tasks"`
	Referenced int               `json:"referenced"`
	Broken     []BrokenReference `json:"broken"`
	Pastes     []TaskPaste       `json:"pastes"`
	Unlinked   []TaskRef         `json:"unlinked"`
}

// TaskRef names a task in a report.
type TaskRef struct {
	TaskID     int64  `json:"task_id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
}

// BrokenReference is a reference nothing resolves any more.
type BrokenReference struct {
	TaskRef
	Ref string `json:"ref"`
}

// TaskPaste is a verbatim spec run found on a task.
type TaskPaste struct {
	TaskRef
	Match refs.PasteMatch `json:"match"`
}

// References checks every open task's references and text.
func (e *Engine) References(ctx context.Context, snap *board.Snapshot) (*ReferenceReport, error) {
	ix, _, err := e.Index(ctx)
	if err != nil {
		return nil, err
	}
	corpus, err := e.Corpus(ctx)
	if err != nil {
		return nil, err
	}
	rep := &ReferenceReport{Rev: ix.Rev, Tasks: len(snap.Tasks), Broken: []BrokenReference{}, Pastes: []TaskPaste{}, Unlinked: []TaskRef{}}
	prefixes := e.Prefixes()
	for _, t := range snap.Tasks {
		ref := TaskRef{TaskID: t.ID, Identifier: t.Identifier, Title: t.Title}
		found := refs.Extract(TaskText(t), prefixes)
		if len(found) == 0 {
			if len(t.RelatedTasks["subtask"]) == 0 {
				rep.Unlinked = append(rep.Unlinked, ref)
			}
		} else {
			rep.Referenced++
		}
		for _, r := range ix.Resolve(found) {
			if !r.Found {
				rep.Broken = append(rep.Broken, BrokenReference{TaskRef: ref, Ref: r.Ref.ID})
			}
		}
		for _, m := range corpus.FindPastes(TaskText(t)) {
			rep.Pastes = append(rep.Pastes, TaskPaste{TaskRef: ref, Match: m})
		}
	}
	return rep, nil
}

// HealthReport is the invariants report with the snapshot's numbers.
type HealthReport struct {
	invariants.Report
	OpenTasks int       `json:"open_tasks"`
	Leases    int       `json:"leases"`
	CheckedAt time.Time `json:"checked_at"`
}

// Health runs the continuous invariants over a snapshot.
func (e *Engine) Health(snap *board.Snapshot) *HealthReport {
	tasks, leases := snap.InvariantTasks()
	rep := invariants.Check(tasks, leases)
	h := &HealthReport{Report: rep, OpenTasks: len(snap.Tasks), Leases: len(snap.Leases), CheckedAt: time.Now().UTC()}
	e.mu.Lock()
	e.health = &rep
	e.healthAt = h.CheckedAt
	e.mu.Unlock()
	return h
}

// ChokepointReport is the CODEOWNERS queue.
type ChokepointReport struct {
	Source     string             `json:"source"`
	Outside    []string           `json:"outside_app_root"`
	Queues     []codeowners.Queue `json:"queues"`
	Unreadable string             `json:"unreadable,omitempty"`
}

// Chokepoints reads CODEOWNERS and queues every claim on each chokepoint.
func (e *Engine) Chokepoints(snap *board.Snapshot) (*ChokepointReport, error) {
	path := filepath.Join(e.RepoRoot, e.Cfg.Codeowners)
	b, err := os.ReadFile(path) //nolint:gosec // repository file named in the user's config
	if err != nil {
		// An absent CODEOWNERS is a finding for the report, not a failure of it.
		return &ChokepointReport{Source: e.Cfg.Codeowners, Unreadable: err.Error(), Queues: []codeowners.Queue{}}, nil //nolint:nilerr
	}
	f, err := codeowners.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", e.Cfg.Codeowners, err)
	}
	inside, outside := f.Chokepoints(e.Cfg.AppRoot)
	claims := e.claims(snap)
	return &ChokepointReport{Source: e.Cfg.Codeowners, Outside: outside, Queues: codeowners.Queues(inside, claims)}, nil
}

func (e *Engine) claims(snap *board.Snapshot) []codeowners.Claim {
	active := map[int64]map[string]time.Time{}
	for _, l := range snap.Leases {
		if active[l.TaskID] == nil {
			active[l.TaskID] = map[string]time.Time{}
		}
		active[l.TaskID][l.Pattern] = l.Created
	}
	claims := []codeowners.Claim{}
	for _, t := range snap.Tasks {
		assignee := ""
		if len(t.Assignees) > 0 {
			assignee = board.DisplayName(t.Assignees[0])
		}
		blocked := []int64{}
		for _, r := range t.RelatedTasks["blocked"] {
			if r != nil {
				blocked = append(blocked, r.ID)
			}
		}
		patterns := map[string]bool{}
		if t.Scope != nil {
			for _, p := range t.Scope.PathsOwned {
				patterns[p] = true
			}
		}
		for p := range active[t.ID] {
			patterns[p] = true
		}
		for p := range patterns {
			since := t.Created
			isActive := false
			if at, ok := active[t.ID][p]; ok {
				isActive = true
				since = at
			}
			claims = append(claims, codeowners.Claim{
				TaskID: t.ID, Identifier: t.Identifier, Title: t.Title, Assignee: assignee,
				Pattern: p, Since: since, Active: isActive, BlockedBy: blocked,
			})
		}
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].TaskID != claims[j].TaskID {
			return claims[i].TaskID < claims[j].TaskID
		}
		return claims[i].Pattern < claims[j].Pattern
	})
	return claims
}

// OpenOnPath answers "is anything open on this path?": declared scopes and
// live leases overlapping it.
type PathHolder struct {
	TaskRef
	Pattern  string `json:"pattern"`
	Leased   bool   `json:"leased"`
	Assignee string `json:"assignee,omitempty"`
	Branch   string `json:"branch,omitempty"`
}

// OpenOnPath lists the open tasks whose scope or lease covers path.
func (e *Engine) OpenOnPath(snap *board.Snapshot, path string) ([]PathHolder, error) {
	norm, err := pathpattern.Normalize(path)
	if err != nil {
		return nil, output.Wrap(output.CodeValidation, err, "invalid path %q", path)
	}
	leased := map[int64]map[string]bool{}
	for _, l := range snap.Leases {
		if leased[l.TaskID] == nil {
			leased[l.TaskID] = map[string]bool{}
		}
		leased[l.TaskID][l.Pattern] = true
	}
	out := []PathHolder{}
	for _, t := range snap.Tasks {
		if t.Scope == nil {
			continue
		}
		for _, p := range t.Scope.PathsOwned {
			if !pathpattern.Overlap(p, norm) {
				continue
			}
			h := PathHolder{TaskRef: TaskRef{TaskID: t.ID, Identifier: t.Identifier, Title: t.Title}, Pattern: p, Leased: leased[t.ID][p], Branch: board.BranchOf(t)}
			if len(t.Assignees) > 0 {
				h.Assignee = board.DisplayName(t.Assignees[0])
			}
			out = append(out, h)
		}
	}
	return out, nil
}

// BranchCheck is the reconciliation of one branch against its task's claims.
type BranchCheck struct {
	TaskRef
	Branch       string                   `json:"branch"`
	LastActivity time.Time                `json:"last_activity"`
	Stale        bool                     `json:"stale"`
	Changed      int                      `json:"changed_files"`
	Result       *client.ScopeCheckResult `json:"result,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

// Reconcile validates every claimed task's branch against the files it
// actually touched (M1.8) and flags branches with no recent commits (M1.9).
// base is the integration branch to diff against; branch limits the run to
// one branch when set.
func (e *Engine) Reconcile(ctx context.Context, snap *board.Snapshot, base, branch string) ([]BranchCheck, error) {
	out := []BranchCheck{}
	for _, t := range snap.Tasks {
		br := board.BranchOf(t)
		if br == "" || (branch != "" && br != branch) {
			continue
		}
		bc := BranchCheck{TaskRef: TaskRef{TaskID: t.ID, Identifier: t.Identifier, Title: t.Title}, Branch: br}
		last, err := worktree.LastActivity(ctx, e.RepoRoot, br)
		if err != nil {
			bc.Error = err.Error()
			out = append(out, bc)
			continue
		}
		bc.LastActivity = last
		bc.Stale = !last.IsZero() && time.Since(last) > e.Cfg.StaleAfter
		files, err := worktree.ChangedFiles(ctx, e.RepoRoot, base, br)
		if err != nil {
			bc.Error = err.Error()
			out = append(out, bc)
			continue
		}
		bc.Changed = len(files)
		if len(files) > 0 {
			res, err := e.Board.Client.CheckScope(ctx, e.Board.Cfg.ProjectID, &client.ScopeCheckRequest{
				TaskIDs: []int64{t.ID}, Files: e.repoRelative(files), Repository: e.Board.Cfg.Repository,
			})
			if err != nil {
				bc.Error = err.Error()
			} else {
				bc.Result = res
			}
		}
		out = append(out, bc)
	}
	return out, nil
}

// repoRelative strips the app root so git paths compare with board paths
// declared relative to the app directory.
func (e *Engine) repoRelative(files []string) []string {
	if e.Cfg.AppRoot == "" {
		return files
	}
	prefix := strings.Trim(e.Cfg.AppRoot, "/") + "/"
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, strings.TrimPrefix(f, prefix))
	}
	return out
}

// WorktreePlan is M4: the commands for a story plus its allocated resources.
type WorktreePlan struct {
	TaskRef
	Plan       worktree.Plan   `json:"plan"`
	Allocation pool.Allocation `json:"allocation"`
}

// PlanWorktree derives the branch and directory from the story, allocates a
// database and port to the worker and records the checkout.
func (e *Engine) PlanWorktree(t *client.Task, worker string) (*WorktreePlan, error) {
	story := worktree.StoryID(t.Title)
	if story == "" {
		story = worktree.StoryID(t.Identifier)
	}
	if story == "" {
		story = strings.ToLower(strings.ReplaceAll(t.Identifier, "-", ""))
		if story == "" {
			story = fmt.Sprintf("t%d", t.ID)
		}
	}
	dry, err := worktree.Build(e.Cfg.Worktree, e.RepoRoot, story, t.Title, t.Identifier, t.ID, "", 0)
	if err != nil {
		return nil, err
	}
	alloc := pool.Allocation{Worker: worker, TaskID: t.ID, Story: story, Branch: dry.Branch, Checkout: dry.Dir, Since: time.Now().UTC()}
	if len(e.Cfg.Pool.Databases) > 0 || len(e.Cfg.Pool.Ports) > 0 {
		alloc, err = e.Pool.Allocate(e.Cfg.Pool, alloc)
		if err != nil {
			e.log(ledger.Entry{Action: "allocate", Actor: worker, TaskID: t.ID, Subject: dry.Dir, Outcome: "refused", Reason: err.Error()})
			return nil, output.Wrap(output.CodeConflict, err, "%v", err)
		}
	}
	plan, err := worktree.Build(e.Cfg.Worktree, e.RepoRoot, story, t.Title, t.Identifier, t.ID, alloc.Database, alloc.Port)
	if err != nil {
		return nil, err
	}
	e.log(ledger.Entry{Action: "allocate", Actor: worker, TaskID: t.ID, Subject: plan.Dir, Outcome: "ok", Metadata: map[string]any{"branch": plan.Branch, "database": alloc.Database, "port": alloc.Port}})
	return &WorktreePlan{TaskRef: TaskRef{TaskID: t.ID, Identifier: t.Identifier, Title: t.Title}, Plan: plan, Allocation: alloc}, nil
}

// Workers is M4.3: who is on which checkout, branch and database, live.
type Workers struct {
	Agents      []*client.ProjectAgent `json:"agents"`
	Allocations []pool.Allocation      `json:"allocations"`
	Worktrees   []worktree.Worktree    `json:"worktrees"`
	Conflicts   []pool.Conflict        `json:"conflicts"`
}

// Workers lists agents, allocations, local worktrees and checkout conflicts.
func (e *Engine) Workers(ctx context.Context) (*Workers, error) {
	agents, err := e.Board.Client.ProjectAgents(ctx, e.Board.Cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	allocs, err := e.Pool.List()
	if err != nil {
		return nil, err
	}
	conflicts, err := e.Pool.Conflicts()
	if err != nil {
		return nil, err
	}
	wts, err := worktree.Existing(ctx, e.RepoRoot)
	if err != nil {
		wts = nil
	}
	if agents == nil {
		agents = []*client.ProjectAgent{}
	}
	if allocs == nil {
		allocs = []pool.Allocation{}
	}
	if conflicts == nil {
		conflicts = []pool.Conflict{}
	}
	if wts == nil {
		wts = []worktree.Worktree{}
	}
	return &Workers{Agents: agents, Allocations: allocs, Worktrees: wts, Conflicts: conflicts}, nil
}

// log appends to the ledger, never failing the caller: the ledger is a
// record, not a gate.
func (e *Engine) log(entry ledger.Entry) {
	if e.Ledger == nil {
		return
	}
	if entry.Actor == "" {
		entry.Actor = e.Board.Identity
	}
	if _, err := e.Ledger.Append(entry); err != nil {
		fmt.Fprintf(os.Stderr, "marshal: ledger: %v\n", err)
	}
}

// Log is the exported form for callers outside the package.
func (e *Engine) Log(entry ledger.Entry) { e.log(entry) }

// Notify posts a Discord message when configured, logging failures.
func (e *Engine) Notify(ctx context.Context, m *discord.Message) {
	if m == nil || !e.Discord.Enabled() {
		return
	}
	if err := e.Discord.Send(ctx, *m); err != nil {
		fmt.Fprintf(os.Stderr, "marshal: discord: %v\n", err)
	}
}
