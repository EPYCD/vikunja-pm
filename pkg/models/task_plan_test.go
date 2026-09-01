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
}
