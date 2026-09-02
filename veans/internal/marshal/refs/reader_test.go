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
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWorktreeReader(t *testing.T) {
	r := WorktreeReader("testdata")
	got, err := r.ReadFile(t.Context(), "prd.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, readFixture(t, "prd.md")) {
		t.Error("ReadFile returned different bytes than the fixture")
	}

	if _, err := r.ReadFile(t.Context(), "../reader.go"); !errors.Is(err, errOutsideRoot) {
		t.Errorf("escaping path: err = %v, want errOutsideRoot", err)
	}
	if _, err := r.ReadFile(t.Context(), "missing.md"); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("missing file: err = %v, want fs.ErrNotExist naming the file", err)
	}
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestGitReader(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ctx := t.Context()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // test drives git with fixed verbs
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=refs test", "GIT_AUTHOR_EMAIL=refs@example.com",
			"GIT_COMMITTER_NAME=refs test", "GIT_COMMITTER_EMAIL=refs@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	prd := readFixture(t, "prd.md")
	spine := readFixture(t, "spine.md")
	git("init", "-q")
	write("prd.md", prd)
	write("spine.md", spine)
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "one")
	first := git("rev-parse", "HEAD")

	spine2 := append(append([]byte{}, spine...), "\nAmended later.\n"...)
	write("spine.md", spine2)
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "two")
	second := git("rev-parse", "HEAD")

	head := GitReader{Root: dir} // empty Rev means HEAD
	got, err := head.ReadFile(ctx, "prd.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, prd) {
		t.Error("ReadFile at HEAD returned different bytes than committed")
	}
	if rev, err := head.ResolveRev(ctx); err != nil || rev != second {
		t.Errorf("ResolveRev = %q, %v; want %q", rev, err, second)
	}
	blob, err := head.BlobSHA(ctx, "prd.md")
	if err != nil {
		t.Fatal(err)
	}
	if !shaRE.MatchString(blob) || blob != gitBlobSHA(prd) {
		t.Errorf("BlobSHA = %q, want git's blob id %q", blob, gitBlobSHA(prd))
	}
	if last, err := head.LastCommitFor(ctx, "prd.md"); err != nil || last != first {
		t.Errorf("LastCommitFor(prd.md) = %q, %v; want first commit %q", last, err, first)
	}
	if last, err := head.LastCommitFor(ctx, "spine.md"); err != nil || last != second {
		t.Errorf("LastCommitFor(spine.md) = %q, %v; want second commit %q", last, err, second)
	}
	if _, err := head.ReadFile(ctx, "missing.md"); err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("missing file: err = %v, want one naming the file", err)
	}
	if _, err := head.LastCommitFor(ctx, "missing.md"); !errors.Is(err, errNoCommit) {
		t.Errorf("LastCommitFor(missing.md) = %v, want errNoCommit", err)
	}

	pinned := GitReader{Root: dir, Rev: first}
	if got, err := pinned.ReadFile(ctx, "spine.md"); err != nil || !bytes.Equal(got, spine) {
		t.Errorf("ReadFile at first commit = %q, %v; want the original spine", got, err)
	}
	if _, err := (GitReader{Root: dir, Rev: "nope"}).ResolveRev(ctx); err == nil {
		t.Error("ResolveRev of an unknown revision should fail")
	}

	ix, warnings, err := Build(ctx, head, []Source{{Prefix: "FR-", File: "prd.md"}, {Prefix: "AD-", File: "spine.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || ix.Rev != second || ix.Files["prd.md"] != blob || ix.Anchors["FR-161"].Line != 7 {
		t.Errorf("Build via git: rev %q files %v warnings %v", ix.Rev, ix.Files, warnings)
	}
	res := ix.Resolve([]Ref{{ID: "FR-161", Prefix: "FR-"}})
	if want := "prd.md@" + second[:12]; res[0].Provenance != want {
		t.Errorf("Provenance = %q, want %q", res[0].Provenance, want)
	}
}
