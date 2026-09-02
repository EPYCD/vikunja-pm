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
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInstallGitPreCommitHook_WritesIdempotentlyAndUninstalls(t *testing.T) {
	root := newRepo(t)

	path, action, err := InstallGitPreCommitHook(root, false)
	if err != nil || action != "Wrote" {
		t.Fatalf("first install: %q %v", action, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("hook must be executable, mode %v", info.Mode())
	}

	if _, action, err = InstallGitPreCommitHook(root, false); err != nil || action != "Already installed" {
		t.Fatalf("second install: %q %v", action, err)
	}

	// An older veans hook (marker present, body differs) is refreshed in place.
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+GitPreCommitMarker+" old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, action, err = InstallGitPreCommitHook(root, false); err != nil || action != "Updated" {
		t.Fatalf("refresh: %q %v", action, err)
	}

	if _, action, err = UninstallGitPreCommitHook(root); err != nil || action != "Removed" {
		t.Fatalf("uninstall: %q %v", action, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("hook should be gone: %v", err)
	}
	if _, action, err = UninstallGitPreCommitHook(root); err != nil || action != "Not installed" {
		t.Fatalf("uninstall twice: %q %v", action, err)
	}
}

func TestInstallGitPreCommitHook_LeavesForeignHooksAlone(t *testing.T) {
	root := newRepo(t)
	path := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(path, foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := InstallGitPreCommitHook(root, false); !errors.Is(err, ErrForeignHook) {
		t.Fatalf("expected ErrForeignHook, got %v", err)
	}
	if _, _, err := UninstallGitPreCommitHook(root); !errors.Is(err, ErrForeignHook) {
		t.Fatalf("uninstall must not remove a foreign hook: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(foreign) {
		t.Fatalf("foreign hook was modified: %q", got)
	}

	if _, action, err := InstallGitPreCommitHook(root, true); err != nil || action != "Updated" {
		t.Fatalf("--force: %q %v", action, err)
	}
}

func TestGitHooksDir_FollowsGitdirPointer(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "real-git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: real-git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := gitHooksDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(gitDir, "hooks") {
		t.Fatalf("got %s", dir)
	}
}
