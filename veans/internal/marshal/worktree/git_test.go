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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /src/capyard\nHEAD 1111111111111111111111111111111111111111\nbranch refs/heads/main\n\n" +
		"worktree /src/capyard-e5.3\nHEAD 2222222222222222222222222222222222222222\nbranch refs/heads/e5.3-scheduler\nlocked\n\n" +
		"worktree /src/detached\nHEAD 3333333333333333333333333333333333333333\ndetached\n\n" +
		"worktree /src/bare.git\nbare\n"
	got := parseWorktrees(out)
	want := []Worktree{
		{Path: "/src/capyard", Head: "1111111111111111111111111111111111111111", Branch: "main"},
		{Path: "/src/capyard-e5.3", Head: "2222222222222222222222222222222222222222", Branch: "e5.3-scheduler"},
		{Path: "/src/detached", Head: "3333333333333333333333333333333333333333", Detached: true},
		{Path: "/src/bare.git", Bare: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorktrees =\n%+v\nwant\n%+v", got, want)
	}
	if got := parseWorktrees(""); len(got) != 0 {
		t.Fatalf("empty output parsed to %+v", got)
	}
}

type testRepo struct {
	t    *testing.T
	git  string
	home string
}

func newTestRepo(t *testing.T) testRepo {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	return testRepo{t: t, git: gitPath, home: t.TempDir()}
}

func (r testRepo) run(dir, date string, args ...string) {
	r.t.Helper()
	bin := r.git
	cmd := exec.CommandContext(context.Background(), bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + r.home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=veans test",
		"GIT_AUTHOR_EMAIL=veans@example.com",
		"GIT_COMMITTER_NAME=veans test",
		"GIT_COMMITTER_EMAIL=veans@example.com",
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_DATE=" + date,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r testRepo) write(path, content string) {
	r.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	rp, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return rp
}

func TestGitInspection(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	mainDate := "2024-03-04T05:06:07Z"
	featDate := "2024-03-05T06:07:08Z"

	repo := filepath.Join(r.home, "capyard")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	r.run(repo, mainDate, "init", "-q")
	r.run(repo, mainDate, "symbolic-ref", "HEAD", "refs/heads/main")
	r.write(filepath.Join(repo, "a.txt"), "a\n")
	r.run(repo, mainDate, "add", "a.txt")
	r.run(repo, mainDate, "commit", "-q", "-m", "init")

	wt := filepath.Join(r.home, "capyard-e5.3")
	r.run(repo, featDate, "worktree", "add", "-q", wt, "-b", "e5.3-scheduler")
	r.write(filepath.Join(wt, "b.txt"), "b\n")
	r.write(filepath.Join(wt, "a.txt"), "changed\n")
	r.run(wt, featDate, "add", "a.txt", "b.txt")
	r.run(wt, featDate, "commit", "-q", "-m", "scheduler")

	list, err := Existing(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	byBranch := map[string]Worktree{}
	for _, w := range list {
		byBranch[w.Branch] = w
	}
	if len(list) != 2 {
		t.Fatalf("Existing = %+v", list)
	}
	if got := byBranch["main"]; realPath(t, got.Path) != realPath(t, repo) || got.Head == "" || got.Bare || got.Detached {
		t.Fatalf("main worktree = %+v", got)
	}
	if got := byBranch["e5.3-scheduler"]; realPath(t, got.Path) != realPath(t, wt) {
		t.Fatalf("feature worktree = %+v", got)
	}

	// Existing works from a linked worktree too.
	if fromWt, err := Existing(ctx, wt); err != nil || len(fromWt) != 2 {
		t.Fatalf("Existing from worktree = %+v, %v", fromWt, err)
	}

	got, err := LastActivity(ctx, repo, "e5.3-scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := time.Parse(time.RFC3339, featDate); !got.Equal(want) {
		t.Fatalf("LastActivity(feature) = %v, want %v", got, want)
	}
	got, err = LastActivity(ctx, repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := time.Parse(time.RFC3339, mainDate); !got.Equal(want) {
		t.Fatalf("LastActivity(main) = %v, want %v", got, want)
	}
	got, err = LastActivity(ctx, repo, "does-not-exist")
	if err != nil || !got.IsZero() {
		t.Fatalf("LastActivity(missing) = %v, %v; want zero time, nil", got, err)
	}
	if _, err := LastActivity(ctx, filepath.Join(r.home, "not-a-repo"), "main"); err == nil {
		t.Fatal("expected an error outside a repository")
	}

	files, err := ChangedFiles(ctx, repo, "main", "e5.3-scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.txt", "b.txt"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("ChangedFiles = %v, want %v", files, want)
	}
	files, err = ChangedFiles(ctx, repo, "main", "main")
	if err != nil || len(files) != 0 {
		t.Fatalf("ChangedFiles(main, main) = %v, %v", files, err)
	}
	if _, err := ChangedFiles(ctx, repo, "main", "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown branch")
	}
}
