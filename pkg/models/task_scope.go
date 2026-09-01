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

	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

const (
	maxScopePaths     = 100
	maxScopeEndpoints = 100
)

// TaskScope is the machine-readable "what this task touches" attached to a
// task: the files it will edit, the files it reads or affects, the API surface
// it changes, and free-form notes on what is out of scope. Coding agents write
// it when they decompose a plan and read it before they pick work up.
//
// paths_owned is the enforced part: claiming the task leases those patterns,
// and a claim whose patterns overlap another active lease in the project is
// refused. Everything else is advisory.
type TaskScope struct {
	ID     int64 `xorm:"bigint autoincr not null unique pk" json:"id" readOnly:"true" doc:"The unique, numeric id of this scope."`
	TaskID int64 `xorm:"bigint not null unique index" json:"task_id" param:"projecttask" readOnly:"true" doc:"The task this scope belongs to. Taken from the URL."`

	PathsOwned    []string `xorm:"json null" json:"paths_owned" maxItems:"100" doc:"Repository-relative globs the task will EDIT, e.g. \"pkg/models/tasks.go\" or \"frontend/src/components/**\". Leased on claim: a claim overlapping another task's active lease is refused with 409."`
	PathsAffected []string `xorm:"json null" json:"paths_affected" maxItems:"100" doc:"Repository-relative globs the task reads or depends on but does not edit. Advisory; shown to agents and never enforced."`
	Endpoints     []string `xorm:"json null" json:"endpoints" maxItems:"100" doc:"API surface the task adds or changes, as free text such as \"POST /api/v2/tasks/{id}/claim\". Advisory."`
	Notes         string   `xorm:"longtext null" json:"notes" doc:"Free-form scope notes — typically what is explicitly out of scope. May contain HTML like a task description."`

	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"When this scope was first written."`
	Updated time.Time `xorm:"updated not null" json:"updated" readOnly:"true" doc:"When this scope was last changed."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

func (*TaskScope) TableName() string {
	return "task_scopes"
}

// CanRead follows the task: anyone who can read the task can read its scope.
func (ts *TaskScope) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	t := &Task{ID: ts.TaskID}
	return t.CanRead(s, a)
}

// CanUpdate follows the task's write permission. There is no separate create:
// PUT upserts, because an agent should not have to know whether a scope row
// exists before writing one.
func (ts *TaskScope) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	t := &Task{ID: ts.TaskID}
	return t.CanWrite(s, a)
}

// CanDelete follows the task's write permission.
func (ts *TaskScope) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	t := &Task{ID: ts.TaskID}
	return t.CanWrite(s, a)
}

// getTaskScope returns the scope of a task, or nil when none was written.
func getTaskScope(s *xorm.Session, taskID int64) (*TaskScope, error) {
	scope := &TaskScope{}
	has, err := s.Where("task_id = ?", taskID).Get(scope)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return scope, nil
}

func getTaskScopesForTasks(s *xorm.Session, taskIDs []int64) (map[int64]*TaskScope, error) {
	scopes := []*TaskScope{}
	if len(taskIDs) == 0 {
		return map[int64]*TaskScope{}, nil
	}
	if err := s.In("task_id", taskIDs).Find(&scopes); err != nil {
		return nil, err
	}
	out := make(map[int64]*TaskScope, len(scopes))
	for _, sc := range scopes {
		out[sc.TaskID] = sc
	}
	return out, nil
}

// normalizeScopeList canonicalises every pattern, drops duplicates and empty
// entries, and rejects anything that could escape the repository.
func normalizeScopeList(raw []string, limit int) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		if strings.TrimSpace(r) == "" {
			continue
		}
		p, err := NormalizeScopePath(r)
		if err != nil {
			return nil, err
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) > limit {
		return nil, ErrInvalidScopePath{Pattern: out[limit], Reason: "too many entries"}
	}
	return out, nil
}

func normalizeEndpoints(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		e := strings.TrimSpace(r)
		if e == "" || seen[e] {
			continue
		}
		if len(e) > maxScopePathLength {
			return nil, ErrInvalidScopePath{Pattern: e, Reason: "endpoint longer than 500 characters"}
		}
		seen[e] = true
		out = append(out, e)
	}
	if len(out) > maxScopeEndpoints {
		return nil, ErrInvalidScopePath{Pattern: out[maxScopeEndpoints], Reason: "too many endpoints"}
	}
	return out, nil
}

// ReadOne loads the scope by task. A task without a scope is not an error:
// the zero scope for that task is returned, so clients can always render the
// section and PUT into it.
func (ts *TaskScope) ReadOne(s *xorm.Session, _ web.Auth) error {
	existing, err := getTaskScope(s, ts.TaskID)
	if err != nil {
		return err
	}
	if existing == nil {
		ts.ID = 0
		ts.PathsOwned = []string{}
		ts.PathsAffected = []string{}
		ts.Endpoints = []string{}
		return nil
	}
	*ts = *existing
	ts.ensureLists()
	return nil
}

func (ts *TaskScope) ensureLists() {
	if ts.PathsOwned == nil {
		ts.PathsOwned = []string{}
	}
	if ts.PathsAffected == nil {
		ts.PathsAffected = []string{}
	}
	if ts.Endpoints == nil {
		ts.Endpoints = []string{}
	}
}

// Update writes the scope, creating the row if the task has none. Patterns
// are normalised before saving so equal intents always compare equal when
// leases are checked.
func (ts *TaskScope) Update(s *xorm.Session, _ web.Auth) error {
	owned, err := normalizeScopeList(ts.PathsOwned, maxScopePaths)
	if err != nil {
		return err
	}
	affected, err := normalizeScopeList(ts.PathsAffected, maxScopePaths)
	if err != nil {
		return err
	}
	endpoints, err := normalizeEndpoints(ts.Endpoints)
	if err != nil {
		return err
	}
	ts.PathsOwned, ts.PathsAffected, ts.Endpoints = owned, affected, endpoints

	existing, err := getTaskScope(s, ts.TaskID)
	if err != nil {
		return err
	}
	if existing == nil {
		ts.ID = 0
		if _, err := s.Insert(ts); err != nil {
			return err
		}
	} else {
		ts.ID = existing.ID
		ts.Created = existing.Created
		if _, err := s.ID(existing.ID).Cols("paths_owned", "paths_affected", "endpoints", "notes").Update(ts); err != nil {
			return err
		}
	}

	// A task that is already claimed keeps its leases in step with its
	// declared paths, so widening the scope mid-work is checked against
	// everyone else's leases just like the original claim was.
	held, err := getLeasesForTask(s, ts.TaskID)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		task, err := GetTaskByIDSimple(s, ts.TaskID)
		if err != nil {
			return err
		}
		if err := acquirePathLeasesForTask(s, &task, held[0].UserID, ts.PathsOwned); err != nil {
			return err
		}
	}

	if err := updateProjectLastUpdated(s, &Project{ID: taskProjectID(s, ts.TaskID)}); err != nil {
		return err
	}
	return nil
}

// Delete removes the scope and any leases that were derived from it.
func (ts *TaskScope) Delete(s *xorm.Session, _ web.Auth) error {
	if _, err := s.Where("task_id = ?", ts.TaskID).Delete(&TaskScope{}); err != nil {
		return err
	}
	return releasePathLeasesForTask(s, ts.TaskID)
}

func taskProjectID(s *xorm.Session, taskID int64) int64 {
	t, err := GetTaskByIDSimple(s, taskID)
	if err != nil {
		return 0
	}
	return t.ProjectID
}
