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
package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// GitPreCommitMarker identifies the hook veans wrote, so install stays
// idempotent and uninstall never touches someone else's hook.
const GitPreCommitMarker = "# veans:pre-commit"

// GitPreCommitHook runs the scope check on the index before every commit;
// a stray or a collision aborts the commit with the verdicts on stdout.
const GitPreCommitHook = `#!/usr/bin/env sh
` + GitPreCommitMarker + ` — installed by 'veans hooks install'; remove with 'veans hooks uninstall'.
# Refuses commits that touch files outside your tasks' paths_owned or files
# another in-progress task has leased. Set VEANS_SKIP_CHECK=1 to bypass once.
if [ -n "$VEANS_SKIP_CHECK" ]; then exit 0; fi
if ! command -v veans >/dev/null 2>&1; then exit 0; fi
if [ -z "$(git diff --cached --name-only --diff-filter=ACMRD)" ]; then exit 0; fi
veans check --staged
`

// ErrForeignHook is returned when a pre-commit hook exists that veans did
// not write; --force replaces it.
var ErrForeignHook = errors.New("a pre-commit hook that veans did not install already exists")

// InstallGitPreCommitHook writes .git/hooks/pre-commit in the repository
// at repoRoot. Returns the path and what happened: Wrote, Updated or
// Already installed.
func InstallGitPreCommitHook(repoRoot string, force bool) (string, string, error) {
	dir, err := gitHooksDir(repoRoot)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, "pre-commit")
	existing, err := os.ReadFile(path)
	switch {
	case err == nil && string(existing) == GitPreCommitHook:
		return path, "Already installed", nil
	case err == nil && !strings.Contains(string(existing), GitPreCommitMarker) && !force:
		return path, "", ErrForeignHook
	case err != nil && !os.IsNotExist(err):
		return path, "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return path, "", err
	}
	if err := os.WriteFile(path, []byte(GitPreCommitHook), 0o755); err != nil {
		return path, "", err
	}
	if err == nil {
		return path, "Updated", nil
	}
	return path, "Wrote", nil
}

// UninstallGitPreCommitHook removes the hook if veans wrote it.
func UninstallGitPreCommitHook(repoRoot string) (string, string, error) {
	dir, err := gitHooksDir(repoRoot)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, "pre-commit")
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return path, "Not installed", nil
	}
	if err != nil {
		return path, "", err
	}
	if !strings.Contains(string(existing), GitPreCommitMarker) {
		return path, "", ErrForeignHook
	}
	if err := os.Remove(path); err != nil {
		return path, "", err
	}
	return path, "Removed", nil
}

// gitHooksDir honours core.hooksPath-less repositories and worktrees, where
// .git is a file pointing at the real git dir.
func gitHooksDir(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "hooks"), nil
	}
	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(raw))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", errors.New(".git is neither a directory nor a gitdir pointer")
	}
	dir := strings.TrimPrefix(line, prefix)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	return filepath.Join(dir, "hooks"), nil
}
