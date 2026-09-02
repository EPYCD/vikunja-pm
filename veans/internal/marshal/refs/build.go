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
	"context"
	"crypto/sha1" //nolint:gosec // git blob ids are sha1 by definition; provenance, not security
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const maxTextLines = 40

var headingRE = regexp.MustCompile(`^ {0,3}#{1,6}(?:\s|$)`)

// revResolver is implemented by readers bound to a git revision so Build can
// stamp Index.Rev without depending on the concrete GitReader type.
type revResolver interface {
	ResolveRev(ctx context.Context) (string, error)
}

type fileSource struct {
	src Source
	re  *regexp.Regexp
}

// Build parses all sources through the reader and returns the index. A
// missing file is an error naming the file. Duplicate anchor ids keep the
// first occurrence and are reported in the returned warnings, as are ids a
// custom pattern captured without the source prefix.
func Build(ctx context.Context, r Reader, sources []Source) (*Index, []string, error) {
	if r == nil {
		return nil, nil, errNilReader
	}
	byFile := map[string][]fileSource{}
	var order []string
	for _, s := range sources {
		re, err := compileSource(s)
		if err != nil {
			return nil, nil, err
		}
		if _, seen := byFile[s.File]; !seen {
			order = append(order, s.File)
		}
		byFile[s.File] = append(byFile[s.File], fileSource{src: s, re: re})
	}

	ix := &Index{Anchors: map[string]Anchor{}, Files: map[string]string{}}
	if rr, ok := r.(revResolver); ok {
		rev, err := rr.ResolveRev(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve revision: %w", err)
		}
		ix.Rev = rev
	}

	var warnings []string
	for _, file := range order {
		b, err := r.ReadFile(ctx, file)
		if err != nil {
			return nil, nil, fmt.Errorf("spec file %s: %w", file, err)
		}
		ix.Files[file] = gitBlobSHA(b)
		anchors, w := parseFile(file, splitLines(b), byFile[file])
		warnings = append(warnings, w...)
		for _, a := range anchors {
			if first, dup := ix.Anchors[a.ID]; dup {
				warnings = append(warnings, fmt.Sprintf("%s:%d: duplicate anchor %s (keeping %s:%d)",
					a.File, a.Line, a.ID, first.File, first.Line))
				continue
			}
			ix.Anchors[a.ID] = a
		}
	}
	return ix, warnings, nil
}

type anchorHit struct {
	line  int // 0-based
	id    string
	title string
}

func parseFile(file string, lines []string, srcs []fileSource) ([]Anchor, []string) {
	var hits []anchorHit
	isAnchor := make([]bool, len(lines))
	badPrefix := make([]int, len(srcs))
	badExample := make([]string, len(srcs))
	for i, line := range lines {
		for si, fs := range srcs {
			loc := fs.re.FindStringSubmatchIndex(line)
			if loc == nil || loc[2] < 0 {
				continue
			}
			id := line[loc[2]:loc[3]]
			if !strings.HasPrefix(id, fs.src.Prefix) {
				if badPrefix[si] == 0 {
					badExample[si] = fmt.Sprintf("%q at %s:%d", id, file, i+1)
				}
				badPrefix[si]++
				continue
			}
			isAnchor[i] = true
			hits = append(hits, anchorHit{line: i, id: id, title: cleanTitle(line[loc[3]:])})
			break
		}
	}

	var warnings []string
	for si, n := range badPrefix {
		if n > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: pattern for %s captured %d id(s) without the prefix (first: %s); ignored",
				file, srcs[si].src.Prefix, n, badExample[si]))
		}
	}

	anchors := make([]Anchor, 0, len(hits))
	for _, h := range hits {
		text := anchorText(lines, h.line, isAnchor)
		anchors = append(anchors, Anchor{
			ID:    h.id,
			File:  file,
			Line:  h.line + 1,
			Title: h.title,
			Text:  text,
			Hash:  textHash(text),
		})
	}
	return anchors, warnings
}

// anchorText collects the anchor line and what follows, stopping before the
// next anchor line, a markdown heading, or the first of two consecutive blank
// lines. The anchor line itself may be a heading, so the checks start after it.
func anchorText(lines []string, start int, isAnchor []bool) string {
	end := start + 1
	for end < len(lines) && end-start < maxTextLines {
		if isAnchor[end] || headingRE.MatchString(lines[end]) {
			break
		}
		if isBlank(lines[end]) && end+1 < len(lines) && isBlank(lines[end+1]) {
			break
		}
		end++
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

// cleanTitle strips emphasis, heading marks and separators from both ends of
// the text following an anchor id, looping because they nest ("** — **T**").
func cleanTitle(rest string) string {
	for {
		trimmed := strings.TrimSpace(rest)
		trimmed = strings.TrimLeft(trimmed, "*_ \t")
		trimmed = strings.TrimLeft(trimmed, "—–-: \t")
		trimmed = strings.TrimRight(trimmed, "*_# \t")
		if trimmed == rest {
			return trimmed
		}
		rest = trimmed
	}
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

func splitLines(b []byte) []string {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// textHash collapses all whitespace so re-wrapped or re-indented prose keeps
// its hash; only wording changes count as drift.
func textHash(text string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(sum[:])
}

// gitBlobSHA reproduces git's blob id for the bytes, so a worktree build gets
// the same provenance sha that `git rev-parse REV:path` reports for the
// committed file, without shelling out.
func gitBlobSHA(b []byte) string {
	h := sha1.New() //nolint:gosec // git blob ids are sha1 by definition; provenance, not security
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
