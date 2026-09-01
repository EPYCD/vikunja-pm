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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeScopePath(t *testing.T) {
	ok := map[string]string{
		"pkg/models/tasks.go":     "pkg/models/tasks.go",
		"./pkg/models/":           "pkg/models",
		"/pkg//models//tasks.go":  "pkg/models/tasks.go",
		"  frontend/src/**  ":     "frontend/src/**",
		"pkg\\models\\tasks.go":   "pkg/models/tasks.go",
		"**":                      "**",
		"pkg/models/*_test.go":    "pkg/models/*_test.go",
		"a/b/c/d/e/f/g/h/i/j/k/l": "a/b/c/d/e/f/g/h/i/j/k/l",
		"./././pkg":               "pkg",
		"api:pkg/models/**":       "api:pkg/models/**",
		"web: ./src//App.vue":     "web:src/App.vue",
		"c:program/x":             "c:program/x",
	}
	for in, want := range ok {
		got, err := NormalizeScopePath(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}

	bad := []string{"", "   ", "/", ".", "./", "../etc/passwd", "pkg/../secrets", "pkg/./x", "a\nb", strings.Repeat("x", 501), "api:", "bad repo:pkg/x", "api:../x"}
	for _, in := range bad {
		_, err := NormalizeScopePath(in)
		require.Error(t, err, "%q must be rejected", in)
		assert.True(t, IsErrInvalidScopePath(err), "%q: got %T", in, err)
	}
}

func TestPathPatternsOverlap(t *testing.T) {
	overlap := [][2]string{
		{"api:pkg/models/**", "api:pkg/models/tasks.go"},
		{"pkg/models/tasks.go", "pkg/models/tasks.go"},
		{"pkg/models", "pkg/models/tasks.go"},
		{"pkg/models/**", "pkg/models/tasks.go"},
		{"pkg/**", "pkg/models/tasks.go"},
		{"**", "anything/at/all.go"},
		{"pkg/models/*.go", "pkg/models/tasks.go"},
		{"pkg/models/*_test.go", "pkg/models/tasks_test.go"},
		{"pkg/models/**", "pkg/models/deep/nested/file.go"},
		{"pkg/models/**", "pkg/models/*.go"},
		{"pkg/**", "pkg/models/**"},
		{"pkg/models/*.go", "pkg/models/*_test.go"},
		{"frontend/src/**/*.vue", "frontend/src/components/Foo.vue"},
		{"frontend/src/**/*.vue", "frontend/src/**"},
		{"pkg/models/ta?ks.go", "pkg/models/tasks.go"},
		{"pkg/models/[tu]asks.go", "pkg/models/tasks.go"},
	}
	for _, c := range overlap {
		assert.True(t, PathPatternsOverlap(c[0], c[1]), "%q vs %q should overlap", c[0], c[1])
		assert.True(t, PathPatternsOverlap(c[1], c[0]), "%q vs %q should overlap (symmetric)", c[1], c[0])
	}

	disjoint := [][2]string{
		{"api:pkg/models/**", "web:pkg/models/tasks.go"},
		{"api:src/index.ts", "src/index.ts"},
		{"pkg/models/tasks.go", "pkg/models/labels.go"},
		{"pkg/models/tasks.go", "pkg/models/tasks.go.bak"},
		{"pkg/models/**", "pkg/routes/**"},
		{"pkg/models/*.go", "pkg/models/deep/file.go"},
		{"pkg/models/*.go", "pkg/models/tasks.ts"},
		{"pkg/models/*_test.go", "pkg/models/tasks.go"},
		{"frontend/src/**/*.vue", "frontend/src/components/Foo.ts"},
		{"frontend/**", "pkg/**"},
		{"pkg/models/*.go", "pkg/routes/*.go"},
		{"a/b", "a/bc"},
	}
	for _, c := range disjoint {
		assert.False(t, PathPatternsOverlap(c[0], c[1]), "%q vs %q should be disjoint", c[0], c[1])
		assert.False(t, PathPatternsOverlap(c[1], c[0]), "%q vs %q should be disjoint (symmetric)", c[1], c[0])
	}
}
