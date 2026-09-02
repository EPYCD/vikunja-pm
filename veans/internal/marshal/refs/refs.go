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

// Package refs implements the drift-sentinel primitives: it indexes anchors
// such as FR-161 or AD-14 in spec files, finds references to them in task
// text, resolves those references against an index built at a git revision,
// diffs two indexes to surface drift, and detects verbatim pastes of spec
// prose. It is a pure library with no CLI, HTTP, or board access.
package refs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source is one configured anchor family: where references of a prefix resolve.
type Source struct {
	// Prefix like "FR-", "AD-", "NFR-", "D-". Matching is case-sensitive.
	Prefix string `yaml:"prefix"`
	// File is repo-relative (forward slashes).
	File string `yaml:"file"`
	// Pattern is a Go regexp applied to each line; its first capture group is
	// the anchor id (e.g. "FR-161a"). Optional: when empty a default is used
	// that matches the id at the start of a line optionally wrapped in markdown
	// emphasis or headings, e.g. `**FR-161 — …**`, `### AD-14 — …`, `- D-5 …`.
	Pattern string `yaml:"pattern,omitempty"`
}

// Anchor is one resolvable location in a spec file.
type Anchor struct {
	ID    string // "FR-161a"
	File  string // repo-relative
	Line  int    // 1-based line of the anchor
	Title string // rest of the anchor line after the id, without emphasis/heading marks and leading separators
	Text  string // anchor line plus following lines up to the next anchor, heading, or two blank lines; capped at 40 lines
	Hash  string // sha256 hex of Text with all whitespace collapsed
}

// Index is every anchor of every source at one revision of the repository.
type Index struct {
	Rev     string            // git revision the index was built from ("" when built from the working tree)
	Anchors map[string]Anchor // by ID
	Files   map[string]string // file -> blob sha ("" when unknown)
}

// Reader returns the bytes of a repo-relative file at the index's revision.
type Reader interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
}

// Ref is a reference found in free text.
type Ref struct {
	ID     string // "FR-161"
	Prefix string
	Offset int // byte offset into the text passed to Extract, before HTML stripping
}

// Resolution is what a task's reference renders as.
type Resolution struct {
	Ref        Ref
	Found      bool
	Anchor     Anchor // zero when !Found
	Provenance string // "prd.md@<12-char sha>" from Index.Rev, else Index.Files[file]; "" when neither is known
}

// Drift lists how anchors moved between two indexes.
type Drift struct {
	Changed  []string // ids present in both with different Hash
	Vanished []string // ids in old not in new
	Appeared []string // ids in new not in old
}

// idBody is the part of an id after the prefix: "161", "161a", "N3".
const idBody = `[A-Z]?\d+[a-z]?`

// leadingMarks are the markdown decorations tolerated before an anchor id at
// the start of a line: headings, bullets, blockquotes, emphasis, whitespace.
const leadingMarks = `^[\s#*_>+-]*`

// idEnd is a word boundary that, unlike \b, lets "_" close an id so
// underscore emphasis (`_FR-3_`) works like asterisks do.
const idEnd = `(?:[^0-9A-Za-z]|$)`

const provenanceLen = 12

var (
	errNoPrefix       = errors.New("prefix is required")
	errNoFile         = errors.New("file is required")
	errNoCaptureGroup = errors.New("pattern needs a capture group for the anchor id")
	errNilReader      = errors.New("nil reader")
)

func defaultPattern(prefix string) string {
	return leadingMarks + `(` + regexp.QuoteMeta(prefix) + idBody + `)` + idEnd
}

func compileSource(s Source) (*regexp.Regexp, error) {
	if s.Prefix == "" {
		return nil, errNoPrefix
	}
	if s.File == "" {
		return nil, fmt.Errorf("source %s: %w", s.Prefix, errNoFile)
	}
	pat := s.Pattern
	if pat == "" {
		pat = defaultPattern(s.Prefix)
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", s.Prefix, err)
	}
	if re.NumSubexp() == 0 {
		return nil, fmt.Errorf("source %s: %w", s.Prefix, errNoCaptureGroup)
	}
	return re, nil
}

// LoadSources reads a YAML list of Source from bytes (used by the caller's
// config). Unknown keys and unusable patterns are errors; empty input yields
// no sources.
func LoadSources(b []byte) ([]Source, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var sources []Source
	if err := dec.Decode(&sources); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("parse sources: %w", err)
	}
	for i := range sources {
		sources[i].File = strings.TrimPrefix(sources[i].File, "./")
		if _, err := compileSource(sources[i]); err != nil {
			return nil, fmt.Errorf("sources[%d]: %w", i, err)
		}
	}
	return sources, nil
}

// Resolve looks each ref up in the index. A nil index resolves nothing.
func (ix *Index) Resolve(refs []Ref) []Resolution {
	out := make([]Resolution, 0, len(refs))
	for _, ref := range refs {
		res := Resolution{Ref: ref}
		if ix != nil {
			if a, ok := ix.Anchors[ref.ID]; ok {
				res.Found = true
				res.Anchor = a
				res.Provenance = ix.provenance(a.File)
			}
		}
		out = append(out, res)
	}
	return out
}

func (ix *Index) provenance(file string) string {
	sha := ix.Rev
	if sha == "" {
		sha = ix.Files[file]
	}
	if sha == "" {
		return ""
	}
	return file + "@" + shortSHA(sha)
}

func shortSHA(sha string) string {
	if len(sha) > provenanceLen {
		return sha[:provenanceLen]
	}
	return sha
}

// Diff compares two indexes and lists anchors whose Text hash changed,
// anchors that vanished, and anchors that appeared. Each list is sorted by id.
func Diff(prev, next *Index) Drift {
	var d Drift
	pa, na := anchorsOf(prev), anchorsOf(next)
	for _, id := range slices.Sorted(maps.Keys(pa)) {
		n, ok := na[id]
		switch {
		case !ok:
			d.Vanished = append(d.Vanished, id)
		case n.Hash != pa[id].Hash:
			d.Changed = append(d.Changed, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(na)) {
		if _, ok := pa[id]; !ok {
			d.Appeared = append(d.Appeared, id)
		}
	}
	return d
}

func anchorsOf(ix *Index) map[string]Anchor {
	if ix == nil {
		return nil
	}
	return ix.Anchors
}
