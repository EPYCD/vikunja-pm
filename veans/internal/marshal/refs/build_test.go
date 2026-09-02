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
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestBuildAnchors(t *testing.T) {
	ix, warnings, err := Build(t.Context(), WorktreeReader("testdata"), testSources)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if ix.Rev != "" {
		t.Errorf("Rev = %q, want empty for a worktree build", ix.Rev)
	}
	for _, f := range []string{"prd.md", "spine.md", "epics.md"} {
		if sha := ix.Files[f]; len(sha) != 40 {
			t.Errorf("Files[%s] = %q, want a 40-hex blob sha", f, sha)
		}
	}

	tests := []struct {
		id    string
		file  string
		line  int
		title string
		text  string
	}{
		{
			id: "FR-161", file: "prd.md", line: 7,
			title: "An account can be created by anyone with a verified email address",
			text: "**FR-161 — An account can be created by anyone with a verified email address**\n\n" +
				"Anyone with a verified email address can create an account. The account\n" +
				"owner can change the display name at any time.\n\n" +
				"> Note: verification links expire after twenty-four hours.",
		},
		{
			id: "FR-161a", file: "prd.md", line: 14,
			title: "Account creation requires accepting the terms of service",
			text: "**FR-161a — Account creation requires accepting the terms of service**\n\n" +
				"The sign-up form blocks submission until the terms checkbox is ticked.",
		},
		{
			// Ends at the "## Files" heading.
			id: "FR-161b", file: "prd.md", line: 18,
			title: "Account names are unique",
			text: "**FR-161b — Account names are unique**\n\n" +
				"Two accounts can never share the same name, compared case-insensitively.",
		},
		{
			// Ends at two consecutive blank lines.
			id: "FR-162", file: "prd.md", line: 24,
			title: "A file belongs to exactly one work item",
			text: "**FR-162 — A file belongs to exactly one work item**\n\n" +
				"Files uploaded to a work item stay with it when the item moves between projects.\n" +
				"Deleting the work item deletes its files after a thirty day grace period.",
		},
		{
			// Ends at the NFR-N3 anchor of the other source sharing the file.
			id: "FR-163", file: "prd.md", line: 32,
			title: "Files are scanned before download",
			text: "**FR-163 — Files are scanned before download**\n\n" +
				"Every file is scanned by the configured antivirus before the first download\nis served.",
		},
		{
			id: "NFR-N3", file: "prd.md", line: 37,
			title: "Scanning finishes within five seconds",
			text: "**NFR-N3 — Scanning finishes within five seconds**\n\n" +
				"The scan of a file up to one hundred megabytes finishes within five seconds.",
		},
		{
			// Heading anchor; its own heading marks must not end its text.
			id: "AD-14", file: "spine.md", line: 5,
			title: "A work item owns files",
			text: "### AD-14 — A work item owns files\n\n" +
				"Files are stored under the work item's directory. Moving a work item\n" +
				"moves its files with it.\n\n" +
				"- Storage backend: local disk or S3.\n- Retention: thirty days after deletion.",
		},
		{
			id: "AD-15", file: "spine.md", line: 13,
			title: "Buckets are per view",
			text:  "### AD-15 — Buckets are per view\n\nEach project view carries its own bucket configuration.",
		},
		{
			id: "D-5", file: "epics.md", line: 7,
			title: "We ship onboarding before file handling, because sign-up is the",
			text:  "- D-5 We ship onboarding before file handling, because sign-up is the\n  funnel's first step.",
		},
		{
			id: "D-6", file: "epics.md", line: 9,
			title: "File scanning waits for the second release.",
			text:  "- **D-6** — File scanning waits for the second release.",
		},
	}
	if len(ix.Anchors) != len(tests) {
		t.Errorf("got %d anchors, want %d: %v", len(ix.Anchors), len(tests), ix.Anchors)
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			a, ok := ix.Anchors[tc.id]
			if !ok {
				t.Fatalf("anchor %s not found", tc.id)
			}
			if a.ID != tc.id || a.File != tc.file || a.Line != tc.line {
				t.Errorf("got %s %s:%d, want %s %s:%d", a.ID, a.File, a.Line, tc.id, tc.file, tc.line)
			}
			if a.Title != tc.title {
				t.Errorf("Title = %q, want %q", a.Title, tc.title)
			}
			if a.Text != tc.text {
				t.Errorf("Text = %q\nwant %q", a.Text, tc.text)
			}
			if len(a.Hash) != 64 || a.Hash != textHash(tc.text) {
				t.Errorf("Hash = %q, want sha256 of normalized text", a.Hash)
			}
		})
	}
}

func TestTextHashIgnoresWhitespace(t *testing.T) {
	if textHash("a  b\n c\t") != textHash("a b c") {
		t.Error("hash should not change with whitespace layout")
	}
	if textHash("a b c") == textHash("a b d") {
		t.Error("hash should change with wording")
	}
}

