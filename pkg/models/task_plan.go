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
	"fmt"
	"sort"
	"strings"

	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// PlannedScope is the scope part of a planned task; the same three lists
// and notes a TaskScope carries.
type PlannedScope struct {
	PathsOwned    []string `json:"paths_owned" maxItems:"100" doc:"Repository-relative globs the task will edit. Leased on claim."`
	PathsAffected []string `json:"paths_affected" maxItems:"100" doc:"Globs the task reads or depends on but does not edit."`
	Endpoints     []string `json:"endpoints" maxItems:"100" doc:"API surface the task adds or changes."`
	Notes         string   `json:"notes" doc:"Free-form scope notes, typically what is out of scope."`
}

// PlannedTask is one task of a decomposition, addressed by a key that only
// exists inside the plan so dependencies can be declared before ids exist.
type PlannedTask struct {
	Key         string        `json:"key" required:"true" minLength:"1" maxLength:"100" doc:"A name unique within the plan, used by parent_key, blocked_by and follows. Never stored."`
	Title       string        `json:"title" required:"true" minLength:"1" maxLength:"250" doc:"The task title."`
	Description string        `json:"description,omitempty" doc:"The task description, HTML like any task description."`
	Priority    int64         `json:"priority,omitempty" minimum:"0" maximum:"5" doc:"0 unset up to 5 do-now."`
	ParentKey   string        `json:"parent_key,omitempty" doc:"Key of the task this one is a subtask of."`
	BlockedBy   []string      `json:"blocked_by,omitempty" maxItems:"50" doc:"Keys of tasks that must be done before this one can be claimed."`
	Follows     []string      `json:"follows,omitempty" maxItems:"50" doc:"Keys of tasks this one comes after; treated like blocked_by for readiness."`
	Scope       *PlannedScope `json:"scope,omitempty" doc:"The task's scope. Omit for tasks that do not touch the repository."`
}

// TaskPlan is the body of POST /projects/{id}/plan.
type TaskPlan struct {
	Tasks  []*PlannedTask `json:"tasks" required:"true" minItems:"1" maxItems:"200" doc:"The tasks to create, in any order."`
	DryRun bool           `json:"dry_run,omitempty" doc:"Only lint; create nothing even when the plan is clean."`
}

// Severities and codes of plan findings.
const (
	PlanSeverityError   = "error"
	PlanSeverityWarning = "warning"

	PlanFindingDuplicateKey        = "duplicate_key"
	PlanFindingUnknownReference    = "unknown_reference"
	PlanFindingSelfReference       = "self_reference"
	PlanFindingDependencyCycle     = "dependency_cycle"
	PlanFindingParentCycle         = "parent_cycle"
	PlanFindingInvalidPath         = "invalid_path"
	PlanFindingMissingScope        = "missing_scope"
	PlanFindingOverlapWithoutOrder = "overlap_without_order"
	PlanFindingOverlapWithExisting = "overlap_with_existing"
)

// PlanFinding is one thing the linter noticed. Errors stop creation;
// warnings are reported and the plan is created anyway.
type PlanFinding struct {
	Severity string   `json:"severity" doc:"error or warning."`
	Code     string   `json:"code" doc:"Stable machine-readable code: duplicate_key, unknown_reference, self_reference, dependency_cycle, parent_cycle, invalid_path, missing_scope, overlap_without_order, overlap_with_existing."`
	Message  string   `json:"message" doc:"What is wrong, for a human or an agent to act on."`
	Keys     []string `json:"keys" doc:"The plan keys involved."`
	TaskIDs  []int64  `json:"task_ids,omitempty" doc:"Existing task ids involved, for overlap_with_existing."`
}

// PlannedTaskResult maps a plan key onto the task it became.
type PlannedTaskResult struct {
	Key        string `json:"key"`
	ID         int64  `json:"id"`
	Index      int64  `json:"index"`
	Identifier string `json:"identifier"`
}

