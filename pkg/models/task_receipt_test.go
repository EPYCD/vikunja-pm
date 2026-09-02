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

// receiptProject makes the CI bot the receipt bot of project 1 and returns it.
func receiptProject(t *testing.T) *user.User {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	bot := insertBot(t, s, 1)
	_, err := s.ID(1).Cols("receipt_bot_id").Update(&Project{ReceiptBotID: bot.ID})
	require.NoError(t, err)
	// CI closes tasks, so it needs write access like any other member.
	_, err = s.Insert(&ProjectUser{ProjectID: 1, UserID: bot.ID, Permission: PermissionWrite})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return bot
}

func greenGates() []GateResult {
	return []GateResult{
		{Name: "typecheck", Status: GateStatusPassed, DurationMS: 1200},
		{Name: "lint", Status: GateStatusPassed, DurationMS: 800},
		{Name: "test", Status: GateStatusPassed, DurationMS: 30000},
		{Name: "build", Status: GateStatusPassed, DurationMS: 45000},
	}
}

func TestTaskReceipt(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("only the receipt bot may post", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		bot := receiptProject(t)
		s := db.NewSession()
		defer s.Close()

		r := &TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Gates: greenGates()}
		can, err := r.CanCreate(s, user1)
		require.NoError(t, err)
		assert.False(t, can, "the owner is not CI")

		other := &user.User{Username: "bot-other", BotOwnerID: 1, Status: user.StatusActive}
		_, err = s.Insert(other)
		require.NoError(t, err)
		can, err = r.CanCreate(s, other)
		require.NoError(t, err)
		assert.False(t, can, "another bot is not CI either")

		can, err = r.CanCreate(s, bot)
		require.NoError(t, err)
		assert.True(t, can)
	})

	t.Run("nobody may post when the project names no receipt bot", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		can, err := (&TaskReceipt{TaskID: 1}).CanCreate(s, user1)
		require.NoError(t, err)
		assert.False(t, can)
	})

	t.Run("create computes passed and lists newest first", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		bot := receiptProject(t)
		s := db.NewSession()
		defer s.Close()

		red := &TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Branch: "e5.3-scheduler", Gates: greenGates(), DocsAPIRequired: true}
		require.NoError(t, red.Create(s, bot))
		assert.False(t, red.Passed, "docs:api required but not regenerated")
		assert.Equal(t, int64(1), red.ProjectID)

		green := &TaskReceipt{TaskID: 1, CommitSHA: "def5678", Gates: greenGates(), DocsAPIRequired: true, DocsAPIRegenerated: true, Merged: true, MergeSHA: "0123abcd"}
		require.NoError(t, green.Create(s, bot))
		assert.True(t, green.Passed)

		failed := &TaskReceipt{TaskID: 1, CommitSHA: "fed9876", Gates: []GateResult{{Name: "test", Status: GateStatusFailed, DurationMS: 10}}}
		require.NoError(t, failed.Create(s, bot))
		assert.False(t, failed.Passed)

		res, count, total, err := (&TaskReceipt{TaskID: 1}).ReadAll(s, user1, "", 1, 50)
		require.NoError(t, err)
		receipts := res.([]*TaskReceipt)
		assert.Equal(t, 3, count)
		assert.Equal(t, int64(3), total)
		assert.Equal(t, "fed9876", receipts[0].CommitSHA, "newest first")
		require.NotNil(t, receipts[0].PostedBy)
		assert.Equal(t, bot.ID, receipts[0].PostedBy.ID)
	})

	t.Run("malformed receipts are refused", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		bot := receiptProject(t)
		s := db.NewSession()
		defer s.Close()

		err := (&TaskReceipt{TaskID: 1, CommitSHA: "not-a-sha", Gates: greenGates()}).Create(s, bot)
		assert.True(t, IsErrReceiptInvalid(err), "got %v", err)
		err = (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Gates: []GateResult{{Name: "test", Status: "green"}}}).Create(s, bot)
		assert.True(t, IsErrReceiptInvalid(err), "got %v", err)
		err = (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234"}).Create(s, bot)
		assert.True(t, IsErrReceiptInvalid(err), "got %v", err)
	})
}

