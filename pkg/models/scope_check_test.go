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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathCoveredBy(t *testing.T) {
	yes := [][2]string{
		{"pkg/models/tasks.go", "pkg/models/tasks.go"},
		{"pkg/models", "pkg/models/tasks.go"},
		{"pkg/models/**", "pkg/models/sub/deep.go"},
		{"pkg/models/*.go", "pkg/models/tasks.go"},
		{"**/*_test.go", "pkg/models/tasks_test.go"},
		{"api:pkg/**", "api:pkg/models/tasks.go"},
	}
	for _, c := range yes {
		assert.True(t, PathCoveredBy(c[0], c[1]), "%q should cover %q", c[0], c[1])
	}
	no := [][2]string{
		{"pkg/models/tasks.go", "pkg/models/tasks.go.bak"},
		{"pkg/models/*.go", "pkg/models/sub/deep.go"},
		{"pkg/routes/**", "pkg/models/tasks.go"},
		{"api:pkg/**", "web:pkg/models/tasks.go"},
		{"pkg/**", "api:pkg/models/tasks.go"},
		{"pkg/models/tasks.go", "pkg/models"},
	}
	for _, c := range no {
		assert.False(t, PathCoveredBy(c[0], c[1]), "%q must not cover %q", c[0], c[1])
	}
}

// Fixtures: task 9 (project 1) owns pkg/models/*.go; task 1 (project 1) holds
// a lease on pkg/models/tasks.go; task 19 (project 10) owns pkg/models/**
// and affects pkg/routes/api/v2/tasks.go.
func TestCheckScope(t *testing.T) {
	t.Run("owned, affected, stray and collision", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		res, err := CheckScope(s, 10, &ScopeCheckRequest{
			TaskIDs: []int64{19},
			Files:   []string{"pkg/models/scope.go", "pkg/routes/api/v2/tasks.go", "frontend/src/App.vue"},
		})
		require.NoError(t, err)
		assert.True(t, res.Enforced)
		assert.False(t, res.OK)
		assert.Equal(t, 1, res.Strays)
		assert.Equal(t, 1, res.Affected)
		assert.Equal(t, 0, res.Collisions)
		assert.Equal(t, ScopeVerdictOwned, res.Files[0].Verdict)
		assert.Equal(t, []int64{19}, res.Files[0].TaskIDs)
		assert.Equal(t, ScopeVerdictAffected, res.Files[1].Verdict)
		assert.Equal(t, ScopeVerdictUnscoped, res.Files[2].Verdict)
	})

	t.Run("a file leased by an unreferenced task is a collision", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		res, err := CheckScope(s, 1, &ScopeCheckRequest{
			TaskIDs: []int64{9},
			Files:   []string{"pkg/models/tasks.go", "pkg/models/other.go"},
		})
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.Equal(t, 1, res.Collisions)
		assert.Equal(t, ScopeVerdictLeasedByOther, res.Files[0].Verdict)
		assert.Equal(t, int64(1), res.Files[0].HeldByTaskID)
		assert.Equal(t, ScopeVerdictOwned, res.Files[1].Verdict)

		// Referencing the holder makes the same file fine.
		res, err = CheckScope(s, 1, &ScopeCheckRequest{TaskIDs: []int64{1, 9}, Files: []string{"pkg/models/tasks.go"}})
		require.NoError(t, err)
		assert.True(t, res.OK)
		assert.Equal(t, ScopeVerdictOwned, res.Files[0].Verdict)
	})

	t.Run("no declared scope means nothing is enforced", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		res, err := CheckScope(s, 10, &ScopeCheckRequest{TaskIDs: []int64{21}, Files: []string{"anything.go"}})
		require.NoError(t, err)
		assert.False(t, res.Enforced)
		assert.True(t, res.OK)
		assert.Equal(t, 0, res.Strays)
		assert.Equal(t, ScopeVerdictUnscoped, res.Files[0].Verdict)
	})

	t.Run("a task from another project lends no scope", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		res, err := CheckScope(s, 1, &ScopeCheckRequest{TaskIDs: []int64{19}, Files: []string{"pkg/models/x.go"}})
		require.NoError(t, err)
		assert.False(t, res.Enforced)
	})

	t.Run("repository prefix namespaces the files", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		res, err := CheckScope(s, 10, &ScopeCheckRequest{TaskIDs: []int64{19}, Files: []string{"pkg/models/x.go"}, Repository: "web"})
		require.NoError(t, err)
		assert.Equal(t, "web:pkg/models/x.go", res.Files[0].Path)
		assert.Equal(t, ScopeVerdictUnscoped, res.Files[0].Verdict, "task 19 owns the bare pkg/models/**, not web:'s")
	})
}
