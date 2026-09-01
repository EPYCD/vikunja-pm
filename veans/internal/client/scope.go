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

package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// TaskScope mirrors pkg/models.TaskScope: what a task will edit, what it
// touches and which API surface it changes. Only paths_owned is enforced
// (leased on claim); the rest is advisory context for agents.
type TaskScope struct {
	ID            int64     `json:"id,omitempty"`
	TaskID        int64     `json:"task_id,omitempty"`
	PathsOwned    []string  `json:"paths_owned"`
	PathsAffected []string  `json:"paths_affected"`
	Endpoints     []string  `json:"endpoints"`
	Notes         string    `json:"notes"`
	Created       time.Time `json:"created,omitempty"`
	Updated       time.Time `json:"updated,omitempty"`
}

// TaskPathLease is one path pattern held by an in-progress task.
type TaskPathLease struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"task_id"`
	ProjectID  int64     `json:"project_id"`
	UserID     int64     `json:"user_id"`
	Pattern    string    `json:"pattern"`
	Task       *Task     `json:"task,omitempty"`
	User       *User     `json:"user,omitempty"`
	Created    time.Time `json:"created,omitempty"`
	LastActive time.Time `json:"last_active,omitempty"`
	Stale      bool      `json:"stale,omitempty"`
}

// LeaseConflict is one owned path that overlaps another task's lease.
type LeaseConflict struct {
	TaskID       int64     `json:"task_id"`
	Pattern      string    `json:"pattern"`
	HeldByTaskID int64     `json:"held_by_task_id"`
	HeldByUserID int64     `json:"held_by_user_id"`
	HeldPattern  string    `json:"held_pattern"`
	LastActive   time.Time `json:"last_active,omitempty"`
	Stale        bool      `json:"stale,omitempty"`
}

// ScopeCheckRequest is the body of POST /projects/{id}/scope-check.
type ScopeCheckRequest struct {
	TaskIDs    []int64  `json:"task_ids"`
	Files      []string `json:"files"`
	Repository string   `json:"repository,omitempty"`
}

// ScopeCheckFile is one file's verdict: owned, affected, unscoped or
// leased_by_other.
type ScopeCheckFile struct {
	Path         string  `json:"path"`
	Verdict      string  `json:"verdict"`
	TaskIDs      []int64 `json:"task_ids"`
	HeldByTaskID int64   `json:"held_by_task_id,omitempty"`
}

// ScopeCheckResult mirrors models.ScopeCheckResult.
type ScopeCheckResult struct {
	Enforced   bool             `json:"enforced"`
	OK         bool             `json:"ok"`
	Strays     int              `json:"strays"`
	Affected   int              `json:"affected"`
	Collisions int              `json:"collisions"`
	Files      []ScopeCheckFile `json:"files"`
}

// PlannedScope, PlannedTask and TaskPlan mirror the plan request body.
type PlannedScope struct {
	PathsOwned    []string `json:"paths_owned"`
	PathsAffected []string `json:"paths_affected"`
	Endpoints     []string `json:"endpoints"`
	Notes         string   `json:"notes"`
}

type PlannedTask struct {
	Key         string        `json:"key"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Priority    int64         `json:"priority,omitempty"`
	ParentKey   string        `json:"parent_key,omitempty"`
	BlockedBy   []string      `json:"blocked_by,omitempty"`
	Follows     []string      `json:"follows,omitempty"`
	Scope       *PlannedScope `json:"scope,omitempty"`
}

type TaskPlan struct {
	Tasks  []*PlannedTask `json:"tasks"`
	DryRun bool           `json:"dry_run,omitempty"`
}