// PlanResult is what a plan request returns: the lint findings and, when the
// plan was clean and not a dry run, the created tasks.
type PlanResult struct {
	OK       bool                `json:"ok" doc:"True when the plan has no errors. Warnings do not clear it."`
	Created  bool                `json:"created" doc:"True when the tasks were created in this request."`
	Findings []PlanFinding       `json:"findings"`
	Tasks    []PlannedTaskResult `json:"tasks" doc:"Key to id mapping of the created tasks; empty on a dry run or when the plan had errors."`
}

// ApplyTaskPlan lints a decomposition as a set — dangling references, cycles,
// paths that overlap without an ordering — and creates all of it in one go
// when it is clean: tasks, parent/blocked/follows relations and scopes. The
// caller has checked write access on the project and holds the transaction.
func ApplyTaskPlan(s *xorm.Session, a web.Auth, projectID int64, plan *TaskPlan) (*PlanResult, error) {
	findings, err := lintTaskPlan(s, projectID, plan)
	if err != nil {
		return nil, err
	}
	res := &PlanResult{Findings: findings, Tasks: []PlannedTaskResult{}}
	for _, f := range findings {
		if f.Severity == PlanSeverityError {
			return res, nil
		}
	}
	res.OK = true
	if plan.DryRun {
		return res, nil
	}

	tasks := make([]*Task, 0, len(plan.Tasks))
	byKey := map[string]*Task{}
	for _, p := range plan.Tasks {
		t := &Task{
			Title:       strings.TrimSpace(p.Title),
			Description: p.Description,
			Priority:    p.Priority,
			ProjectID:   projectID,
		}
		tasks = append(tasks, t)
		byKey[p.Key] = t
	}
	if err := createTasks(s, projectID, tasks, a, false, true); err != nil {
		return nil, err
	}

	for _, p := range plan.Tasks {
		t := byKey[p.Key]
		if p.ParentKey != "" {
			if err := (&TaskRelation{TaskID: t.ID, OtherTaskID: byKey[p.ParentKey].ID, RelationKind: RelationKindParenttask}).Create(s, a); err != nil {
				return nil, err
			}
		}
		for _, k := range p.BlockedBy {
			if err := (&TaskRelation{TaskID: t.ID, OtherTaskID: byKey[k].ID, RelationKind: RelationKindBlocked}).Create(s, a); err != nil {
				return nil, err
			}
		}
		for _, k := range p.Follows {
			if err := (&TaskRelation{TaskID: t.ID, OtherTaskID: byKey[k].ID, RelationKind: RelationKindFollows}).Create(s, a); err != nil {
				return nil, err
			}
		}
		if p.Scope != nil {
			sc := &TaskScope{
				TaskID:        t.ID,
				PathsOwned:    p.Scope.PathsOwned,
				PathsAffected: p.Scope.PathsAffected,
				Endpoints:     p.Scope.Endpoints,
				Notes:         p.Scope.Notes,
			}
			if err := sc.Update(s, a); err != nil {
				return nil, err
			}
		}
	}

	project, err := GetProjectSimpleByID(s, projectID)
	if err != nil {
		return nil, err
	}
	for _, p := range plan.Tasks {
		t := byKey[p.Key]
		res.Tasks = append(res.Tasks, PlannedTaskResult{
			Key:        p.Key,
			ID:         t.ID,
			Index:      t.Index,
			Identifier: taskIdentifier(project, t),
		})
	}
	res.Created = true
	return res, nil
}

func taskIdentifier(project *Project, t *Task) string {
	if project.Identifier == "" {
		return fmt.Sprintf("#%d", t.Index)
	}
	return fmt.Sprintf("%s-%d", project.Identifier, t.Index)
}

