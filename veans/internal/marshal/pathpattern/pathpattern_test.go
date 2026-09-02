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

package pathpattern

import (
	"strings"
	"testing"
)

// The cases below are the server's (pkg/models/path_pattern_test.go and
// TestPathCoveredBy in scope_check_test.go). When one side changes, the other
// must too.

func TestNormalize(t *testing.T) {
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
		got, err := Normalize(in)
		if err != nil {
			t.Errorf("Normalize(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}

	bad := []string{"", "   ", "/", ".", "./", "../etc/passwd", "pkg/../secrets", "pkg/./x", "a\nb", strings.Repeat("x", 501), "api:", "bad repo:pkg/x", "api:../x"}
	for _, in := range bad {
		if _, err := Normalize(in); err == nil {
			t.Errorf("Normalize(%q) must be rejected", in)
		}
	}
}

func TestOverlap(t *testing.T) {
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
		if !Overlap(c[0], c[1]) {
			t.Errorf("%q vs %q should overlap", c[0], c[1])
		}
		if !Overlap(c[1], c[0]) {
			t.Errorf("%q vs %q should overlap (symmetric)", c[1], c[0])
		}
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
		if Overlap(c[0], c[1]) {
			t.Errorf("%q vs %q should be disjoint", c[0], c[1])
		}
		if Overlap(c[1], c[0]) {
			t.Errorf("%q vs %q should be disjoint (symmetric)", c[1], c[0])
		}
	}
}

func TestCovers(t *testing.T) {
	yes := [][2]string{
		{"pkg/models/tasks.go", "pkg/models/tasks.go"},
		{"pkg/models", "pkg/models/tasks.go"},
		{"pkg/models/**", "pkg/models/sub/deep.go"},
		{"pkg/models/*.go", "pkg/models/tasks.go"},
		{"**/*_test.go", "pkg/models/tasks_test.go"},
		{"api:pkg/**", "api:pkg/models/tasks.go"},
	}
	for _, c := range yes {
		if !Covers(c[0], c[1]) {
			t.Errorf("%q should cover %q", c[0], c[1])
		}
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
		if Covers(c[0], c[1]) {
			t.Errorf("%q must not cover %q", c[0], c[1])
		}
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"pkg/models/*.go", "frontend/src/**", "docs/README.md"}
	hit := []string{"pkg/models/tasks.go", "frontend/src/a/b/c.vue", "docs/README.md", "docs/README.md/odd"}
	for _, f := range hit {
		if !MatchAny(patterns, f) {
			t.Errorf("MatchAny should match %q", f)
		}
	}
	miss := []string{"pkg/models/sub/deep.go", "frontend/README.md", "docs/README.mdx", "README.md"}
	for _, f := range miss {
		if MatchAny(patterns, f) {
			t.Errorf("MatchAny must not match %q", f)
		}
	}
	if MatchAny(nil, "anything") {
		t.Error("no patterns must match nothing")
	}
}

func TestSplitRepo(t *testing.T) {
	cases := []struct{ in, repo, rest string }{
		{"api:pkg/models/**", "api", "pkg/models/**"},
		{"pkg/models/**", "", "pkg/models/**"},
		{"c:program/x", "c", "program/x"},
		{":pkg", "", ":pkg"},
		{"pkg/we:ird", "", "pkg/we:ird"},
		{"*:x", "", "*:x"},
		{"bad repo:pkg/x", "bad repo", "pkg/x"},
	}
	for _, c := range cases {
		repo, rest := SplitRepo(c.in)
		if repo != c.repo || rest != c.rest {
			t.Errorf("SplitRepo(%q) = (%q, %q), want (%q, %q)", c.in, repo, rest, c.repo, c.rest)
		}
	}
}