// PlanFinding is one linter result; severity is error or warning.
type PlanFinding struct {
	Severity string   `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Keys     []string `json:"keys"`
	TaskIDs  []int64  `json:"task_ids,omitempty"`
}

type PlannedTaskResult struct {
	Key        string `json:"key"`
	ID         int64  `json:"id"`
	Index      int64  `json:"index"`
	Identifier string `json:"identifier"`
}

// PlanResult mirrors models.PlanResult.
type PlanResult struct {
	OK       bool                `json:"ok"`
	Created  bool                `json:"created"`
	Findings []PlanFinding       `json:"findings"`
	Tasks    []PlannedTaskResult `json:"tasks"`
}

// Readiness reasons the server reports; a task with none is ready.
const (
	ReadinessReasonDone          = "done"
	ReadinessReasonAssigned      = "assigned"
	ReadinessReasonBlocked       = "blocked"
	ReadinessReasonLeaseConflict = "lease_conflict"
)

// TaskReadiness is one row of the server-side ready queue.
type TaskReadiness struct {
	Task           *Task           `json:"task"`
	Ready          bool            `json:"ready"`
	Reasons        []string        `json:"reasons"`
	BlockedBy      []*Task         `json:"blocked_by"`
	LeaseConflicts []LeaseConflict `json:"lease_conflicts"`
}

// GetTaskScope reads a task's scope. A task without one yields empty lists,
// not an error.
func (c *Client) GetTaskScope(ctx context.Context, taskID int64) (*TaskScope, error) {
	var out TaskScope
	if err := c.Do(ctx, "GET", fmt.Sprintf("/tasks/%d/scope", taskID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutTaskScope replaces a task's scope wholesale. If the task already holds
// leases, the server re-leases the new paths_owned and refuses with CONFLICT
// when they overlap another task's lease.
func (c *Client) PutTaskScope(ctx context.Context, taskID int64, scope *TaskScope) (*TaskScope, error) {
	var out TaskScope
	if err := c.Do(ctx, "PUT", fmt.Sprintf("/tasks/%d/scope", taskID), nil, scope, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTaskScope removes the scope and any leases derived from it.
func (c *Client) DeleteTaskScope(ctx context.Context, taskID int64) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/tasks/%d/scope", taskID), nil, nil, nil)
}

// ListProjectLeases returns every active lease in the project, oldest first,
// with the holding task and user embedded. The endpoint is not paginated.
func (c *Client) ListProjectLeases(ctx context.Context, projectID int64) ([]*TaskPathLease, error) {
	items, _, err := doList[*TaskPathLease](ctx, c, fmt.Sprintf("/projects/%d/leases", projectID), nil)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// ReleaseTaskLeases drops every lease the task holds without touching its
// status or assignees. Idempotent.
func (c *Client) ReleaseTaskLeases(ctx context.Context, taskID int64) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/tasks/%d/leases", taskID), nil, nil, nil)
}

// ProjectReadiness returns the ready queue of one bucket of a kanban view —
// the view's default bucket when bucketID is 0. Not paginated.
func (c *Client) ProjectReadiness(ctx context.Context, projectID, viewID, bucketID int64) ([]*TaskReadiness, error) {
	q := url.Values{}
	if bucketID != 0 {
		q.Set("bucket_id", strconv.FormatInt(bucketID, 10))
	}
	items, _, err := doList[*TaskReadiness](ctx, c, fmt.Sprintf("/projects/%d/views/%d/readiness", projectID, viewID), q)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// CheckScope judges changed files against the referenced tasks' scopes and
// everyone else's leases. Read-only.
func (c *Client) CheckScope(ctx context.Context, projectID int64, req *ScopeCheckRequest) (*ScopeCheckResult, error) {
	var out ScopeCheckResult
	if err := c.Do(ctx, "POST", fmt.Sprintf("/projects/%d/scope-check", projectID), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyPlan lints a decomposition and, unless dry-run or erroneous, creates
// all of it in one transaction.
func (c *Client) ApplyPlan(ctx context.Context, projectID int64, plan *TaskPlan) (*PlanResult, error) {
	var out PlanResult
	if err := c.Do(ctx, "POST", fmt.Sprintf("/projects/%d/plan", projectID), nil, plan, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HeartbeatTaskLeases refreshes the task's leases and returns them.
func (c *Client) HeartbeatTaskLeases(ctx context.Context, taskID int64) ([]*TaskPathLease, error) {
	var env Paginated[*TaskPathLease]
	if err := c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/leases/heartbeat", taskID), nil, nil, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}
