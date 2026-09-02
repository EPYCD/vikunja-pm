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

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture lease 1 (task 1, user 1, project 1) was last active on 2026-09-01,
// long before any test runs, so it is stale under any positive threshold.
func TestLeaseHeartbeat(t *testing.T) {
	user1 := &user.User{ID: 1}
	user2 := &user.User{ID: 2}

	t.Run("an old lease reads as stale, a fresh one does not", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		leases, err := getLeasesForTasks(s, []int64{1})
		require.NoError(t, err)
		require.Len(t, leases[1], 1)
		assert.True(t, leases[1][0].Stale)

		fresh, err := HeartbeatTaskPathLeases(s, 1)
		require.NoError(t, err)
		require.Len(t, fresh, 1)
		assert.False(t, fresh[0].Stale)
		assert.WithinDuration(t, time.Now(), fresh[0].LastActive, time.Minute)
	})

	t.Run("threshold zero never flags", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		config.ServiceLeaseStaleAfter.Set("0")
		defer config.ServiceLeaseStaleAfter.Set("4h")
		s := db.NewSession()
		defer s.Close()

		leases, err := getLeasesForTasks(s, []int64{1})
		require.NoError(t, err)
		assert.False(t, leases[1][0].Stale)
	})

	t.Run("readiness conflicts carry the holder's staleness", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		conflicts, err := findConflictingLeases(s, 1, 9, []string{"pkg/models/*.go"})
		require.NoError(t, err)
		require.Len(t, conflicts, 1)
		assert.True(t, conflicts[0].Stale)
		assert.False(t, conflicts[0].LastActive.IsZero())
	})

	t.Run("a comment by the holder is activity, by someone else is not", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())
		require.NoError(t, (&TaskComment{TaskID: 1, Comment: "<p>still on it</p>"}).Create(s, user2))
		require.NoError(t, s.Commit())
		assert.True(t, leaseStale(t, 1), "user 2 does not hold the lease")

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, s2.Begin())
		require.NoError(t, (&TaskComment{TaskID: 1, Comment: "<p>still on it</p>"}).Create(s2, user1))
		require.NoError(t, s2.Commit())
		assert.False(t, leaseStale(t, 1))
	})

	t.Run("updating the task refreshes the holder's leases", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		require.NoError(t, s.Begin())
		task := &Task{ID: 1, Title: "renamed"}
		require.NoError(t, task.updateSingleTask(s, user1, []string{"title"}))
		require.NoError(t, s.Commit())

		assert.False(t, leaseStale(t, 1))
	})
}

func leaseStale(t *testing.T, taskID int64) bool {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	leases, err := getLeasesForTasks(s, []int64{taskID})
	require.NoError(t, err)
	require.Len(t, leases[taskID], 1)
	return leases[taskID][0].Stale
}