// lintTaskPlan checks the plan as a whole. Order of findings is stable so
// agents can diff two runs.
func lintTaskPlan(s *xorm.Session, projectID int64, plan *TaskPlan) ([]PlanFinding, error) {
	l := &planLinter{byKey: map[string]*PlannedTask{}, deps: map[string][]string{}, parents: map[string][]string{}, owned: map[string][]string{}}
	l.indexKeys(plan)
	l.checkReferences()
	l.checkCycles()
	l.checkScopes()
	l.checkOverlapsWithinPlan()
	if err := l.checkOverlapsWithBoard(s, projectID); err != nil {
		return nil, err
	}
	return l.findings, nil
}

type planLinter struct {
	findings []PlanFinding
	byKey    map[string]*PlannedTask
	order    []string
	deps     map[string][]string
	parents  map[string][]string
	owned    map[string][]string
}

func (l *planLinter) add(sev, code, msg string, keys ...string) {
	l.findings = append(l.findings, PlanFinding{Severity: sev, Code: code, Message: msg, Keys: keys})
}

func (l *planLinter) indexKeys(plan *TaskPlan) {
	for _, p := range plan.Tasks {
		p.Key = strings.TrimSpace(p.Key)
		if _, dup := l.byKey[p.Key]; dup {
			l.add(PlanSeverityError, PlanFindingDuplicateKey, fmt.Sprintf("key %q is used by more than one task", p.Key), p.Key)
			continue
		}
		l.byKey[p.Key] = p
		l.order = append(l.order, p.Key)
	}
}

// checkReferences validates every key a task points at and builds the
// dependency and parent graphs from the valid ones.
func (l *planLinter) checkReferences() {
	for _, key := range l.order {
		p := l.byKey[key]
		ordering := append(append([]string{}, p.BlockedBy...), p.Follows...)
		refs := ordering
		if p.ParentKey != "" {
			refs = append(refs, p.ParentKey)
		}
		for _, r := range refs {
			switch {
			case r == key:
				l.add(PlanSeverityError, PlanFindingSelfReference, fmt.Sprintf("%q references itself", key), key)
			case l.byKey[r] == nil:
				l.add(PlanSeverityError, PlanFindingUnknownReference, fmt.Sprintf("%q references unknown key %q", key, r), key, r)
			}
		}
		for _, r := range ordering {
			if r != key && l.byKey[r] != nil {
				l.deps[key] = append(l.deps[key], r)
			}
		}
		if p.ParentKey != "" && p.ParentKey != key && l.byKey[p.ParentKey] != nil {
			l.parents[key] = []string{p.ParentKey}
		}
	}
}

func (l *planLinter) checkCycles() {
	if cycle := findCycle(l.order, l.deps); len(cycle) > 0 {
		l.add(PlanSeverityError, PlanFindingDependencyCycle, "blocked_by/follows form a cycle: "+strings.Join(cycle, " -> "), cycle...)
	}
	if cycle := findCycle(l.order, l.parents); len(cycle) > 0 {
		l.add(PlanSeverityError, PlanFindingParentCycle, "parent_key forms a cycle: "+strings.Join(cycle, " -> "), cycle...)
	}
}

func (l *planLinter) checkScopes() {
	for _, key := range l.order {
		p := l.byKey[key]
		if p.Scope == nil || len(p.Scope.PathsOwned) == 0 {
			l.add(PlanSeverityWarning, PlanFindingMissingScope, fmt.Sprintf("%q declares no paths_owned; nothing will be leased when it is claimed", key), key)
			continue
		}
		for _, raw := range append(append([]string{}, p.Scope.PathsOwned...), p.Scope.PathsAffected...) {
			if _, err := NormalizeScopePath(raw); err != nil {
				l.add(PlanSeverityError, PlanFindingInvalidPath, fmt.Sprintf("%q: %v", key, err), key)
			}
		}
		if norm, err := normalizeScopeList(p.Scope.PathsOwned, maxScopePaths); err == nil {
			l.owned[key] = norm
		}
	}
}

