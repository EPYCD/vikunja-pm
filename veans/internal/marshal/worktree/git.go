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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path, Branch, Head string
	Bare, Detached     bool
}

// Existing lists the worktrees attached to the repository at repoRoot.
func Existing(ctx context.Context, repoRoot string) ([]Worktree, error) {
	out, err := git(ctx, repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out), nil
}

func parseWorktrees(out string) []Worktree {
	var (
		list []Worktree
		cur  *Worktree
	)
	flush := func() {
		if cur != nil {
			list = append(list, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		if key == "worktree" {
			flush()
			cur = &Worktree{Path: val}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		}
	}
	flush()
	return list
}

// LastActivity returns the author date of the newest commit on branch, or
// the zero time when the branch does not resolve to a commit.
func LastActivity(ctx context.Context, repoRoot, branch string) (time.Time, error) {
	if _, err := git(ctx, repoRoot, "rev-parse", "--verify", "--quiet", branch+"^{commit}"); err != nil {
		// --quiet exits 1 (silently) for an unknown revision; anything else
		// is a real failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	out, err := git(ctx, repoRoot, "log", "-1", "--format=%aI", branch, "--")
	if err != nil {
		return time.Time{}, err
	}
	out = strings.TrimSpace(out)
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse author date %q: %w", out, err)
	}
	return t, nil
}

// ChangedFiles returns the paths added, copied, modified, renamed or deleted
// on branch since it diverged from base (`git diff base...branch`).
func ChangedFiles(ctx context.Context, repoRoot, base, branch string) ([]string, error) {
	out, err := git(ctx, repoRoot, "diff", "--name-only", "--diff-filter=ACMRD", "-z", base+"..."+branch, "--")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// git runs `git -C repoRoot args...` and returns stdout. GIT_* variables are
// dropped from the environment so a stray GIT_DIR/GIT_WORK_TREE cannot point
// the command at a different repository.
func git(ctx context.Context, repoRoot string, args ...string) (string, error) {
	argv := append([]string{"-C", repoRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_OPTIONAL_LOCKS=0")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
