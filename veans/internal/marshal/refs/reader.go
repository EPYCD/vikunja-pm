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

package refs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	errOutsideRoot = errors.New("path escapes the worktree root")
	errNoCommit    = errors.New("no commit touches path at revision")
)

type worktreeReader struct {
	root string
}

// WorktreeReader reads from a directory on disk. Paths that resolve outside
// root are rejected.
func WorktreeReader(root string) Reader {
	if root == "" {
		root = "."
	}
	return worktreeReader{root: filepath.Clean(root)}
}

func (w worktreeReader) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	full := filepath.Join(w.root, filepath.FromSlash(path))
	rel, err := filepath.Rel(w.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s: %w", path, errOutsideRoot)
	}
	b, err := os.ReadFile(full) //nolint:gosec // full is confined below root by the Rel check above
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return b, nil
}

// GitReader reads `rev:path` via `git -C root show`; it also exposes the blob
// sha and resolved commit so provenance can be stamped. An empty Rev means HEAD.
type GitReader struct {
	Root, Rev string
}

func (g GitReader) rev() string {
	if g.Rev == "" {
		return "HEAD"
	}
	return g.Rev
}

func (g GitReader) git(ctx context.Context, args ...string) (string, error) {
	root := g.Root
	if root == "" {
		root = "."
	}
	argv := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...) //nolint:gosec // fixed git verbs; root/rev/path come from the operator's own config, no shell
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

// ReadFile returns the blob at Rev:path.
func (g GitReader) ReadFile(ctx context.Context, path string) ([]byte, error) {
	out, err := g.git(ctx, "show", g.rev()+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// ResolveRev returns the full commit sha Rev points at (tags are peeled).
func (g GitReader) ResolveRev(ctx context.Context) (string, error) {
	out, err := g.git(ctx, "rev-parse", "--verify", g.rev()+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// BlobSHA returns the blob id of path at Rev (`git rev-parse Rev:path`).
func (g GitReader) BlobSHA(ctx context.Context, path string) (string, error) {
	out, err := g.git(ctx, "rev-parse", "--verify", g.rev()+":"+path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// LastCommitFor returns the most recent commit at or before Rev that touched
// path (`git log -1 --format=%H Rev -- path`).
func (g GitReader) LastCommitFor(ctx context.Context, path string) (string, error) {
	out, err := g.git(ctx, "log", "-1", "--format=%H", g.rev(), "--", path)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", fmt.Errorf("%s at %s: %w", path, g.rev(), errNoCommit)
	}
	return sha, nil
}