func TestBuildTextCappedAt40Lines(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("**FR-1 — long**\n")
	for i := range 60 {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	ix, _, err := Build(t.Context(), memReader{"prd.md": sb.String()}, []Source{{Prefix: "FR-", File: "prd.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(ix.Anchors["FR-1"].Text, "\n")); n != 40 {
		t.Errorf("Text has %d lines, want 40", n)
	}
}

func TestBuildDuplicateWarns(t *testing.T) {
	tests := []struct {
		name    string
		files   memReader
		sources []Source
		wantMsg string
	}{
		{
			name:    "within a file",
			files:   memReader{"prd.md": "**FR-1 — first**\n\ntext one\n\n**FR-1 — second**\n\ntext two\n"},
			sources: []Source{{Prefix: "FR-", File: "prd.md"}},
			wantMsg: "prd.md:5: duplicate anchor FR-1 (keeping prd.md:1)",
		},
		{
			name:    "across files",
			files:   memReader{"prd.md": "**FR-1 — first**\n", "other.md": "**FR-1 — second**\n"},
			sources: []Source{{Prefix: "FR-", File: "prd.md"}, {Prefix: "FR-", File: "other.md"}},
			wantMsg: "other.md:1: duplicate anchor FR-1 (keeping prd.md:1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ix, warnings, err := Build(t.Context(), tc.files, tc.sources)
			if err != nil {
				t.Fatal(err)
			}
			if a := ix.Anchors["FR-1"]; a.Title != "first" || a.File != "prd.md" {
				t.Errorf("kept %+v, want the first definition", a)
			}
			if len(warnings) != 1 || warnings[0] != tc.wantMsg {
				t.Errorf("warnings = %q, want [%q]", warnings, tc.wantMsg)
			}
		})
	}
}

func TestBuildMissingFile(t *testing.T) {
	_, _, err := Build(t.Context(), memReader{}, []Source{{Prefix: "FR-", File: "docs/prd.md"}})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "docs/prd.md") || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error %q should name the file and wrap fs.ErrNotExist", err)
	}
}

func TestBuildRejectsBadSources(t *testing.T) {
	tests := []struct {
		name    string
		source  Source
		wantErr string
	}{
		{"no prefix", Source{File: "prd.md"}, "prefix is required"},
		{"no file", Source{Prefix: "FR-"}, "file is required"},
		{"no capture group", Source{Prefix: "FR-", File: "prd.md", Pattern: `^FR-\d+`}, "capture group"},
		{"bad regexp", Source{Prefix: "FR-", File: "prd.md", Pattern: `(`}, "error parsing regexp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Build(t.Context(), memReader{"prd.md": ""}, []Source{tc.source})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildCustomPattern(t *testing.T) {
	ix, warnings, err := Build(t.Context(), WorktreeReader("testdata"),
		[]Source{{Prefix: "AD-", File: "spine.md", Pattern: `^### (AD-\d+)`}})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(ix.Anchors) != 2 || ix.Anchors["AD-14"].Title != "A work item owns files" {
		t.Errorf("got anchors %v warnings %v", ix.Anchors, warnings)
	}

	// A capture without the prefix is misconfiguration: ignored, warned once.
	ix, warnings, err = Build(t.Context(), WorktreeReader("testdata"),
		[]Source{{Prefix: "AD-", File: "spine.md", Pattern: `^### AD-(\d+)`}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Anchors) != 0 {
		t.Errorf("anchors without the prefix must be ignored, got %v", ix.Anchors)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `2 id(s) without the prefix (first: "14" at spine.md:5)`) {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestDefaultPattern(t *testing.T) {
	tests := []struct {
		line   string
		prefix string
		want   string
	}{
		{"**FR-161 — An account can be created**", "FR-", "FR-161"},
		{"**FR-161a — with suffix**", "FR-", "FR-161a"},
		{"### AD-14 — A work item owns files", "AD-", "AD-14"},
		{"- **D-5** decided", "D-", "D-5"},
		{"NFR-N3 bare", "NFR-", "NFR-N3"},
		{"> **FR-2** quoted", "FR-", "FR-2"},
		{"  * _FR-3_ indented", "FR-", "FR-3"},
		{"See FR-161 inline", "FR-", ""},
		{"**NFR-N3 — other family**", "FR-", ""},
		{"**FR-161abc**", "FR-", ""},
		{"fr-1 lowercase", "FR-", ""},
		{"AD-14 is not a D- anchor", "D-", ""},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			re, err := compileSource(Source{Prefix: tc.prefix, File: "x.md"})
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if m := re.FindStringSubmatch(tc.line); m != nil {
				got = m[1]
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
