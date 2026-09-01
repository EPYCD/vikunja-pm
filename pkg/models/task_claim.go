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
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// TaskClaim is the request body of the atomic task claim.
//
// A claim is "take this task if nobody else has it": in one transaction it
// checks the task is unassigned (or already the caller's), optionally that it
// still sits in the bucket the caller last saw it in, then moves it into the
// target bucket and assigns the caller. Two agents racing for the same task
// therefore get exactly one winner instead of two assignees.
type TaskClaim struct {
	TaskID int64 `json:"-" param:"projecttask"`
	// The view whose buckets the claim operates on.
	ProjectViewID int64 `json:"project_view_id" required:"true" minimum:"1" doc:"The kanban view whose buckets the claim operates on."`
	// Where the task goes once claimed, typically the view's In Progress bucket.
	BucketID int64 `json:"bucket_id" required:"true" minimum:"1" doc:"The bucket to move the task into on success, typically In Progress. Must belong to project_view_id."`
	// A compare-and-swap guard: if set, the claim is refused with 409 when the
	// task is not currently in this bucket, so a task somebody else already
	// moved on is never claimed on stale information.
	ExpectedBucketID int64 `json:"expected_bucket_id,omitempty" doc:"Optional guard: refuse the claim unless the task is currently in this bucket. Pass the Todo bucket to claim only tasks nobody has started. Ignored when the task is already claimed by you and already in bucket_id."`
}

// ClaimTask performs the atomic claim described on TaskClaim. It expects the
// caller to have already verified write access (TaskBucket.CanUpdate covers the
// bucket and the task) and to be running inside a transaction: the current
// bucket row is read under a row lock where the database supports one, so the
// check-then-move is not interleavable with a concurrent claim.
//
// Re-claiming a task the caller already holds is a no-op that returns the task,
// so an agent can call it on every start-of-work without special-casing.
func ClaimTask(s *xorm.Session, a web.Auth, c *TaskClaim) (*Task, error) {
	claimant, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}

	task, err := GetTaskByIDSimple(s, c.TaskID)
	if err != nil {
		return nil, err
	}

	// SELECT ... FOR UPDATE pins the task's bucket row for the rest of the
	// transaction on the engines that support it. SQLite has no row locks
	// but serialises writers at the database level, which gives the same
	// guarantee for the check-then-move below.
	q := s.Where("task_id = ? AND project_view_id = ?", c.TaskID, c.ProjectViewID)
	if db.Type() == schemas.MYSQL || db.Type() == schemas.POSTGRES {
		q = q.ForUpdate()
	}
	current := &TaskBucket{}
	if _, err := q.Get(current); err != nil {
		return nil, err
	}

	assignees := []*TaskAssginee{}
	if err := s.Where("task_id = ?", c.TaskID).Find(&assignees); err != nil {
		return nil, err
	}
	mine := false
	for _, as := range assignees {
		if as.UserID != claimant.ID {
			return nil, ErrTaskAlreadyClaimed{TaskID: c.TaskID, UserID: as.UserID}
		}
		mine = true
	}

	alreadyThere := current.BucketID == c.BucketID
	if !(mine && alreadyThere) && c.ExpectedBucketID != 0 && current.BucketID != c.ExpectedBucketID {
		return nil, ErrTaskNotInExpectedBucket{
			TaskID:           c.TaskID,
			BucketID:         current.BucketID,
			ExpectedBucketID: c.ExpectedBucketID,
		}
	}

	if !alreadyThere {
		tb := &TaskBucket{
			TaskID:        c.TaskID,
			ProjectViewID: c.ProjectViewID,
			BucketID:      c.BucketID,
			ProjectID:     task.ProjectID,
		}
		if err := updateTaskBucket(s, a, tb); err != nil {
			return nil, err
		}
	}

	if !mine {
		project, err := GetProjectSimpleByTaskID(s, c.TaskID)
		if err != nil {
			return nil, err
		}
		if err := task.addNewAssigneeByID(s, claimant.ID, project, a); err != nil {
			return nil, err
		}
	}

	full := &Task{ID: c.TaskID}
	if err := full.ReadOne(s, a); err != nil {
		return nil, err
	}
	return full, nil
}
