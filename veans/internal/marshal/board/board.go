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

// Package board is Marshal's view of the Workman project: the veans
// configuration for coordinates, a token for identity, and the reads every
// check needs turned into the plain shapes the checkers take.
package board

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/config"
	"code.vikunja.io/veans/internal/credentials"
	"code.vikunja.io/veans/internal/marshal/invariants"
	"code.vikunja.io/veans/internal/output"
)

// MarshalBotPrefix names the identities Marshal creates: the service itself
// and the CI receipt bot.
const (
	MarshalBotPrefix = "bot-marshal-"
	CIBotPrefix      = "bot-ci-"
)

// Board is an authenticated handle on one project.
type Board struct {
	Cfg    *config.Config
	Client *client.Client
	// Identity is the username the token belongs to, for the ledger.
	Identity string
}

// Open loads .veans.yml from dir upward and picks a token: MARSHAL_TOKEN,
// then the Marshal bot's stored credential, then the repo's veans bot as a
// last resort so read-only commands work on a checkout that only ran
// `veans init`.
func Open(dir string) (*Board, error) {
	path, err := config.Find(dir)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, output.Wrap(output.CodeNotConfigured, err, "no .veans.yml found — run `veans init` first")
		}
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	b := &Board{Cfg: cfg}
	if tok := os.Getenv("MARSHAL_TOKEN"); tok != "" {
		b.Client = client.New(cfg.Server, tok)
		b.Identity = "marshal"
		return b, nil
	}
	store := credentials.Default()
	repo := strings.TrimPrefix(cfg.Bot.Username, "bot-")
	for _, account := range []string{MarshalBotPrefix + repo, cfg.Bot.Username} {
		tok, err := store.Get(cfg.Server, account)
		if err == nil && tok != "" {
			b.Client = client.New(cfg.Server, tok)
			b.Identity = account
			if cfg.HTTPTimeout > 0 {
				b.Client.HTTPClient.Timeout = cfg.HTTPTimeout
			}
			return b, nil
		}
	}
	return nil, output.New(output.CodeAuth, "no token for Marshal on %s — set MARSHAL_TOKEN or run `marshal setup`", cfg.Server)
}

// TaskURL is the board page of a task.
func (b *Board) TaskURL(taskID int64) string {
	return strings.TrimRight(b.Cfg.Server, "/") + fmt.Sprintf("/tasks/%d", taskID)
}

// Snapshot is everything the checkers read at once.
type Snapshot struct {
	Tasks  []*client.Task
	ByID   map[int64]*client.Task
	Leases []*client.TaskPathLease
}

// Snapshot reads the project's open tasks with scope, buckets and relations,
// plus every live lease.
func (b *Board) Snapshot(ctx context.Context) (*Snapshot, error) {
	tasks, err := b.Client.ListProjectTasks(ctx, b.Cfg.ProjectID, &client.TaskListOptions{
		Filter: "done = false",
		Expand: []string{"buckets", "scope", "leases"},
	})
	if err != nil {
		return nil, err
	}
	leases, err := b.Client.ListProjectLeases(ctx, b.Cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Tasks: tasks, ByID: map[int64]*client.Task{}, Leases: leases}
	for _, t := range tasks {
		s.ByID[t.ID] = t
	}
	return s, nil
}

// InvariantTasks converts the snapshot into the invariants checker's input.
func (s *Snapshot) InvariantTasks() ([]invariants.Task, []invariants.Lease) {
	tasks := make([]invariants.Task, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		it := invariants.Task{ID: t.ID, Identifier: t.Identifier, Title: t.Title, Done: t.Done}
		for _, a := range t.Assignees {
			it.Assignees = append(it.Assignees, displayName(a))
		}
		if t.Scope != nil {
			it.Paths = append(it.Paths, t.Scope.PathsOwned...)
		}
		it.BlockedBy = relatedIDs(t, "blocked")
		it.Follows = relatedIDs(t, "follows")
		if parents := relatedIDs(t, "parenttask"); len(parents) > 0 {
			it.ParentID = parents[0]
		}
		tasks = append(tasks, it)
	}
	leases := make([]invariants.Lease, 0, len(s.Leases))
	for _, l := range s.Leases {
		leases = append(leases, invariants.Lease{TaskID: l.TaskID, Pattern: l.Pattern, Stale: l.Stale})
	}
	return tasks, leases
}

// BranchOf returns the branch a task is being worked on, from its
// veans:branch label, or "".
func BranchOf(t *client.Task) string {
	for _, l := range t.Labels {
		if strings.HasPrefix(l.Title, "veans:branch:") {
			return strings.TrimPrefix(l.Title, "veans:branch:")
		}
	}
	return ""
}

func relatedIDs(t *client.Task, kind string) []int64 {
	out := []int64{}
	for _, r := range t.RelatedTasks[kind] {
		if r != nil {
			out = append(out, r.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func displayName(u *client.User) string {
	if u == nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// DisplayName is the name to show for a user.
func DisplayName(u *client.User) string { return displayName(u) }

// Comment posts an HTML comment on a task.
func (b *Board) Comment(ctx context.Context, taskID int64, html string) error {
	_, err := b.Client.AddTaskComment(ctx, taskID, html)
	return err
}

// HasCommentContaining reports whether any comment on the task contains
// needle, for once-only notices.
func (b *Board) HasCommentContaining(ctx context.Context, taskID int64, needle string) (bool, error) {
	comments, err := b.Client.ListTaskComments(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, c := range comments {
		if strings.Contains(c.Comment, needle) {
			return true, nil
		}
	}
	return false, nil
}

// EnsureLabel attaches a label by title, creating it when missing.
func (b *Board) EnsureLabel(ctx context.Context, task *client.Task, title, hexColor string) error {
	for _, l := range task.Labels {
		if l.Title == title {
			return nil
		}
	}
	labels, err := b.Client.ListLabels(ctx, title)
	if err != nil {
		return err
	}
	var label *client.Label
	for _, l := range labels {
		if l.Title == title {
			label = l
			break
		}
	}
	if label == nil {
		label, err = b.Client.CreateLabel(ctx, &client.Label{Title: title, HexColor: hexColor})
		if err != nil {
			return err
		}
	}
	if err := b.Client.AddLabelToTask(ctx, task.ID, label.ID); err != nil {
		return err
	}
	task.Labels = append(task.Labels, label)
	return nil
}

// DropLabel detaches a label by title if present.
func (b *Board) DropLabel(ctx context.Context, task *client.Task, title string) error {
	for i, l := range task.Labels {
		if l.Title == title {
			if err := b.Client.RemoveLabelFromTask(ctx, task.ID, l.ID); err != nil {
				return err
			}
			task.Labels = append(task.Labels[:i], task.Labels[i+1:]...)
			return nil
		}
	}
	return nil
}

// Labels Marshal manages. They are flags, not state: the ledger has the history.
const (
	LabelDrift  = "marshal:drift"
	LabelBroken = "marshal:broken-ref"
	LabelPaste  = "marshal:paste"
	LabelStale  = "marshal:stale"
	LabelStray  = "marshal:stray"
)
