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
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codesOf(findings []PlanFinding) []string {
	out := []string{}
	for _, f := range findings {
		out = append(out, f.Code)
	}
	return out
}

func TestApplyTaskPlan(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("lint errors stop creation", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		res, err := ApplyTaskPlan(s, user1, 1, &TaskPlan{Tasks: []*PlannedTask{
			{Key: "a", Title: "A", BlockedBy: []string{"b"}},
			{Key: "b", Title: "B", BlockedBy: []string{"a"}},
			{Key: "a", Title: "dup"},
			{Key: "c", Title: "C", ParentKey: "nope", Scope: &PlannedScope{PathsOwned: []string{"../x"}}},
		}})
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.False(t, res.Created)
		assert.Empty(t, res.Tasks)
		codes := codesOf(res.Findings)
		assert.Contains(t, codes, PlanFindingDuplicateKey)
		assert.Contains(t, codes, PlanFindingDependencyCycle)
		assert.Contains(t, codes, PlanFindingUnknownReference)
		assert.Contains(t, codes, PlanFindingInvalidPath)
		require.NoError(t, s.Commit())
		db.AssertMissing(t, "tasks", map[string]interface{}{"title": "A"})
	})

	t.Run("warnings do not stop creation", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		res, err := ApplyTaskPlan(s, user1, 1, &TaskPlan{Tasks: []*PlannedTask{
			{Key: "epic", Title: "Epic"},
			{Key: "api", Title: "API", ParentKey: "epic", Priority: 3, Scope: &PlannedScope{PathsOwned: []string{"pkg/routes/**"}, Endpoints: []string{"GET /x"}}},
			{Key: "ui", Title: "UI", ParentKey: "epic", BlockedBy: []string{"api"}, Scope: &PlannedScope{PathsOwned: []string{"frontend/**"}}},
			{Key: "docs", Title: "Docs", Follows: []string{"ui"}, Scope: &PlannedScope{PathsOwned: []string{"docs/**"}}},
			// Overlaps task 9's fixture scope (pkg/models/*.go) with no ordering.
			{Key: "models", Title: "Models", Scope: &PlannedScope{PathsOwned: []string{"pkg/models/tasks.go"}}},
			{Key: "models2", Title: "Models again", Scope: &PlannedScope{PathsOwned: []string{"pkg/models/**"}}},
		}})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		assert.True(t, res.OK)
		assert.True(t, res.Created)
		require.Len(t, res.Tasks, 6)
		codes := codesOf(res.Findings)
		assert.Contains(t, codes, PlanFindingMissingScope, "epic has no scope")
		assert.Contains(t, codes, PlanFindingOverlapWithoutOrder, "models and models2 overlap with no dependency")
		assert.Contains(t, codes, PlanFindingOverlapWithExisting, "models overlaps fixture task 9")
		assert.NotContains(t, codes, PlanFindingDependencyCycle)

		byKey := map[string]PlannedTaskResult{}
		for _, r := range res.Tasks {
			byKey[r.Key] = r
			assert.NotZero(t, r.ID)
			assert.NotZero(t, r.Index)
		}
		s2 := db.NewSession()
		defer s2.Close()
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": byKey["api"].ID, "other_task_id": byKey["epic"].ID, "relation_kind": "parenttask"}, false)
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": byKey["epic"].ID, "other_task_id": byKey["api"].ID, "relation_kind": "subtask"}, false)
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": byKey["ui"].ID, "other_task_id": byKey["api"].ID, "relation_kind": "blocked"}, false)
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": byKey["docs"].ID, "other_task_id": byKey["ui"].ID, "relation_kind": "follows"}, false)
		db.AssertExists(t, "tasks", map[string]interface{}{"id": byKey["api"].ID, "priority": 3}, false)
		sc, err := getTaskScope(s2, byKey["api"].ID)
		require.NoError(t, err)
		require.NotNil(t, sc)
		assert.Equal(t, []string{"pkg/routes/**"}, sc.PathsOwned)
		assert.Equal(t, []string{"GET /x"}, sc.Endpoints)

		// The epic is not claimable while its subtasks are open; ui waits for api.
		ids, err := unfinishedBlockers(s2, byKey["epic"].ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []int64{byKey["api"].ID, byKey["ui"].ID}, ids)
		ids, err = unfinishedBlockers(s2, byKey["ui"].ID)
		require.NoError(t, err)
		assert.Equal(t, []int64{byKey["api"].ID}, ids)
	})

	t.Run("ordered overlaps are fine and dry run creates nothing", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		res, err := ApplyTaskPlan(s, user1, 1, &TaskPlan{DryRun: true, Tasks: []*PlannedTask{
			{Key: "first", Title: "First", Scope: &PlannedScope{PathsOwned: []string{"pkg/routes/**"}}},
			{Key: "second", Title: "Second", BlockedBy: []string{"first"}, Scope: &PlannedScope{PathsOwned: []string{"pkg/routes/api/v2/x.go"}}},
		}})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		assert.True(t, res.OK)
		assert.False(t, res.Created)
		assert.NotContains(t, codesOf(res.Findings), PlanFindingOverlapWithoutOrder)
		db.AssertMissing(t, "tasks", map[string]interface{}{"title": "First"})
	})

	t.Run("references to board tasks resolve by identifier", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// Fixture project 1 is TEST1; task 1 has index 1, so TEST1-1, #1 and 1 all name it.
		res, err := ApplyTaskPlan(s, user1, 1, &TaskPlan{Tasks: []*PlannedTask{
			{Key: "follow-up", Title: "Follow up", BlockedBy: []string{"TEST1-1"}, Scope: &PlannedScope{PathsOwned: []string{"pkg/models/tasks.go"}}},
			{Key: "child", Title: "Child", ParentKey: "1"},
			{Key: "bad", Title: "Bad", BlockedBy: []string{"#99999"}},
		}})
		require.NoError(t, err)
		assert.False(t, res.OK)
		codes := codesOf(res.Findings)
		assert.Contains(t, codes, PlanFindingUnknownReference, "#99999 is nobody")
		for _, f := range res.Findings {
			if f.Code == PlanFindingOverlapWithExisting {
				assert.NotContains(t, f.TaskIDs, int64(1), "follow-up depends on task 1, so sharing its file is planned: %s", f.Message)
			}
		}

		res, err = ApplyTaskPlan(s, user1, 1, &TaskPlan{Tasks: []*PlannedTask{
			{Key: "follow-up", Title: "Follow up", BlockedBy: []string{"#1"}, Scope: &PlannedScope{PathsOwned: []string{"pkg/models/tasks.go"}}},
			{Key: "child", Title: "Child", ParentKey: "1"},
		}})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.True(t, res.OK)
		require.Len(t, res.Tasks, 2)
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": res.Tasks[0].ID, "other_task_id": 1, "relation_kind": "blocked"}, false)
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": res.Tasks[1].ID, "other_task_id": 1, "relation_kind": "parenttask"}, false)
	})
}

