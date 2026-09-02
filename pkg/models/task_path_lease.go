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
	"strings"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/builder"
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
	// Owner is the human behind a bot holder — the person to ask about the
	// lease. Empty when the holder is a human.
	Owner *user.User `xorm:"-" json:"owner,omitempty" readOnly:"true" doc:"When the holder is a bot, the user who owns it, embedded when listing a project's leases."`

	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When the lease was taken."`
	// LastActive moves whenever the holder touches the task (claim, update,
	// comment, heartbeat); it is what tells a live agent from a dead one.
	LastActive time.Time `xorm:"datetime null" json:"last_active" readOnly:"true" doc:"When the holding task last showed activity: a claim, an update, a comment or an explicit heartbeat."`
	Stale      bool      `xorm:"-" json:"stale" readOnly:"true" doc:"True when last_active is older than service.leasestaleafter. Stale leases still block; they are a hint to release."`
	// StaleNotifiedAt records the one stale notification per lease so the
	// cron never nags.
	StaleNotifiedAt time.Time `xorm:"datetime null" json:"-"`

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
	if err := embedBotOwners(s, leases); err != nil {
		return err
	}
	markStaleLeases(leases)
	return nil
}

// embedBotOwners attaches the owning human to every lease whose holder is a bot.
func embedBotOwners(s *xorm.Session, leases []*TaskPathLease) error {
	ownerIDs := []int64{}
	for _, l := range leases {
		if l.User != nil && l.User.IsBot() {
			ownerIDs = append(ownerIDs, l.User.BotOwnerID)
		}
	}
	if len(ownerIDs) == 0 {
		return nil
	}
	owners, err := user.GetUsersByIDs(s, ownerIDs)
	if err != nil {
		return err
	}
	for _, l := range leases {
		if l.User != nil && l.User.IsBot() {
			l.Owner = owners[l.User.BotOwnerID]
		}
	}
	return nil
}

// leaseIsStale is the one definition of "stale" every surface shares.
func leaseIsStale(lastActive time.Time) bool {
	after := config.ServiceLeaseStaleAfter.GetDuration()
	if after <= 0 || lastActive.IsZero() {
		return false
	}
	return time.Since(lastActive) > after
}

func markStaleLeases(leases []*TaskPathLease) {
	for _, l := range leases {
		l.Stale = leaseIsStale(l.LastActive)
	}
}

// touchLeasesHeldBy refreshes last_active on the task's leases when the
// acting user is their holder, so ordinary work counts as a heartbeat.
func touchLeasesHeldBy(s *xorm.Session, taskID, userID int64) error {
	_, err := s.Where("task_id = ? AND user_id = ?", taskID, userID).
		Cols("last_active").
		Update(&TaskPathLease{LastActive: time.Now()})
	return err
}

// HeartbeatTaskPathLeases is the explicit "still working" signal for agents
// that go long between board updates. Refreshes every lease on the task and
// returns them. The caller has checked write access on the task.
func HeartbeatTaskPathLeases(s *xorm.Session, taskID int64) ([]*TaskPathLease, error) {
	if _, err := s.Where("task_id = ?", taskID).Cols("last_active").Update(&TaskPathLease{LastActive: time.Now()}); err != nil {
		return nil, err
	}
	leases, err := getLeasesForTask(s, taskID)
	if err != nil {
		return nil, err
	}
	markStaleLeases(leases)
	return leases, nil
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
	markStaleLeases(leases)
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
					LastActive:   l.LastActive,
					Stale:        leaseIsStale(l.LastActive),
					HeldSince:    l.Created,
				})
				break
			}
		}
	}
	if err := describeLeaseHolders(s, conflicts); err != nil {
		return nil, err
	}
	return conflicts, nil
}

// describeLeaseHolders fills in the holder's username and branch so a refusal
// names who to talk to rather than an id to look up.
func describeLeaseHolders(s *xorm.Session, conflicts []ErrPathLeaseConflict) error {
	if len(conflicts) == 0 {
		return nil
	}
	userIDs := make([]int64, 0, len(conflicts))
	taskIDs := make([]int64, 0, len(conflicts))
	for _, c := range conflicts {
		userIDs = append(userIDs, c.HeldByUserID)
		taskIDs = append(taskIDs, c.HeldByTaskID)
	}
	users, err := user.GetUsersByIDs(s, userIDs)
	if err != nil {
		return err
	}
	branches, err := branchLabelsForTasks(s, taskIDs)
	if err != nil {
		return err
	}
	for i := range conflicts {
		if u := users[conflicts[i].HeldByUserID]; u != nil {
			conflicts[i].HeldByUsername = u.Username
		}
		conflicts[i].HeldByBranch = branches[conflicts[i].HeldByTaskID]
	}
	return nil
}

// branchLabelPrefix is the label veans attaches on claim to say which branch
// carries the work.
const branchLabelPrefix = "veans:branch:"

func branchLabelsForTasks(s *xorm.Session, taskIDs []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	rows := []struct {
		TaskID int64  `xorm:"task_id"`
		Title  string `xorm:"title"`
	}{}
	err := s.Table("labels").
		Join("INNER", "label_tasks", "label_tasks.label_id = labels.id").
		In("label_tasks.task_id", taskIDs).
		Where(builder.Like{"labels.title", branchLabelPrefix + "%"}).
		Cols("label_tasks.task_id", "labels.title").
		Find(&rows)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if _, seen := out[r.TaskID]; !seen {
			out[r.TaskID] = strings.TrimPrefix(r.Title, branchLabelPrefix)
		}
	}
	return out, nil
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
			TaskID:     task.ID,
			ProjectID:  task.ProjectID,
			UserID:     userID,
			Pattern:    p,
			LastActive: time.Now(),
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
func ReleaseTaskPathLeases(s *xorm.Session, a web.Auth, taskID int64) error {
	if err := releasePathLeasesForTask(s, taskID); err != nil {
		return err
	}
	task, err := GetTaskByIDSimple(s, taskID)
	if err != nil {
		return err
	}
	doer, err := user.GetFromAuth(a)
	if err != nil {
		return err
	}
	events.DispatchOnCommit(s, &TaskLeasesReleasedEvent{Task: &task, Doer: doer})
	return nil
}