// checkOverlapsWithinPlan: two tasks that edit the same files must be
// ordered, otherwise the second one is simply lease-blocked at claim time
// and nobody planned for it. Parent/child counts as ordered: a parent waits
// for its subtasks.
func (l *planLinter) checkOverlapsWithinPlan() {
	reach := transitiveClosure(l.order, mergeEdges(l.deps, l.parents))
	for i := 0; i < len(l.order); i++ {
		for j := i + 1; j < len(l.order); j++ {
			a, b := l.order[i], l.order[j]
			if reach[a][b] || reach[b][a] {
				continue
			}
			if pa, pb, ok := firstOverlap(l.owned[a], l.owned[b]); ok {
				l.add(PlanSeverityWarning, PlanFindingOverlapWithoutOrder,
					fmt.Sprintf("%q (%s) and %q (%s) own overlapping paths but neither depends on the other; whichever is claimed second will wait", a, pa, b, pb), a, b)
			}
		}
	}
}

// checkOverlapsWithBoard compares owned paths against the scopes of open
// tasks already in the project.
func (l *planLinter) checkOverlapsWithBoard(s *xorm.Session, projectID int64) error {
	existing := []*Task{}
	if err := s.Where("project_id = ? AND done = ?", projectID, false).Cols("id").Find(&existing); err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(existing))
	for _, t := range existing {
		ids = append(ids, t.ID)
	}
	scopes, err := getTaskScopesForTasks(s, ids)
	if err != nil {
		return err
	}
	existingIDs := make([]int64, 0, len(scopes))
	for id := range scopes {
		existingIDs = append(existingIDs, id)
	}
	sort.Slice(existingIDs, func(i, j int) bool { return existingIDs[i] < existingIDs[j] })
	for _, key := range l.order {
		for _, id := range existingIDs {
			if pa, pb, ok := firstOverlap(l.owned[key], scopes[id].PathsOwned); ok {
				l.findings = append(l.findings, PlanFinding{
					Severity: PlanSeverityWarning,
					Code:     PlanFindingOverlapWithExisting,
					Message:  fmt.Sprintf("%q (%s) overlaps open task %d (%s); add a dependency or narrow the scope", key, pa, id, pb),
					Keys:     []string{key},
					TaskIDs:  []int64{id},
				})
			}
		}
	}
	return nil
}

func firstOverlap(a, b []string) (string, string, bool) {
	for _, pa := range a {
		for _, pb := range b {
			if PathPatternsOverlap(pa, pb) {
				return pa, pb, true
			}
		}
	}
	return "", "", false
}

func mergeEdges(a, b map[string][]string) map[string][]string {
	out := map[string][]string{}
	for k, v := range a {
		out[k] = append(out[k], v...)
	}
	for k, v := range b {
		out[k] = append(out[k], v...)
	}
	return out
}

// findCycle returns the keys of the first cycle found by depth-first search,
// in walk order, or nil.
func findCycle(order []string, edges map[string][]string) []string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	stack := []string{}
	var cycle []string
	var visit func(string) bool
	visit = func(n string) bool {
		color[n] = grey
		stack = append(stack, n)
		for _, m := range edges[n] {
			switch color[m] {
			case grey:
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == m {
						cycle = append(append([]string{}, stack[i:]...), m)
						return true
					}
				}
			case white:
				if visit(m) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return false
	}
	for _, n := range order {
		if color[n] == white && visit(n) {
			return cycle
		}
	}
	return nil
}

func transitiveClosure(order []string, edges map[string][]string) map[string]map[string]bool {
	reach := map[string]map[string]bool{}
	for _, n := range order {
		reach[n] = map[string]bool{}
		seen := map[string]bool{}
		queue := append([]string{}, edges[n]...)
		for len(queue) > 0 {
			m := queue[0]
			queue = queue[1:]
			if seen[m] {
				continue
			}
			seen[m] = true
			reach[n][m] = true
			queue = append(queue, edges[m]...)
		}
	}
	return reach
}