// Fixtures in project 1: task 1 is parent of 29 and blocks nothing; task 29
// is a subtask of 1. Task 1 has a scope (pkg/models/tasks.go).
func TestExportTaskPlan(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()

	plan, err := ExportTaskPlan(s, 1)
	require.NoError(t, err)
	require.NotEmpty(t, plan.Tasks)
	byKey := map[string]*ExportedTask{}
	for _, e := range plan.Tasks {
		byKey[e.Key] = e
		assert.NotZero(t, e.ID)
		assert.NotEmpty(t, e.Title)
	}
	one, ok := byKey["TEST1-1"]
	require.True(t, ok, "task 1 exports under its identifier")
	require.NotNil(t, one.Scope)
	assert.Equal(t, []string{"pkg/models/tasks.go"}, one.Scope.PathsOwned)
	var sub *ExportedTask
	for _, e := range plan.Tasks {
		if e.ID == 29 {
			sub = e
		}
	}
	require.NotNil(t, sub, "task 29 is open in project 1")
	assert.Equal(t, "TEST1-1", sub.ParentKey)
	_, done := byKey["TEST1-2"]
	assert.False(t, done, "done tasks are not part of the plan")

	// The export is a valid plan for a dry run once the keys are new.
	tasks := make([]*PlannedTask, 0, 2)
	for _, e := range []*ExportedTask{one, sub} {
		p := e.PlannedTask
		p.Key = "new-" + p.Key
		p.ParentKey = ""
		tasks = append(tasks, &p)
	}
	s2 := db.NewSession()
	defer s2.Close()
	res, err := ApplyTaskPlan(s2, &user.User{ID: 1}, 1, &TaskPlan{DryRun: true, Tasks: tasks})
	require.NoError(t, err)
	assert.True(t, res.OK)
}