func TestDoneRequiresReceipt(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("without a receipt bot nothing changes", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		task := &Task{ID: 1, Done: true}
		require.NoError(t, task.Update(s, user1))
	})

	t.Run("done is refused until CI posts a merged, passing receipt", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		bot := receiptProject(t)
		s := db.NewSession()
		defer s.Close()

		err := (&Task{ID: 1, Done: true}).Update(s, user1)
		require.Error(t, err)
		assert.True(t, IsErrTaskDoneRequiresReceipt(err), "got %v", err)
		_ = s.Rollback()

		s2 := db.NewSession()
		defer s2.Close()
		// A passing but unmerged receipt is not enough...
		require.NoError(t, (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Gates: greenGates()}).Create(s2, bot))
		err = (&Task{ID: 1, Done: true}).Update(s2, user1)
		assert.True(t, IsErrTaskDoneRequiresReceipt(err), "got %v", err)
		_ = s2.Rollback()

		s3 := db.NewSession()
		defer s3.Close()
		// ...nor a merged but failing one.
		require.NoError(t, (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Merged: true, Gates: []GateResult{{Name: "test", Status: GateStatusFailed}}}).Create(s3, bot))
		err = (&Task{ID: 1, Done: true}).Update(s3, user1)
		assert.True(t, IsErrTaskDoneRequiresReceipt(err), "got %v", err)
		_ = s3.Rollback()

		s4 := db.NewSession()
		defer s4.Close()
		require.NoError(t, (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Merged: true, Gates: greenGates()}).Create(s4, bot))
		require.NoError(t, (&Task{ID: 1, Done: true}).Update(s4, user1))
		require.NoError(t, s4.Commit())
		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": true}, false)
	})

	t.Run("the user who submitted for review cannot close, CI can", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		bot := receiptProject(t)
		setup := db.NewSession()
		require.NoError(t, (&TaskReceipt{TaskID: 1, CommitSHA: "abc1234", Merged: true, Gates: greenGates()}).Create(setup, bot))
		// User 1 moved task 1 into its current bucket (fixtures leave the column null).
		_, err := setup.Where("task_id = ? AND project_view_id = ?", 1, 4).Cols("moved_by_id").Update(&TaskBucket{MovedByID: 1})
		require.NoError(t, err)
		require.NoError(t, setup.Commit())
		setup.Close()

		s := db.NewSession()
		defer s.Close()
		err = (&Task{ID: 1, Done: true}).Update(s, user1)
		require.Error(t, err)
		assert.True(t, IsErrTaskDoneBySubmitter(err), "got %v", err)
		_ = s.Rollback()

		s2 := db.NewSession()
		defer s2.Close()
		// The same rule holds for a drag into the done bucket.
		err = moveTask(s2, user1, 1, 3)
		require.Error(t, err)
		assert.True(t, IsErrTaskDoneBySubmitter(err), "got %v", err)
		_ = s2.Rollback()

		s3 := db.NewSession()
		defer s3.Close()
		require.NoError(t, (&Task{ID: 1, Done: true}).Update(s3, bot))
		require.NoError(t, s3.Commit())
		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": true}, false)
	})
}

func TestProjectReceiptBot(t *testing.T) {
	user1 := &user.User{ID: 1}

	t.Run("must be an existing bot", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		p := &Project{ID: 1, Title: "Test1", ReceiptBotID: 2}
		err := UpdateProject(s, p, user1, false)
		assert.True(t, IsErrReceiptBotInvalid(err), "a human cannot be CI: %v", err)
		p.ReceiptBotID = 999999
		err = UpdateProject(s, p, user1, false)
		assert.True(t, IsErrReceiptBotInvalid(err), "got %v", err)
	})

	t.Run("an admin can set and clear it", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		bot := insertBot(t, s, 1)
		p := &Project{ID: 1, Title: "Test1", ReceiptBotID: bot.ID}
		require.NoError(t, UpdateProject(s, p, user1, false))
		require.NoError(t, s.Commit())
		db.AssertExists(t, "projects", map[string]interface{}{"id": 1, "receipt_bot_id": bot.ID}, false)

		s2 := db.NewSession()
		defer s2.Close()
		require.NoError(t, UpdateProject(s2, &Project{ID: 1, Title: "Test1"}, user1, false))
		require.NoError(t, s2.Commit())
		db.AssertExists(t, "projects", map[string]interface{}{"id": 1, "receipt_bot_id": 0}, false)
	})
}
