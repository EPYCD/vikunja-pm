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

// Fixture topology: task 19 lives in project 10 (owned by user 6, shared to
// user 1 with write) and sits in bucket 10 of kanban view 40, whose other
// bucket is 26. It has no assignees, no limits and no done bucket, so a claim
// exercises only the claim semantics and nothing incidental.
func TestClaimTask(t *testing.T) {
	user1 := &user.User{ID: 1}
	user6 := &user.User{ID: 6}

	claim := func(taskID, expected int64) *TaskClaim {
		return &TaskClaim{TaskID: taskID, ProjectViewID: 40, BucketID: 26, ExpectedBucketID: expected}
	}

	currentBucket := func(t *testing.T, taskID int64) int64 {
		s := db.NewSession()
		defer s.Close()
		tb := &TaskBucket{}
		_, err := s.Where("task_id = ? AND project_view_id = ?", taskID, 40).Get(tb)
		require.NoError(t, err)
		return tb.BucketID
	}

	t.Run("claims an unassigned task and moves it", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		task, err := ClaimTask(s, user1, claim(19, 10))
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		require.Len(t, task.Assignees, 1)
		assert.Equal(t, int64(1), task.Assignees[0].ID)
		assert.Equal(t, int64(26), currentBucket(t, 19))
	})

	t.Run("re-claiming your own task is a no-op", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, claim(19, 10))
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		// The expected bucket is now stale on purpose: a task the caller
		// already holds must not be refused on the guard.
		task, err := ClaimTask(s2, user1, claim(19, 10))
		require.NoError(t, err)
		require.Len(t, task.Assignees, 1)
		assert.Equal(t, int64(26), currentBucket(t, 19))
	})

	t.Run("refuses a task another user holds", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, claim(19, 10))
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		_, err = ClaimTask(s2, user6, claim(19, 0))
		require.Error(t, err)
		assert.True(t, IsErrTaskAlreadyClaimed(err), "got %T: %v", err, err)
		assert.Equal(t, int64(1), err.(ErrTaskAlreadyClaimed).UserID)
	})

	t.Run("refuses a task that left the expected bucket", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, claim(19, 26))
		require.Error(t, err)
		assert.True(t, IsErrTaskNotInExpectedBucket(err), "got %T: %v", err, err)
		e := err.(ErrTaskNotInExpectedBucket)
		assert.Equal(t, int64(10), e.BucketID)
		assert.Equal(t, int64(26), e.ExpectedBucketID)
	})

	t.Run("no guard claims regardless of bucket", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		task, err := ClaimTask(s, user1, claim(19, 0))
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.Len(t, task.Assignees, 1)
		assert.Equal(t, int64(26), currentBucket(t, 19))
	})

	t.Run("nonexistent task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, user1, claim(99999, 0))
		require.Error(t, err)
		assert.True(t, IsErrTaskDoesNotExist(err), "got %T: %v", err, err)
	})

	t.Run("link shares cannot claim", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())

		_, err := ClaimTask(s, &LinkSharing{ID: 1, ProjectID: 1}, claim(19, 0))
		require.Error(t, err)
	})
}
