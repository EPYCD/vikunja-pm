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
	"xorm.io/xorm"
)

// Fixture topology used here: project 1, kanban view 4 with buckets 1 (default),
// 2 and 3 (done). Task 9 sits in bucket 1 and owns pkg/models/*.go; task 1 is
// held by user 1 with pkg/models/tasks.go leased, which overlaps it.
func setClaimBucket(t *testing.T) {
	const viewID, bucketID = 4, 2
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	_, err := s.ID(viewID).Cols("claim_bucket_id").Update(&ProjectView{ClaimBucketID: bucketID})
	require.NoError(t, err)
	// Bucket 2 carries a limit of 3 in the fixtures; the lock must not be
	// confused with the limit.
	_, err = s.ID(bucketID).Cols("limit").Update(&Bucket{Limit: 0})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
}

func moveTask(s *xorm.Session, a *user.User, taskID, bucketID int64) error {
	tb := &TaskBucket{TaskID: taskID, ProjectViewID: 4, BucketID: bucketID, ProjectID: 1}
	return tb.Update(s, a)
}

func TestClaimOnBucketEntry(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("a drag into the claim bucket is refused while the paths are leased elsewhere", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()

		err := moveTask(s, user1, 9, 2)
		require.Error(t, err)
		require.True(t, IsErrPathLeaseConflict(err), "got %v", err)
		conflict := err.(ErrPathLeaseConflict)
		assert.Equal(t, int64(1), conflict.HeldByTaskID)
		assert.Equal(t, "user1", conflict.HeldByUsername, "the refusal names the holder")
		assert.False(t, conflict.HeldSince.IsZero(), "the refusal says since when")
		_ = s.Rollback()
		db.AssertExists(t, "task_buckets", map[string]interface{}{"task_id": 9, "bucket_id": 1}, false)
	})

	t.Run("a drag into the claim bucket leases the paths and assigns the mover", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, releasePathLeasesForTask(s, 1))

		require.NoError(t, moveTask(s, user1, 9, 2))
		require.NoError(t, s.Commit())

		db.AssertExists(t, "task_buckets", map[string]interface{}{"task_id": 9, "bucket_id": 2, "moved_by_id": 1}, false)
		db.AssertExists(t, "task_path_leases", map[string]interface{}{"task_id": 9, "user_id": 1, "pattern": "pkg/models/*.go"}, false)
		db.AssertExists(t, "task_assignees", map[string]interface{}{"task_id": 9, "user_id": 1}, false)
	})

	t.Run("a lead dragging a teammate's task leases it to the teammate", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, releasePathLeasesForTask(s, 1))
		_, err := s.Insert(&TaskAssginee{TaskID: 9, UserID: 2})
		require.NoError(t, err)

		require.NoError(t, moveTask(s, user1, 9, 2))
		require.NoError(t, s.Commit())

		db.AssertExists(t, "task_path_leases", map[string]interface{}{"task_id": 9, "user_id": 2}, false)
		db.AssertMissing(t, "task_assignees", map[string]interface{}{"task_id": 9, "user_id": 1})
	})

	t.Run("a blocked task cannot be dragged into the claim bucket", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, releasePathLeasesForTask(s, 1))
		rel := &TaskRelation{TaskID: 9, OtherTaskID: 4, RelationKind: RelationKindBlocked}
		require.NoError(t, rel.Create(s, user1))

		err := moveTask(s, user1, 9, 2)
		require.Error(t, err)
		assert.True(t, IsErrTaskBlocked(err), "got %v", err)
	})

	t.Run("the claim endpoint keeps working with a claim bucket configured", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, releasePathLeasesForTask(s, 1))

		task, err := ClaimTask(s, user1, &TaskClaim{TaskID: 9, ProjectViewID: 4, BucketID: 2, ExpectedBucketID: 1})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.Len(t, task.Assignees, 1)
		db.AssertExists(t, "task_path_leases", map[string]interface{}{"task_id": 9, "user_id": 1}, false)
	})

	t.Run("moves into other buckets are untouched", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setClaimBucket(t)
		s := db.NewSession()
		defer s.Close()

		// Bucket 1 -> bucket 1 is a no-op; task 3 sits in bucket 2, move it to 1.
		require.NoError(t, moveTask(s, user1, 3, 1))
		require.NoError(t, s.Commit())
		db.AssertMissing(t, "task_path_leases", map[string]interface{}{"task_id": 3})
	})
}

func TestBlockedRelationCycle(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("a two-node blocked cycle is refused", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, (&TaskRelation{TaskID: 1, OtherTaskID: 2, RelationKind: RelationKindBlocked}).Create(s, user1))
		err := (&TaskRelation{TaskID: 2, OtherTaskID: 1, RelationKind: RelationKindBlocked}).Create(s, user1)
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err), "got %v", err)
	})

	t.Run("a longer cycle through blocking is refused too", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, (&TaskRelation{TaskID: 1, OtherTaskID: 2, RelationKind: RelationKindBlocked}).Create(s, user1))
		require.NoError(t, (&TaskRelation{TaskID: 2, OtherTaskID: 3, RelationKind: RelationKindBlocked}).Create(s, user1))
		err := (&TaskRelation{TaskID: 1, OtherTaskID: 3, RelationKind: RelationKindBlocking}).Create(s, user1)
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err), "got %v", err)
	})

	t.Run("a chain without a cycle is fine", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, (&TaskRelation{TaskID: 1, OtherTaskID: 2, RelationKind: RelationKindBlocked}).Create(s, user1))
		require.NoError(t, (&TaskRelation{TaskID: 2, OtherTaskID: 3, RelationKind: RelationKindBlocked}).Create(s, user1))
		require.NoError(t, (&TaskRelation{TaskID: 1, OtherTaskID: 3, RelationKind: RelationKindFollows}).Create(s, user1))
	})
}
