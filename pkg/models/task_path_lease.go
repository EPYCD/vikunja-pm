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

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// TaskPathLease is one path pattern an in-progress task holds exclusively
// within its project. Leases are created from the task's scope when it is
// claimed and released when the task is done, scrapped, deleted or explicitly
// released — so at any moment the set of active leases in a project is the
// set of files agents are currently editing.
type TaskPathLease struct {
	ID        int64  `xorm:"bigint autoincr not null unique pk" json:"id" readOnly:"true" doc:"The unique, numeric id of this lease."`
	TaskID    int64  `xorm:"bigint not null index" json:"task_id" readOnly:"true" doc:"The task holding the lease."`
	ProjectID int64  `xorm:"bigint not null index" json:"project_id" param:"project" readOnly:"true" doc:"The project the lease is scoped to. Leases never conflict across projects."`
	UserID    int64  `xorm:"bigint not null" json:"user_id" readOnly:"true" doc:"The user (usually a bot) that claimed the task and therefore holds the lease."`
	Pattern   string `xorm:"varchar(500) not null" json:"pattern" readOnly:"true" doc:"The normalised, repository-relative glob this lease covers."`

	Task *Task      `xorm:"-" json:"task,omitempty" readOnly:"true" doc:"The holding task, embedded when listing a project's leases."`
	User *user.User `xorm:"-" json:"user,omitempty" readOnly:"true" doc:"The holding user, embedded when listing a project's leases."`

	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When the lease was taken."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

func (*TaskPathLease) TableName() string {
	return "task_path_leases"
}

// CanRead follows the project: leases are visible to anyone who can see the
// project, because knowing what is being edited is the point.
func (l *TaskPathLease) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	p := &Project{ID: l.ProjectID}
	return p.CanRead(s, a)
}

// ReadAll lists the active leases of a project, oldest first, with the
// holding task and user embedded. Not paginated: a project has at most a
// handful of tasks in flight, and every consumer wants the full picture.
func (l *TaskPathLease) ReadAll(s *xorm.Session, a web.Auth, _ string, _ int, _ int) (interface{}, int, int64, error) {
	can, _, err := l.CanRead(s, a)
	if err != nil {
		return nil, 0, 0, err
	}
	if !can {
		return nil, 0, 0, ErrGenericForbidden{}
	}
	leases := []*TaskPathLease{}
	if err := s.Where("project_id = ?", l.ProjectID).OrderBy("created ASC, id ASC").Find(&leases); err != nil {
		return nil, 0, 0, err
	}
	if err := embedLeaseHolders(s, leases); err != nil {
		return nil, 0, 0, err
	}
	return leases, len(leases), int64(len(leases)), nil
}

func embedLeaseHolders(s *xorm.Session, leases []*TaskPathLease) error {
	if len(leases) == 0 {
		return nil
	}
	taskIDs := make([]int64, 0, len(leases))
	userIDs := make([]int64, 0, len(leases))
	for _, l := range leases {
		taskIDs = append(taskIDs, l.TaskID)
		userIDs = append(userIDs, l.UserID)
	}
	tasks := []*Task{}
	if err := s.In("id", taskIDs).Find(&tasks); err != nil {
		return err
	}
	taskMap := make(map[int64]*Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.ID] = t
	}
	users, err := user.GetUsersByIDs(s, userIDs)
	if err != nil {
		return err
	}
	for _, l := range leases {
		l.Task = taskMap[l.TaskID]
		l.User = users[l.UserID]
	}
	return nil
}

func getLeasesForTask(s *xorm.Session, taskID int64) ([]*TaskPathLease, error) {
	leases := []*TaskPathLease{}
	err := s.Where("task_id = ?", taskID).Find(&leases)
	return leases, err
}

func getLeasesForTasks(s *xorm.Session, taskIDs []int64) (map[int64][]*TaskPathLease, error) {
	out := map[int64][]*TaskPathLease{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	leases := []*TaskPathLease{}
	if err := s.In("task_id", taskIDs).OrderBy("id ASC").Find(&leases); err != nil {
		return nil, err
	}
	for _, l := range leases {
		out[l.TaskID] = append(out[l.TaskID], l)
	}
	return out, nil
}

// findConflictingLeases returns, for each of the given patterns, the first
// active lease in the project held by a different task that overlaps it.
func findConflictingLeases(s *xorm.Session, projectID, excludeTaskID int64, patterns []string) ([]ErrPathLeaseConflict, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	active := []*TaskPathLease{}
	if err := s.Where("project_id = ? AND task_id != ?", projectID, excludeTaskID).Find(&active); err != nil {
		return nil, err
	}
	conflicts := []ErrPathLeaseConflict{}
	for _, p := range patterns {
		for _, l := range active {
			if PathPatternsOverlap(p, l.Pattern) {
				conflicts = append(conflicts, ErrPathLeaseConflict{
					TaskID:       excludeTaskID,
					Pattern:      p,
					HeldByTaskID: l.TaskID,
					HeldByUserID: l.UserID,
					HeldPattern:  l.Pattern,
				})
				break
			}
		}
	}
	return conflicts, nil
}

// acquirePathLeasesForTask replaces the task's leases with one per pattern,
// refusing if any pattern overlaps another task's active lease. Called inside
// the claim transaction, after the bucket row is locked, so two claims cannot
// both pass the overlap check.
func acquirePathLeasesForTask(s *xorm.Session, task *Task, userID int64, patterns []string) error {
	conflicts, err := findConflictingLeases(s, task.ProjectID, task.ID, patterns)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return conflicts[0]
	}
	if err := releasePathLeasesForTask(s, task.ID); err != nil {
		return err
	}
	for _, p := range patterns {
		if _, err := s.Insert(&TaskPathLease{
			TaskID:    task.ID,
			ProjectID: task.ProjectID,
			UserID:    userID,
			Pattern:   p,
		}); err != nil {
			return err
		}
	}
	return nil
}

// releasePathLeasesForTask drops every lease the task holds. Safe to call for
// a task that holds none.
func releasePathLeasesForTask(s *xorm.Session, taskID int64) error {
	_, err := s.Where("task_id = ?", taskID).Delete(&TaskPathLease{})
	return err
}

// ReleaseTaskPathLeases is the explicit release used by the API: an agent
// that abandons work, or a human unblocking a stale lease, without touching
// the task's status. The caller has already checked write access on the task.
func ReleaseTaskPathLeases(s *xorm.Session, taskID int64) error {
	return releasePathLeasesForTask(s, taskID)
}
