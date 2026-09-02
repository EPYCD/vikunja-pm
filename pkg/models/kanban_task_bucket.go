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
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// TaskBucket represents the relation between a task and a kanban bucket.
// A task can only appear once per project view which is ensured by a
// unique index on the combination of task_id and project_view_id.
type TaskBucket struct {
	BucketID int64   `xorm:"bigint not null index" json:"bucket_id" param:"bucket" doc:"The bucket to move the task into. On /api/v2 this is taken from the URL; a value in the body is ignored."`
	Bucket   *Bucket `xorm:"-" json:"bucket" readOnly:"true" doc:"The resolved target bucket, including its updated task count."`
	// The task which belongs to the bucket. Together with ProjectViewID
	// this field is part of a unique index to prevent duplicates.
	TaskID int64 `xorm:"bigint not null index unique(task_view)" json:"task_id" doc:"The id of the task to place in the bucket."`
	// The view this bucket belongs to. Combined with TaskID this forms a
	// unique index.
	ProjectViewID int64 `xorm:"bigint not null index unique(task_view)" json:"project_view_id" param:"view" doc:"The view the bucket belongs to. On /api/v2 this is taken from the URL; a value in the body is ignored."`
	ProjectID     int64 `xorm:"-" json:"-" param:"project"`
	Task          *Task `xorm:"-" json:"task" readOnly:"true" doc:"The task as it stands after the move, reflecting any done-state change."`
	// Who last moved the task into this bucket, so "the one who submitted for
	// review is not the one who closes" can be checked.
	MovedByID int64     `xorm:"bigint null" json:"moved_by_id" readOnly:"true" doc:"The user who last moved the task into this bucket. Set by the server."`
	MovedAt   time.Time `xorm:"datetime null" json:"moved_at" readOnly:"true" doc:"When the task was last moved into this bucket. Set by the server."`

	// ClaimTask enforces the lock itself, before and after the move.
	skipClaimGuard bool `xorm:"-" json:"-"`

	web.Permissions `xorm:"-" json:"-"`
	web.CRUDable    `xorm:"-" json:"-"`
}

func (b *TaskBucket) TableName() string {
	return "task_buckets"
}

func (b *TaskBucket) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	bucket := Bucket{
		ID:            b.BucketID,
		ProjectID:     b.ProjectID,
		ProjectViewID: b.ProjectViewID,
	}
	canDoBucket, err := bucket.canDoBucket(s, a)
	if err != nil || !canDoBucket {
		return false, err
	}

	// The task comes from the request body and may live in a different
	// project than the bucket, so it needs its own write check.
	task := &Task{ID: b.TaskID}
	return task.CanWrite(s, a)
}

func (b *TaskBucket) upsert(s *xorm.Session, movedByID int64) (err error) {
	// A native upsert moves the task in one atomic statement, without
	// depending on the affected-row count (MySQL/MariaDB report 0 affected
	// rows for an unchanged value).
	onConflict := "ON CONFLICT (task_id, project_view_id) DO UPDATE SET bucket_id = excluded.bucket_id, moved_by_id = excluded.moved_by_id, moved_at = excluded.moved_at"
	if db.Type() == schemas.MYSQL {
		onConflict = "ON DUPLICATE KEY UPDATE bucket_id = VALUES(bucket_id), moved_by_id = VALUES(moved_by_id), moved_at = VALUES(moved_at)"
	}

	// Raw SQL bypasses xorm's bean-based table-name handling, so qualify the
	// table ourselves to honor a configured postgres schema (database.schema).
	table := s.Engine().TableName(b, true)
	query := "INSERT INTO " + table + " (task_id, project_view_id, bucket_id, moved_by_id, moved_at) VALUES (?, ?, ?, ?, ?) " + onConflict
	_, err = s.Exec(query, b.TaskID, b.ProjectViewID, b.BucketID, movedByID, time.Now())
	return
}

