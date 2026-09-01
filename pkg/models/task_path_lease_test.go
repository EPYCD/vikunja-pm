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
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leasesOf(t *testing.T, taskID int64) []*TaskPathLease {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	leases, err := getLeasesForTask(s, taskID)
	require.NoError(t, err)
	return leases
}

func TestTaskPathLease(t *testing.T) {
	user1 := &user.User{ID: 1}
	user6 := &user.User{ID: 6}

	t.Run("claiming takes the leases the scope declares", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26, ExpectedBucketID: 10})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		leases := leasesOf(t, 19)
		require.Len(t, leases, 1)
		assert.Equal(t, "pkg/models/**", leases[0].Pattern)
		assert.Equal(t, int64(10), leases[0].ProjectID)
		assert.Equal(t, int64(1), leases[0].UserID)
	})

	t.Run("a claim overlapping another task's lease is refused and rolls back", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// Task 9 owns pkg/models/*.go; task 1 already leases pkg/models/tasks.go.
		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 9, ProjectViewID: 4, BucketID: 1, ExpectedBucketID: 1})
		require.Error(t, err)
		require.True(t, IsErrPathLeaseConflict(err), "got %T: %v", err, err)
		e := err.(ErrPathLeaseConflict)
		assert.Equal(t, int64(1), e.HeldByTaskID)
		assert.Equal(t, "pkg/models/*.go", e.Pattern)
		assert.Equal(t, "pkg/models/tasks.go", e.HeldPattern)
		require.NoError(t, s.Rollback())

		s2 := db.NewSession()
		defer s2.Close()
		n, err := s2.Where("task_id = ?", 9).Count(&TaskAssginee{})
		require.NoError(t, err)
		assert.Zero(t, n, "the refused claim must not leave an assignee behind")
	})

	t.Run("leases are per project", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// Task 20 (project 11) owns the same pkg/models/** as task 19 (project 10).
		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26})
		require.NoError(t, err)
		_, err = ClaimTask(s, user1, &TaskClaim{TaskID: 20, ProjectViewID: 44, BucketID: 27})
		require.NoError(t, err, "same pattern in another project must not conflict")
		require.NoError(t, s.Commit())
	})

	t.Run("finishing the task releases its leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.Len(t, leasesOf(t, 19), 1)

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		task, err := GetTaskByIDSimple(s2, 19)
		require.NoError(t, err)
		task.Done = true
		require.NoError(t, task.Update(s2, user1))
		require.NoError(t, s2.Commit())

		assert.Empty(t, leasesOf(t, 19))
	})

	t.Run("moving into the done bucket releases its leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// Task 1 holds a fixture lease; view 4's done bucket is 3.
		tb := &TaskBucket{TaskID: 1, ProjectViewID: 4, BucketID: 3, ProjectID: 1}
		require.NoError(t, tb.Update(s, user1))
		require.NoError(t, s.Commit())

		assert.Empty(t, leasesOf(t, 1))
	})

	t.Run("deleting the task releases its leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		task := &Task{ID: 1}
		require.NoError(t, task.Delete(s, user1))
		require.NoError(t, s.Commit())

		assert.Empty(t, leasesOf(t, 1))
	})

	t.Run("explicit release drops the leases and nothing else", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		require.NoError(t, ReleaseTaskPathLeases(s, 1))
		require.NoError(t, s.Commit())
		assert.Empty(t, leasesOf(t, 1))

		// Now task 9's claim goes through.
		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		_, err := ClaimTask(s2, user1, &TaskClaim{TaskID: 9, ProjectViewID: 4, BucketID: 1, ExpectedBucketID: 1})
		require.NoError(t, err)
		require.NoError(t, s2.Commit())
		require.Len(t, leasesOf(t, 9), 1)
	})

	t.Run("re-claiming refreshes leases from the current scope", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		sc := &TaskScope{TaskID: 19, PathsOwned: []string{"pkg/routes/**"}}
		require.NoError(t, sc.Update(s2, user1))
		require.NoError(t, s2.Commit())

		leases := leasesOf(t, 19)
		require.Len(t, leases, 1)
		assert.Equal(t, "pkg/routes/**", leases[0].Pattern, "scope update on a claimed task swaps the lease")
	})

	t.Run("ReadAll lists a project's leases with holders", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		result, _, total, err := (&TaskPathLease{ProjectID: 1}).ReadAll(s, user1, "", 1, 0)
		require.NoError(t, err)
		leases, ok := result.([]*TaskPathLease)
		require.True(t, ok)
		require.EqualValues(t, 1, total)
		require.Len(t, leases, 1)
		assert.Equal(t, "pkg/models/tasks.go", leases[0].Pattern)
		require.NotNil(t, leases[0].Task)
		assert.Equal(t, int64(1), leases[0].Task.ID)
		require.NotNil(t, leases[0].User)
		assert.Equal(t, int64(1), leases[0].User.ID)
		assert.WithinDuration(t, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), leases[0].Created, 24*time.Hour)
	})

	t.Run("CanRead follows the project", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		can, _, err := (&TaskPathLease{ProjectID: 9}).CanRead(s, user1)
		require.NoError(t, err)
		assert.True(t, can, "read-only is enough to see leases")
		can, _, err = (&TaskPathLease{ProjectID: 20}).CanRead(s, user1)
		require.NoError(t, err)
		assert.False(t, can)
		can, _, err = (&TaskPathLease{ProjectID: 10}).CanRead(s, user6)
		require.NoError(t, err)
		assert.True(t, can, "owner")
	})
}

