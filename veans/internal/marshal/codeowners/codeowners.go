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

// Package codeowners parses a GitHub CODEOWNERS file and answers "is this
// path a chokepoint, and who is queued on it".
package codeowners

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"code.vikunja.io/veans/internal/marshal/pathpattern"
)

// Rule is one CODEOWNERS line.
type Rule struct {
	Pattern string   // as written, e.g. "/captain-yard-web/src/lib/contract/" or "*.md"
	Owners  []string // "@subinsayzz", "@org/team"
	Line    int
	// Comment is the comment block heading the rule (joined, trimmed), used as
	// the human "why it collides". Consecutive rules share the block above the
	// first of them; a blank line ends the block.
	Comment string
}

// File is a parsed CODEOWNERS file.
type File struct{ Rules []Rule }

// Parse reads a CODEOWNERS file. Patterns are validated against the board's
// scope-path dialect (see Chokepoints) so a rule that parses here is one the
// board can hold a claim on.
func Parse(b []byte) (*File, error) {
	f := &File{}
	var (
		comment       []string
		commentSealed bool // a rule has consumed the block; it stays until a blank line
	)
	sc := bufio.NewScanner(bytes.NewReader(b))
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		switch {
		case raw == "":
			comment, commentSealed = nil, false
			continue
		case strings.HasPrefix(raw, "#"):
			if commentSealed {
				comment, commentSealed = nil, false
			}
			if text := strings.TrimSpace(strings.TrimPrefix(raw, "#")); text != "" {
				comment = append(comment, text)
			}
			continue
		}
		rule, err := parseRule(raw, line)
		if err != nil {
			return nil, err
		}
		rule.Comment = strings.Join(comment, " ")
		commentSealed = true
		f.Rules = append(f.Rules, rule)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading CODEOWNERS: %w", err)
	}
	return f, nil
}

func parseRule(raw string, line int) (Rule, error) {
	fields := strings.Fields(raw)
	pattern := fields[0]
	if strings.HasPrefix(pattern, "!") {
		return Rule{}, fmt.Errorf("CODEOWNERS line %d: negated patterns are not supported", line)
	}
	if _, err := pathpattern.Normalize(boardPattern(pattern)); err != nil {
		return Rule{}, fmt.Errorf("CODEOWNERS line %d: %w", line, err)
	}
	rule := Rule{Pattern: pattern, Line: line}
	for _, tok := range fields[1:] {
		if strings.HasPrefix(tok, "#") {
			break
		}
		rule.Owners = append(rule.Owners, tok)
	}
	return rule, nil
}

// Matches reports the rules covering a repo-relative file path, following
// GitHub semantics: a leading "/" anchors to the repo root; a trailing "/"
// matches everything below the directory; a pattern without a slash matches
// the basename anywhere; "*" and "**" as in gitignore. Later rules take
// precedence for ownership but ALL matching rules are returned, last first.
func (f *File) Matches(path string) []Rule {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "./"), "/")
	if path == "" {
		return nil
	}
	var out []Rule
	for i := len(f.Rules) - 1; i >= 0; i-- {
		if pathpattern.MatchAny(globs(f.Rules[i].Pattern), path) {
			out = append(out, f.Rules[i])
		}
	}
	return out
}

// Chokepoints returns the distinct patterns, in file order, normalised to
// the form the board uses for paths_owned: leading "/" stripped, trailing "/"
// replaced by "/**", e.g. "/captain-yard-web/src/lib/contract/" ->
// "captain-yard-web/src/lib/contract/**". When the board's paths are relative
// to a sub-directory, root ("captain-yard-web") strips that prefix too;
// rules that do not fall under it come back in outside. Unanchored patterns
// ("*.md", "apps/") apply everywhere, so they are always inside.
func (f *File) Chokepoints(root string) (inside, outside []string) {
	root = strings.Trim(root, "/")
	seen := map[string]bool{}
	for _, r := range f.Rules {
		p := boardPattern(r.Pattern)
		if seen[p] {
			continue
		}
		seen[p] = true
		if root == "" {
			inside = append(inside, p)
			continue
		}
		switch {
		case p == root:
			inside = append(inside, "**")
		case strings.HasPrefix(p, root+"/"):
			inside = append(inside, strings.TrimPrefix(p, root+"/"))
		case strings.HasPrefix(p, "**/") || p == "**":
			inside = append(inside, p)
		default:
			outside = append(outside, p)
		}
	}
	return inside, outside
}

// boardPattern rewrites a CODEOWNERS pattern into the board's dialect: the
// anchoring slash goes, a directory's trailing slash becomes "/**", and a
// pattern with no slash — which GitHub matches at any depth — gets "**/".
func boardPattern(pattern string) string {
	dir := strings.HasSuffix(pattern, "/")
	p := strings.Trim(pattern, "/")
	if p == "" || p == "**" {
		return "**"
	}
	if !strings.Contains(p, "/") && !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(p, "**") {
		p = "**/" + p
	}
	if dir {
		p += "/**"
	}
	return p
}

// globs lists the board patterns a CODEOWNERS pattern stands for when matched
// against a file. A wildcard-free last segment may name a directory, so the
// subtree below it counts too; a wildcard last segment does not descend
// ("docs/*" owns docs/a.md but not docs/b/c.md — GitHub's documented
// deviation from gitignore). Directory patterns already carry their "/**".
func globs(pattern string) []string {
	p := boardPattern(pattern)
	if strings.HasSuffix(p, "/**") {
		return []string{p}
	}
	segs := strings.Split(p, "/")
	last := segs[len(segs)-1]
	if strings.ContainsAny(last, "*?[") || !strings.ContainsAny(p, "*?[") {
		// A wildcard-free pattern covers its subtree by prefix already.
		return []string{p}
	}
	return []string{p, p + "/**"}
}
