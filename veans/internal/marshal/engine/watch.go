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

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os/exec"
	"strings"
	"time"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/board"
	"code.vikunja.io/veans/internal/marshal/discord"
	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/marshal/notify"
	"code.vikunja.io/veans/internal/marshal/refs"
)

// TickResult summarises one pass of the watcher.
type TickResult struct {
	At          time.Time `json:"at"`
	Rev         string    `json:"rev"`
	Drifted     []string  `json:"drifted"`
	Vanished    []string  `json:"vanished"`
	Broken      int       `json:"broken"`
	Pastes      int       `json:"pastes"`
	Stale       int       `json:"stale"`
	Strays      int       `json:"strays"`
	Collisions  int       `json:"collisions"`
	HealthOK    bool      `json:"health_ok"`
	Notified    int       `json:"notified"`
	Errors      []string  `json:"errors,omitempty"`
	FetchedSpec bool      `json:"fetched_spec"`
}

// Tick is one pass: refresh the spec revision, detect drift against the
// last index, re-check references, pastes, stale branches, strays and the
// invariants, and announce what changed since the previous pass.
func (e *Engine) Tick(ctx context.Context, base string) *TickResult {
	res := &TickResult{At: time.Now().UTC()}
	if e.SpecRev != "" {
		if err := e.fetchSpec(ctx); err != nil {
			res.Errors = append(res.Errors, "git fetch: "+err.Error())
		} else {
			res.FetchedSpec = true
		}
	}

	e.mu.Lock()
	prev := e.index
	e.mu.Unlock()
	ix, _, err := e.Index(ctx)
	if err != nil {
		res.Errors = append(res.Errors, "index: "+err.Error())
		return res
	}
	res.Rev = ix.Rev
	if prev != nil && prev.Rev != ix.Rev {
		e.InvalidateCorpus()
	}

	snap, err := e.Board.Snapshot(ctx)
	if err != nil {
		res.Errors = append(res.Errors, "board: "+err.Error())
		return res
	}

	if prev != nil {
		drift := refs.Diff(prev, ix)
		res.Drifted = drift.Changed
		res.Vanished = drift.Vanished
		res.Notified += e.announceDrift(ctx, snap, ix, drift)
	}
	res.Notified += e.announceReferences(ctx, snap, res)
	res.Notified += e.announceBranches(ctx, snap, base, res)
	res.Notified += e.announceHealth(ctx, snap, res)
	return res
}

