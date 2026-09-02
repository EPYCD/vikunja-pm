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
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

// insertBot adds a bot user owned by ownerID straight into the users table;
// the fixtures carry no bots and the counts elsewhere must not shift.
func insertBot(t *testing.T, s *xorm.Session, ownerID int64) *user.User {
	t.Helper()
	bot := &user.User{Username: "bot-lease-test", Name: "lease bot", BotOwnerID: ownerID, Status: user.StatusActive}
	_, err := s.Insert(bot)
	require.NoError(t, err)
	return bot
}

// Fixture lease 1 (task 1, project 1, held by user 1) was last active on
// 2026-09-01, far beyond any threshold.
func TestLeaseStaleCron(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	t.Run("notifies the holder once", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		policeStaleLeasesAt(now)
		policeStaleLeasesAt(now.Add(time.Hour))

		s := db.NewSession()
		defer s.Close()
		got, err := notifications.GetNotificationsForNameAndUser(s, 1, (&LeaseStaleNotification{}).Name(), 1)
		require.NoError(t, err)
		assert.Len(t, got, 1, "one notification per lease, however often the cron runs")
		db.AssertExists(t, "task_path_leases", map[string]interface{}{"id": 1}, false)
	})

	t.Run("a bot holder's owner is the one told", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		setup := db.NewSession()
		bot := insertBot(t, setup, 2)
		_, err := setup.ID(1).Cols("user_id").Update(&TaskPathLease{UserID: bot.ID})
		require.NoError(t, err)
		// The cron opens its own session; the setup has to be committed for
		// it to see the bot.
		require.NoError(t, setup.Commit())
		setup.Close()

		policeStaleLeasesAt(now)

		s := db.NewSession()
		defer s.Close()
		got, err := notifications.GetNotificationsForNameAndUser(s, 2, (&LeaseStaleNotification{}).Name(), 1)
		require.NoError(t, err)
		assert.Len(t, got, 1, "owner user 2 is notified")
		none, err := notifications.GetNotificationsForNameAndUser(s, bot.ID, (&LeaseStaleNotification{}).Name(), 1)
		require.NoError(t, err)
		assert.Empty(t, none, "bots are never notified")
	})

	t.Run("auto-release is off by default", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		policeStaleLeasesAt(now)
		db.AssertExists(t, "task_path_leases", map[string]interface{}{"id": 1}, false)
	})

	t.Run("auto-release drops silent leases and says so on the task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		config.ServiceLeaseAutoReleaseAfter.Set("24h")
		defer config.ServiceLeaseAutoReleaseAfter.Set("0")

		policeStaleLeasesAt(now)

		db.AssertMissing(t, "task_path_leases", map[string]interface{}{"id": 1})
		s := db.NewSession()
		defer s.Close()
		comments := []*TaskComment{}
		require.NoError(t, s.Where("task_id = ?", 1).Find(&comments))
		found := false
		for _, c := range comments {
			if strings.Contains(c.Comment, "released automatically") {
				found = true
			}
		}
		assert.True(t, found, "the task carries the auto-release comment: %+v", comments)
	})
}

func TestGetProjectAgents(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	bot := insertBot(t, s, 1)
	_, err := s.ID(1).Cols("user_id").Update(&TaskPathLease{UserID: bot.ID})
	require.NoError(t, err)

	agents, err := GetProjectAgents(s, 1)
	require.NoError(t, err)
	require.NotEmpty(t, agents)
	assert.True(t, agents[0].User.IsBot(), "bots come first")
	assert.Equal(t, bot.ID, agents[0].User.ID)
	require.NotNil(t, agents[0].Owner)
	assert.Equal(t, int64(1), agents[0].Owner.ID)
	assert.Equal(t, 1, agents[0].Leases)
	for _, a := range agents[1:] {
		assert.False(t, a.User.IsBot())
		assert.Positive(t, a.OpenTasks+a.Leases)
	}
}
