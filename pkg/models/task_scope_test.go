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

// Fixture topology (task_scopes.yml / task_path_leases.yml):
//   - task 19 (project 10, view 40, bucket 10) owns pkg/models/**; user1 has write.
//   - task 20 (project 11) owns the same pattern — a different project, so no conflict.
//   - task 1 (project 1) owns pkg/models/tasks.go and is already leased by user1.
//   - task 9 (project 1, view 4, bucket 1) owns pkg/models/*.go, which overlaps task 1's lease.
//   - task 18 is in project 9 where user1 is read-only; task 34 in project 20 where user1 has nothing.
func TestTaskScope(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("read of a task without a scope is empty, not an error", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		sc := &TaskScope{TaskID: 10}
		require.NoError(t, sc.ReadOne(s, user1))
		assert.Zero(t, sc.ID)
		assert.Equal(t, []string{}, sc.PathsOwned)
		assert.Equal(t, []string{}, sc.PathsAffected)
		assert.Equal(t, []string{}, sc.Endpoints)
	})

	t.Run("read returns the stored scope", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		sc := &TaskScope{TaskID: 19}
		require.NoError(t, sc.ReadOne(s, user1))
		assert.Equal(t, int64(1), sc.ID)
		assert.Equal(t, []string{"pkg/models/**"}, sc.PathsOwned)
		assert.Equal(t, []string{"pkg/routes/api/v2/tasks.go"}, sc.PathsAffected)
		assert.Equal(t, []string{"POST /api/v2/tasks/{id}/claim"}, sc.Endpoints)
	})

	t.Run("update creates and normalises", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		sc := &TaskScope{
			TaskID:        10,
			PathsOwned:    []string{"./pkg//models/", "pkg/models", "  ", "frontend\\src\\**"},
			PathsAffected: []string{"/docs/"},
			Endpoints:     []string{" GET /x ", "GET /x", ""},
			Notes:         "<p>no frontend</p>",
		}
		require.NoError(t, sc.Update(s, user1))
		require.NoError(t, s.Commit())
		assert.NotZero(t, sc.ID)
		assert.Equal(t, []string{"pkg/models", "frontend/src/**"}, sc.PathsOwned, "duplicates and blanks dropped, forms canonicalised")
		assert.Equal(t, []string{"docs"}, sc.PathsAffected)
		assert.Equal(t, []string{"GET /x"}, sc.Endpoints)

		again := &TaskScope{TaskID: 10}
		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, again.ReadOne(s2, user1))
		assert.Equal(t, sc.ID, again.ID)
		assert.Equal(t, "<p>no frontend</p>", again.Notes)
	})

	t.Run("update replaces in place", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		sc := &TaskScope{TaskID: 19, PathsOwned: []string{"pkg/routes/**"}}
		require.NoError(t, sc.Update(s, user1))
		require.NoError(t, s.Commit())
		assert.Equal(t, int64(1), sc.ID, "must update the existing row, not insert a second")

		s2 := db.NewSession()
		defer s2.Close()
		count, err := s2.Where("task_id = ?", 19).Count(&TaskScope{})
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)
	})

	t.Run("update rejects paths that escape the repository", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		sc := &TaskScope{TaskID: 10, PathsOwned: []string{"../secrets"}}
		err := sc.Update(s, user1)
		require.Error(t, err)
		assert.True(t, IsErrInvalidScopePath(err), "got %T: %v", err, err)
	})

	t.Run("permissions follow the task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		can, err := (&TaskScope{TaskID: 19}).CanUpdate(s, user1)
		require.NoError(t, err)
		assert.True(t, can, "write share on project 10")

		can, err = (&TaskScope{TaskID: 18}).CanUpdate(s, user1)
		require.NoError(t, err)
		assert.False(t, can, "read-only on project 9")

		canRead, _, err := (&TaskScope{TaskID: 18}).CanRead(s, user1)
		require.NoError(t, err)
		assert.True(t, canRead, "read-only still reads")

		canRead, _, err = (&TaskScope{TaskID: 34}).CanRead(s, user1)
		require.NoError(t, err)
		assert.False(t, canRead, "no access to project 20")

		can, err = (&TaskScope{TaskID: 34}).CanDelete(s, user1)
		require.NoError(t, err)
		assert.False(t, can)
	})

	t.Run("delete removes the scope and its leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		sc := &TaskScope{TaskID: 1}
		require.NoError(t, sc.Delete(s, user1))
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		n, err := s2.Where("task_id = ?", 1).Count(&TaskScope{})
		require.NoError(t, err)
		assert.Zero(t, n)
		n, err = s2.Where("task_id = ?", 1).Count(&TaskPathLease{})
		require.NoError(t, err)
		assert.Zero(t, n, "leases derived from the scope go with it")
	})

	t.Run("widening a claimed task's scope re-checks leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// Task 1 is claimed (leased) in project 1. Give task 9 a lease too by
		// claiming it with a non-overlapping path, then widen it onto task 1's file.
		sc := &TaskScope{TaskID: 9, PathsOwned: []string{"pkg/routes/**"}}
		require.NoError(t, sc.Update(s, user1))
		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 9, ProjectViewID: 4, BucketID: 1, ExpectedBucketID: 1})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		sc = &TaskScope{TaskID: 9, PathsOwned: []string{"pkg/routes/**", "pkg/models/tasks.go"}}
		err = sc.Update(s2, user1)
		require.Error(t, err)
		assert.True(t, IsErrPathLeaseConflict(err), "got %T: %v", err, err)
		assert.Equal(t, int64(1), err.(ErrPathLeaseConflict).HeldByTaskID)
	})
}
