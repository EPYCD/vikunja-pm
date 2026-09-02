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

package worktree

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoryID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"E5.3 — A scheduler", "E5.3"},
		{"e0.1", "E0.1"},
		{"E12", "E12"},
		{"E5.3a fix the thing", "E5.3a"},
		{"  e2.10: trailing", "E2.10"},
		{"E5.3-scheduler", "E5.3"},
		{"E5.3.", "E5.3"},
		{"CY-12", ""},
		{"scheduler", ""},
		{"E53foo", ""},
		{"E5.3abc", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := StoryID(tc.in); got != tc.want {
			t.Errorf("StoryID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"A scheduler", "scheduler"},
		{"The Big  Refactor!", "big-refactor"},
		{"Hello_World v2.0", "hello-world-v2-0"},
		{"a", "a"},
		{"The and of", "the-and-of"},
		{"", ""},
		{"Schéma naïve", "sch-ma-na-ve"},
		{"one two three four five six seven eight nine", "one-two-three-four-five-six"},
		{"abcdefghijklmnopqrstuvwxyzabcdefghij", "abcdefghijklmnopqrstuvwxyzabcdef"},
		{"abcdefghijklmnopqrstuvwxyzabcdef tail", "abcdefghijklmnopqrstuvwxyzabcdef"},
	}
	for _, tc := range tests {
		got := Slug(tc.in)
		if got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if len(got) > maxSlugLen {
			t.Errorf("Slug(%q) is %d chars", tc.in, len(got))
		}
	}
}

func TestBuild_DefaultNaming(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "capyard")

	p, err := Build(Naming{}, repo, "E5.3", "A scheduler", "CY-12", 12, "mongodb://127.0.0.1:27101/capyard_w1", 3100)
	if err != nil {
		t.Fatal(err)
	}
	if p.Story != "E5.3" || p.Slug != "scheduler" || p.Branch != "e5.3-scheduler" {
		t.Fatalf("plan = %+v", p)
	}
	if p.Dir != filepath.Join(root, "capyard-e5.3") {
		t.Fatalf("Dir = %q", p.Dir)
	}
	wantCmds := []string{
		"git worktree add ../capyard-e5.3 -b e5.3-scheduler",
		"cd ../capyard-e5.3",
		`printf '%s\n' 'MONGODB_URI=mongodb://127.0.0.1:27101/capyard_w1' 'PORT=3100' >> .env.local`,
	}
	if !reflect.DeepEqual(p.Commands, wantCmds) {
		t.Fatalf("Commands =\n%s\nwant\n%s", strings.Join(p.Commands, "\n"), strings.Join(wantCmds, "\n"))
	}
	wantEnv := []string{"MONGODB_URI=mongodb://127.0.0.1:27101/capyard_w1", "PORT=3100"}
	if !reflect.DeepEqual(p.EnvLines, wantEnv) {
		t.Fatalf("EnvLines = %v", p.EnvLines)
	}
}

func TestBuild_NoResources(t *testing.T) {
	p, err := Build(Naming{}, "/src/capyard", "E5.3", "E5.3 — A scheduler", "", 0, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "scheduler" || p.Branch != "e5.3-scheduler" {
		t.Fatalf("story prefix not stripped from title: %+v", p)
	}
	if len(p.Commands) != 2 || len(p.EnvLines) != 0 {
		t.Fatalf("expected no env command: %+v", p)
	}
	if _, err := Build(Naming{}, "/src/capyard", "E5.3", "A scheduler", "", 0, "", 3100); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_StoryFromTitle(t *testing.T) {
	p, err := Build(Naming{}, "/src/capyard", "", "e0.1 — Bootstrap the repo", "CY-1", 1, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Story != "E0.1" || p.Branch != "e0.1-bootstrap-repo" || p.Dir != "/src/capyard-e0.1" {
		t.Fatalf("plan = %+v", p)
	}
	if _, err := Build(Naming{}, "/src/capyard", "", "no story here", "CY-1", 1, "", 0); err == nil {
		t.Fatal("expected an error without a story id")
	}
}

func TestBuild_CustomNaming(t *testing.T) {
	n := Naming{Branch: "feat/{{.Identifier}}-{{.Slug}}", Dir: "/work/{{.Repo}}/{{.Story}} ({{.TaskID}})"}
	p, err := Build(n, "/src/capyard", "E5.3", "A scheduler", "CY-12", 12, "mongo://x?authSource=admin", 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Branch != "feat/CY-12-scheduler" || p.Dir != "/work/capyard/e5.3 (12)" {
		t.Fatalf("plan = %+v", p)
	}
	wantCmds := []string{
		"git worktree add '/work/capyard/e5.3 (12)' -b feat/CY-12-scheduler",
		"cd '/work/capyard/e5.3 (12)'",
		`printf '%s\n' 'MONGODB_URI=mongo://x?authSource=admin' >> .env.local`,
	}
	if !reflect.DeepEqual(p.Commands, wantCmds) {
		t.Fatalf("Commands =\n%s", strings.Join(p.Commands, "\n"))
	}

	if _, err := Build(Naming{Branch: "{{.Nope}}"}, "/src/capyard", "E5.3", "x", "", 0, "", 0); err == nil {
		t.Fatal("expected an error for an unknown template field")
	}
	if _, err := Build(Naming{Dir: "{{.Story"}, "/src/capyard", "E5.3", "x", "", 0, "", 0); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestBuild_EmptySlugKeepsBranchClean(t *testing.T) {
	p, err := Build(Naming{}, "/src/capyard", "E5.3", "", "", 0, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Branch != "e5.3" {
		t.Fatalf("Branch = %q", p.Branch)
	}
}

func TestShQuote(t *testing.T) {
	if got := shQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("shQuote = %s", got)
	}
	if got := shWord("a b"); got != "'a b'" {
		t.Fatalf("shWord = %s", got)
	}
	if got := shWord(""); got != "''" {
		t.Fatalf("shWord(empty) = %s", got)
	}
}