func (e *Engine) fetchSpec(ctx context.Context) error {
	remote := "origin"
	if i := strings.Index(e.SpecRev, "/"); i > 0 {
		remote = e.SpecRev[:i]
	}
	argv := []string{"-C", e.RepoRoot, "fetch", "--quiet", remote}
	cmd := exec.CommandContext(ctx, "git", argv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// announceDrift is M3.5: when an anchor an open task references changed or
// vanished, the task gets a comment, a label and the assignee a Discord card.
func (e *Engine) announceDrift(ctx context.Context, snap *board.Snapshot, ix *refs.Index, drift refs.Drift) int {
	changed := map[string]bool{}
	for _, id := range drift.Changed {
		changed[id] = true
	}
	vanished := map[string]bool{}
	for _, id := range drift.Vanished {
		vanished[id] = true
	}
	if len(changed) == 0 && len(vanished) == 0 {
		return 0
	}
	n := 0
	prefixes := e.Prefixes()
	for _, t := range snap.Tasks {
		for _, r := range refs.Extract(TaskText(t), prefixes) {
			kind := ""
			switch {
			case changed[r.ID]:
				kind = "changed"
			case vanished[r.ID]:
				kind = "vanished"
			default:
				continue
			}
			key := fmt.Sprintf("drift:%d:%s", t.ID, r.ID)
			value := kind + "@" + ix.Rev
			if a, ok := ix.Anchors[r.ID]; ok {
				value = kind + "@" + a.Hash
			}
			if e.flags.Seen(key, value) {
				continue
			}
			n++
			msg := fmt.Sprintf("Marshal: %s %s since this task referenced it (%s).", r.ID, kind, provenance(ix, r.ID))
			comment := "<p>" + html.EscapeString(msg) + " Re-read the requirement before continuing.</p>"
			if err := e.Board.Comment(ctx, t.ID, comment); err == nil {
				_ = e.Board.EnsureLabel(ctx, t, board.LabelDrift, "f59e0b")
			}
			e.log(ledger.Entry{Action: "drift", TaskID: t.ID, Subject: r.ID, Outcome: kind, Metadata: map[string]any{"rev": ix.Rev}})
			e.Notify(ctx, e.Format.FromFinding(notify.Finding{
				Kind: "drift", TaskID: t.ID, Identifier: t.Identifier, Title: t.Title,
				Summary: msg,
				Details: []discord.Field{{Name: "Assignee", Value: assigneeNames(t), Inline: true}, {Name: "Reference", Value: r.ID, Inline: true}},
			}))
		}
	}
	return n
}

func provenance(ix *refs.Index, id string) string {
	if a, ok := ix.Anchors[id]; ok {
		if ix.Rev != "" {
			return a.File + "@" + short(ix.Rev)
		}
		return a.File
	}
	if ix.Rev != "" {
		return "spec@" + short(ix.Rev)
	}
	return "working tree"
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

func assigneeNames(t *client.Task) string {
	names := []string{}
	for _, a := range t.Assignees {
		names = append(names, board.DisplayName(a))
	}
	if len(names) == 0 {
		return "unassigned"
	}
	return strings.Join(names, ", ")
}

// announceReferences flags broken references (M3.3) and pastes (M3.4) once
// per task and state, and clears the label when the task is fixed.
func (e *Engine) announceReferences(ctx context.Context, snap *board.Snapshot, res *TickResult) int {
	rep, err := e.References(ctx, snap)
	if err != nil {
		res.Errors = append(res.Errors, "references: "+err.Error())
		return 0
	}
	res.Broken = len(rep.Broken)
	res.Pastes = len(rep.Pastes)
	n := 0
	brokenBy := map[int64][]string{}
	for _, b := range rep.Broken {
		brokenBy[b.TaskID] = append(brokenBy[b.TaskID], b.Ref)
	}
	pastesBy := map[int64][]refs.PasteMatch{}
	for _, p := range rep.Pastes {
		pastesBy[p.TaskID] = append(pastesBy[p.TaskID], p.Match)
	}
	for _, t := range snap.Tasks {
		if ids := brokenBy[t.ID]; len(ids) > 0 {
			if !e.flags.Seen(fmt.Sprintf("broken:%d", t.ID), strings.Join(ids, ",")) {
				n++
				msg := fmt.Sprintf("Marshal: %s no longer resolve%s in the spec (%s).", strings.Join(ids, ", "), plural(len(ids), "s", ""), rep.Rev)
				_ = e.Board.Comment(ctx, t.ID, "<p>"+html.EscapeString(msg)+"</p>")
				_ = e.Board.EnsureLabel(ctx, t, board.LabelBroken, "ef4444")
				e.log(ledger.Entry{Action: "broken_ref", TaskID: t.ID, Subject: strings.Join(ids, ","), Outcome: "flagged"})
				e.Notify(ctx, e.Format.FromFinding(notify.Finding{Kind: "broken_ref", TaskID: t.ID, Identifier: t.Identifier, Title: t.Title, Summary: msg}))
			}
		} else {
			e.flags.Clear(fmt.Sprintf("broken:%d", t.ID))
			_ = e.Board.DropLabel(ctx, t, board.LabelBroken)
		}
		if ms := pastesBy[t.ID]; len(ms) > 0 {
			sig := fmt.Sprintf("%d:%d", len(ms), ms[0].Words)
			if !e.flags.Seen(fmt.Sprintf("paste:%d", t.ID), sig) {
				n++
				m := ms[0]
				msg := fmt.Sprintf("Marshal: this task repeats %d words of %s verbatim (from line %d). Link the requirement instead of copying it; the copy will go stale.", m.Words, m.File, m.Line)
				_ = e.Board.Comment(ctx, t.ID, "<p>"+html.EscapeString(msg)+"</p>")
				_ = e.Board.EnsureLabel(ctx, t, board.LabelPaste, "f59e0b")
				e.log(ledger.Entry{Action: "paste", TaskID: t.ID, Subject: m.File, Outcome: "flagged", Metadata: map[string]any{"words": m.Words, "line": m.Line}})
				e.Notify(ctx, e.Format.FromFinding(notify.Finding{Kind: "paste", TaskID: t.ID, Identifier: t.Identifier, Title: t.Title, Summary: msg, Details: []discord.Field{{Name: "Excerpt", Value: m.Excerpt}}}))
			}
		} else {
			e.flags.Clear(fmt.Sprintf("paste:%d", t.ID))
			_ = e.Board.DropLabel(ctx, t, board.LabelPaste)
		}
	}
	return n
}

// announceBranches is M1.8 and M1.9 on a timer: strays on a branch and
// branches gone quiet.
func (e *Engine) announceBranches(ctx context.Context, snap *board.Snapshot, base string, res *TickResult) int {
	checks, err := e.Reconcile(ctx, snap, base, "")
	if err != nil {
		res.Errors = append(res.Errors, "reconcile: "+err.Error())
		return 0
	}
	n := 0
	for _, c := range checks {
		t := snap.ByID[c.TaskID]
		if t == nil {
			continue
		}
		if c.Stale {
			res.Stale++
			if !e.flags.Seen(fmt.Sprintf("stale:%d", c.TaskID), c.LastActivity.Format(time.RFC3339)) {
				n++
				msg := fmt.Sprintf("Marshal: branch %s has had no commit since %s while this task holds its files.", c.Branch, c.LastActivity.Format("2006-01-02"))
				_ = e.Board.Comment(ctx, t.ID, "<p>"+html.EscapeString(msg)+"</p>")
				_ = e.Board.EnsureLabel(ctx, t, board.LabelStale, "64748b")
				e.log(ledger.Entry{Action: "stale", TaskID: t.ID, Subject: c.Branch, Outcome: "flagged", Metadata: map[string]any{"last_activity": c.LastActivity}})
				e.Notify(ctx, e.Format.FromFinding(notify.Finding{Kind: "stale", TaskID: t.ID, Identifier: t.Identifier, Title: t.Title, Summary: msg, Details: []discord.Field{{Name: "Assignee", Value: assigneeNames(t), Inline: true}, {Name: "Branch", Value: c.Branch, Inline: true}}}))
			}
		} else {
			e.flags.Clear(fmt.Sprintf("stale:%d", c.TaskID))
			_ = e.Board.DropLabel(ctx, t, board.LabelStale)
		}
		if c.Result != nil && !c.Result.OK {
			res.Strays += c.Result.Strays
			res.Collisions += c.Result.Collisions
			sig := fmt.Sprintf("%d:%d:%d", c.Changed, c.Result.Strays, c.Result.Collisions)
			if !e.flags.Seen(fmt.Sprintf("stray:%d", c.TaskID), sig) {
				n++
				files := []string{}
				for _, f := range c.Result.Files {
					if f.Verdict == "unscoped" || f.Verdict == "leased_by_other" {
						files = append(files, f.Path+" ("+f.Verdict+")")
					}
					if len(files) == 8 {
						break
					}
				}
				msg := fmt.Sprintf("Marshal: branch %s touches %d file%s outside this task's claim and %d leased by another task.", c.Branch, c.Result.Strays, plural(c.Result.Strays, "s", ""), c.Result.Collisions)
				_ = e.Board.Comment(ctx, t.ID, "<p>"+html.EscapeString(msg)+"</p><ul><li>"+html.EscapeString(strings.Join(files, "</li><li>"))+"</li></ul>")
				_ = e.Board.EnsureLabel(ctx, t, board.LabelStray, "ef4444")
				e.log(ledger.Entry{Action: "stray", TaskID: t.ID, Subject: c.Branch, Outcome: "flagged", Metadata: map[string]any{"strays": c.Result.Strays, "collisions": c.Result.Collisions}})
				e.Notify(ctx, e.Format.FromFinding(notify.Finding{Kind: "stray", TaskID: t.ID, Identifier: t.Identifier, Title: t.Title, Summary: msg, Details: []discord.Field{{Name: "Files", Value: strings.Join(files, "\n")}}}))
			}
		} else if c.Result != nil {
			e.flags.Clear(fmt.Sprintf("stray:%d", c.TaskID))
			_ = e.Board.DropLabel(ctx, t, board.LabelStray)
		}
	}
	return n
}

// announceHealth is M1.10: the invariants, announced when they change.
func (e *Engine) announceHealth(ctx context.Context, snap *board.Snapshot, res *TickResult) int {
	h := e.Health(snap)
	res.HealthOK = h.OK
	sig := fmt.Sprintf("ok=%t collisions=%d cycles=%d tasks=%d", h.OK, h.Collisions, h.Cycles, h.Tasks)
	if e.flags.Seen("health", sig) {
		return 0
	}
	e.log(ledger.Entry{Action: "health", Subject: "invariants", Outcome: map[bool]string{true: "ok", false: "violated"}[h.OK], Metadata: map[string]any{"collisions": h.Collisions, "cycles": h.Cycles, "tasks": h.Tasks, "roots": len(h.UnblockedRoots)}})
	summary := fmt.Sprintf("%d open stories, %d collisions, %d cycles, %d unblocked roots.", h.Tasks, h.Collisions, h.Cycles, len(h.UnblockedRoots))
	fields := []discord.Field{}
	for i, f := range h.Findings {
		if i == 6 {
			fields = append(fields, discord.Field{Name: "…", Value: fmt.Sprintf("%d more", len(h.Findings)-6)})
			break
		}
		fields = append(fields, discord.Field{Name: f.Code, Value: f.Message})
	}
	e.Notify(ctx, e.Format.FromFinding(notify.Finding{Kind: "health", Summary: summary, Details: fields}))
	return 1
}

// HandleDelivery records a board webhook delivery and relays it to Discord.
func (e *Engine) HandleDelivery(ctx context.Context, d notify.Delivery) {
	var ev notify.Event
	_ = json.Unmarshal(d.Data, &ev)
	entry := ledger.Entry{Action: d.EventName, Outcome: "ok", Subject: "webhook"}
	if ev.Task != nil {
		entry.TaskID = ev.Task.ID
	}
	if ev.Doer != nil {
		entry.Actor = ev.Doer.Username
	}
	e.log(entry)
	e.Notify(ctx, e.Format.FromDelivery(d))
}

func plural(n int, many, one string) string {
	if n == 1 {
		return one
	}
	return many
}
