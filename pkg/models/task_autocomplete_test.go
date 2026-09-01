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

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures: task 1 (project 1) is the parent of exactly one subtask, task 29,
// and holds a path lease.
func TestAutoCompleteParentTasks(t *testing.T) {
	user1 := &user.User{ID: 1}

	finishSubtask := func(t *testing.T) {
		t.Helper()
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())
		sub := &Task{ID: 29, Done: true}
		require.NoError(t, sub.updateSingleTask(s, user1, []string{"done"}))
		require.NoError(t, s.Commit())
	}

	t.Run("off by default: the parent stays open", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		finishSubtask(t)
		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": false}, false)
	})

	t.Run("on: the last subtask closes the parent and frees its leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		config.ServiceAutoCompleteParentTasks.Set(true)
		defer config.ServiceAutoCompleteParentTasks.Set(false)

		finishSubtask(t)

		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": true}, false)
		db.AssertMissing(t, "task_path_leases", map[string]interface{}{"task_id": 1})
	})

	t.Run("on: a parent with another open subtask stays open", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		config.ServiceAutoCompleteParentTasks.Set(true)
		defer config.ServiceAutoCompleteParentTasks.Set(false)

		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())
		rel := &TaskRelation{TaskID: 1, OtherTaskID: 2, RelationKind: RelationKindSubtask}
		require.NoError(t, rel.Create(s, user1))
		// Fixture task 2 is done; give the parent an open second child instead.
		rel2 := &TaskRelation{TaskID: 1, OtherTaskID: 3, RelationKind: RelationKindSubtask}
		require.NoError(t, rel2.Create(s, user1))
		require.NoError(t, s.Commit())

		finishSubtask(t)
		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": false}, false)
	})

	t.Run("readiness treats open subtasks as blockers", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		ids, err := unfinishedBlockers(s, 1)
		require.NoError(t, err)
		assert.Contains(t, ids, int64(29))
	})
}