// updateTaskBucket is internally used to actually do the update.
func updateTaskBucket(s *xorm.Session, a web.Auth, b *TaskBucket) (err error) {
	oldTaskBucket := &TaskBucket{}
	_, err = s.
		Where("task_id = ? AND project_view_id = ?", b.TaskID, b.ProjectViewID).
		Get(oldTaskBucket)
	if err != nil {
		return
	}

	if oldTaskBucket.BucketID == b.BucketID {
		// no need to do anything
		return
	}

	view, err := GetProjectViewByIDAndProject(s, b.ProjectViewID, b.ProjectID)
	if err != nil {
		return err
	}

	bucket, err := getBucketByID(s, b.BucketID)
	if err != nil {
		return err
	}

	// If there is a bucket set, make sure they belong to the same project as the task
	if view.ID != bucket.ProjectViewID {
		return ErrBucketDoesNotBelongToProjectView{
			ProjectViewID: view.ID,
			BucketID:      bucket.ID,
		}
	}

	task := &Task{ID: b.TaskID}
	err = task.ReadOne(s, a)
	if err != nil {
		return err
	}

	// Check the bucket limit
	// Only check the bucket limit if the task is being moved between buckets, allow reordering the task within a bucket
	if b.BucketID != 0 && b.BucketID != oldTaskBucket.BucketID {
		taskCount, err := checkBucketLimit(s, a, task, bucket, view, 0)
		if err != nil {
			return err
		}
		bucket.Count = taskCount
	}

	if view.ClaimBucketID != 0 && view.ClaimBucketID == b.BucketID && !b.skipClaimGuard {
		if err := claimOnBucketEntry(s, a, task); err != nil {
			return err
		}
	}

	doneChanged, updateBucket, err := applyDoneBucketTransition(s, a, b, oldTaskBucket, view, task)
	if err != nil {
		return err
	}

	if doneChanged {
		if task.Done {
			task.DoneAt = time.Now()
		} else {
			task.DoneAt = time.Time{}
		}
		_, err = s.Where("id = ?", task.ID).
			Cols(
				"done",
				"due_date",
				"start_date",
				"end_date",
				"done_at",
			).
			Update(task)
		if err != nil {
			return
		}

		err = task.updateReminders(s, task)
		if err != nil {
			return
		}

		if task.Done {
			if err = propagateTaskDone(s, a, task, view); err != nil {
				return
			}
		}
	}

	if updateBucket {
		err = b.upsert(s, a.GetID())
		if err != nil {
			return
		}
		bucket.Count++
	}

	b.Task = task
	b.Bucket = bucket

	return
}

// applyDoneBucketTransition flips the task's done flag when it enters or
// leaves the view's done bucket. A repeating task never rests in the done
// bucket: it is routed back to the default bucket, in which case there may
// be nothing left to move (updateBucket false).
func applyDoneBucketTransition(s *xorm.Session, a web.Auth, b, oldTaskBucket *TaskBucket, view *ProjectView, task *Task) (doneChanged, updateBucket bool, err error) {
	updateBucket = true
	if view.DoneBucketID == 0 {
		return false, true, nil
	}
	if view.DoneBucketID == b.BucketID && !task.Done {
		if err := checkDoneAllowed(s, a, task, oldTaskBucket); err != nil {
			return false, true, err
		}
		doneChanged = true
		task.Done = true
		if task.isRepeating() {
			oldTask := *task
			oldTask.Done = false
			updateDone(&oldTask, task)
			if view.DefaultBucketID != 0 {
				b.BucketID = view.DefaultBucketID
			} else {
				b.BucketID = oldTaskBucket.BucketID
			}
			if b.BucketID == oldTaskBucket.BucketID {
				updateBucket = false
			}
		}
	}
	if oldTaskBucket.BucketID == view.DoneBucketID && task.Done && b.BucketID != view.DoneBucketID {
		doneChanged = true
		task.Done = false
	}
	return doneChanged, updateBucket, nil
}

// propagateTaskDone runs the side effects of a task finishing through a
// bucket move: its leases are freed, its parents may close, and it lands in
// the done bucket of every other manual kanban view of the project.
func propagateTaskDone(s *xorm.Session, a web.Auth, task *Task, view *ProjectView) error {
	if err := releasePathLeasesForTask(s, task.ID); err != nil {
		return err
	}
	if err := completeParentsIfSubtasksDone(s, a, task); err != nil {
		return err
	}
	viewsWithDoneBucket := []*ProjectView{}
	err := s.
		Where("project_id = ? AND view_kind = ? AND bucket_configuration_mode = ? AND id != ? AND done_bucket_id != 0",
			view.ProjectID, ProjectViewKindKanban, BucketConfigurationModeManual, view.ID).
		Find(&viewsWithDoneBucket)
	if err != nil {
		return err
	}
	for _, v := range viewsWithDoneBucket {
		newBucket := &TaskBucket{
			TaskID:        task.ID,
			ProjectViewID: v.ID,
			BucketID:      v.DoneBucketID,
		}
		if err := newBucket.upsert(s, a.GetID()); err != nil {
			return err
		}
	}
	return nil
}

// Update is the handler to update a task bucket
// @Summary Update a task bucket
// @Description Updates a task in a bucket
// @tags task
// @Accept json
// @Produce json
// @Security JWTKeyAuth
// @Param project path int true "Project ID"
// @Param view path int true "Project View ID"
// @Param bucket path int true "Bucket ID"
// @Param taskBucket body models.TaskBucket true "The id of the task you want to move into the bucket."
// @Success 200 {object} models.TaskBucket "The updated task bucket."
// @Failure 400 {object} web.HTTPError "Invalid task bucket object provided."
// @Failure 500 {object} models.Message "Internal error"
// @Router /projects/{project}/views/{view}/buckets/{bucket}/tasks [post]
func (b *TaskBucket) Update(s *xorm.Session, a web.Auth) (err error) {
	err = updateTaskBucket(s, a, b)
	if err != nil {
		return err
	}

	if b.Task != nil {
		events.DispatchOnCommit(s, &TaskUpdatedEvent{
			Task: b.Task,
			Doer: doerFromAuth(s, a),
		})
	}
	return nil
}