func TestClaimTask_Blocked(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("a task with an unfinished blocker cannot be claimed", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		// 19 is blocked by 20 (open) — insert the relation directly; only the
		// "blocked" direction matters to the claim.
		_, err := s.Insert(&TaskRelation{TaskID: 19, OtherTaskID: 20, RelationKind: RelationKindBlocked, CreatedByID: 1})
		require.NoError(t, err)

		_, err = ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26})
		require.Error(t, err)
		require.True(t, IsErrTaskBlocked(err), "got %T: %v", err, err)
		assert.Equal(t, []int64{20}, err.(ErrTaskBlocked).BlockerIDs)
		require.NoError(t, s.Rollback())
	})

	t.Run("a finished blocker no longer blocks", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := s.Insert(&TaskRelation{TaskID: 19, OtherTaskID: 20, RelationKind: RelationKindBlocked, CreatedByID: 1})
		require.NoError(t, err)
		_, err = s.ID(20).Cols("done").Update(&Task{Done: true})
		require.NoError(t, err)

		_, err = ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
	})
}

func TestGetTaskReadiness(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("project 1 default bucket", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		view, err := GetProjectViewByIDAndProject(s, 4, 1)
		require.NoError(t, err)
		rows, err := GetTaskReadiness(s, user1, view, 0)
		require.NoError(t, err)

		byID := map[int64]*TaskReadiness{}
		for _, r := range rows {
			byID[r.Task.ID] = r
			require.NotNil(t, r.Reasons)
			require.NotNil(t, r.BlockedBy)
			require.NotNil(t, r.LeaseConflicts)
			assert.Equal(t, len(r.Reasons) == 0, r.Ready)
		}
		// Bucket 1 of view 4 holds tasks 1, 9, 10, 11, 12 — all open.
		for _, id := range []int64{1, 9, 10, 11, 12} {
			require.Contains(t, byID, id)
		}
		nine := byID[9]
		assert.False(t, nine.Ready)
		assert.Contains(t, nine.Reasons, ReadinessReasonLeaseConflict)
		require.Len(t, nine.LeaseConflicts, 1)
		assert.Equal(t, int64(1), nine.LeaseConflicts[0].HeldByTaskID)
		// Task 10 has no scope, assignees or blockers.
		assert.True(t, byID[10].Ready, "reasons: %v", byID[10].Reasons)
	})

	t.Run("blocked and assigned reasons", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := s.Insert(&TaskRelation{TaskID: 10, OtherTaskID: 11, RelationKind: RelationKindBlocked, CreatedByID: 1})
		require.NoError(t, err)
		_, err = s.Insert(&TaskAssginee{TaskID: 11, UserID: 1})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		view, err := GetProjectViewByIDAndProject(s2, 4, 1)
		require.NoError(t, err)
		rows, err := GetTaskReadiness(s2, user1, view, 1)
		require.NoError(t, err)
		byID := map[int64]*TaskReadiness{}
		for _, r := range rows {
			byID[r.Task.ID] = r
		}
		assert.Equal(t, []string{ReadinessReasonBlocked}, byID[10].Reasons)
		require.Len(t, byID[10].BlockedBy, 1)
		assert.Equal(t, int64(11), byID[10].BlockedBy[0].ID)
		assert.Equal(t, []string{ReadinessReasonAssigned}, byID[11].Reasons)
	})

	t.Run("claimed tasks leave the queue", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())
		_, err := ClaimTask(s, user1, &TaskClaim{TaskID: 19, ProjectViewID: 40, BucketID: 26, ExpectedBucketID: 10})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		view, err := GetProjectViewByIDAndProject(s2, 40, 10)
		require.NoError(t, err)
		rows, err := GetTaskReadiness(s2, user1, view, 0)
		require.NoError(t, err)
		for _, r := range rows {
			assert.NotEqual(t, int64(19), r.Task.ID, "a claimed task moved to In Progress is no longer in the queue")
		}
	})
}
