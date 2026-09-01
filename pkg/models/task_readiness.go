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

package models

import (
	"sort"

	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// Readiness reasons. A task is ready when it has none.
const (
	ReadinessReasonDone          = "done"
	ReadinessReasonAssigned      = "assigned"
	ReadinessReasonBlocked       = "blocked"
	ReadinessReasonLeaseConflict = "lease_conflict"
)

// TaskReadiness is one row of the ready queue: a task in the view's starting
// bucket and exactly why it can or cannot be picked up right now. It is the
// server-side truth `veans list --ready` and the board both read, so an agent
// and a human looking at the same queue see the same answer.
type TaskReadiness struct {
	Task           *Task                  `json:"task" doc:"The task, with assignees, labels and related tasks populated."`
	Ready          bool                   `json:"ready" doc:"True when the task can be claimed right now: not done, unassigned, no unfinished blocker, and no path it owns is leased by another task."`
	Reasons        []string               `json:"reasons" doc:"Why the task is not ready. Empty when ready. One or more of: done, assigned, blocked, lease_conflict."`
	BlockedBy      []*Task                `json:"blocked_by" doc:"The unfinished tasks this one is blocked by."`
	LeaseConflicts []ErrPathLeaseConflict `json:"lease_conflicts" doc:"Each owned path that overlaps another task's active lease, with the holder."`
}

// GetTaskReadiness computes readiness for every open task in one bucket of a
// kanban view — by default the view's default bucket, which is where new work
// waits. The caller has checked read access on the project.
func GetTaskReadiness(s *xorm.Session, a web.Auth, view *ProjectView, bucketID int64) ([]*TaskReadiness, error) {
	if bucketID == 0 {
		var err error
		bucketID, err = getDefaultBucketID(s, view)
		if err != nil {
			return nil, err
		}
	}

	rows := []*TaskBucket{}
	if err := s.Where("project_view_id = ? AND bucket_id = ?", view.ID, bucketID).Find(&rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*TaskReadiness{}, nil
	}
	taskIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		taskIDs = append(taskIDs, r.TaskID)
	}

	tasks := []*Task{}
	if err := s.In("id", taskIDs).Where("done = ?", false).Find(&tasks); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return []*TaskReadiness{}, nil
	}
	// Queue order is board order: the view's positions, then id for tasks
	// that were never positioned.
	positions := []*TaskPosition{}
	if err := s.In("task_id", taskIDs).Where("project_view_id = ?", view.ID).Find(&positions); err != nil {
		return nil, err
	}
	posByTask := make(map[int64]float64, len(positions))
	for _, p := range positions {
		posByTask[p.TaskID] = p.Position
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		pi, pj := posByTask[tasks[i].ID], posByTask[tasks[j].ID]
		if pi != pj {
			return pi < pj
		}
		return tasks[i].ID < tasks[j].ID
	})
	taskMap := make(map[int64]*Task, len(tasks))
	openIDs := make([]int64, 0, len(tasks))
	for _, t := range tasks {
		taskMap[t.ID] = t
		openIDs = append(openIDs, t.ID)
	}
	if err := addMoreInfoToTasks(s, taskMap, a, view, nil); err != nil {
		return nil, err
	}

	scopes, err := getTaskScopesForTasks(s, openIDs)
	if err != nil {
		return nil, err
	}

	out := make([]*TaskReadiness, 0, len(tasks))
	for _, t := range tasks {
		r := &TaskReadiness{
			Task:           t,
			Reasons:        []string{},
			BlockedBy:      []*Task{},
			LeaseConflicts: []ErrPathLeaseConflict{},
		}
		if len(t.Assignees) > 0 {
			r.Reasons = append(r.Reasons, ReadinessReasonAssigned)
		}
		for _, b := range t.RelatedTasks[RelationKindBlocked] {
			if b != nil && !b.Done {
				r.BlockedBy = append(r.BlockedBy, b)
			}
		}
		if len(r.BlockedBy) > 0 {
			r.Reasons = append(r.Reasons, ReadinessReasonBlocked)
		}
		if sc := scopes[t.ID]; sc != nil && len(sc.PathsOwned) > 0 {
			conflicts, err := findConflictingLeases(s, t.ProjectID, t.ID, sc.PathsOwned)
			if err != nil {
				return nil, err
			}
			if len(conflicts) > 0 {
				r.LeaseConflicts = conflicts
				r.Reasons = append(r.Reasons, ReadinessReasonLeaseConflict)
			}
		}
		r.Ready = len(r.Reasons) == 0
		out = append(out, r)
	}
	return out, nil
}

// unfinishedBlockers returns the ids of tasks that block the given task and
// are not done. Used by the claim to refuse work whose prerequisites are
// still open.
func unfinishedBlockers(s *xorm.Session, taskID int64) ([]int64, error) {
	relations := []*TaskRelation{}
	if err := s.Where("task_id = ? AND relation_kind = ?", taskID, RelationKindBlocked).Find(&relations); err != nil {
		return nil, err
	}
	if len(relations) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(relations))
	for _, r := range relations {
		ids = append(ids, r.OtherTaskID)
	}
	open := []*Task{}
	if err := s.In("id", ids).Where("done = ?", false).Cols("id").Find(&open); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(open))
	for _, t := range open {
		out = append(out, t.ID)
	}
	return out, nil
}
