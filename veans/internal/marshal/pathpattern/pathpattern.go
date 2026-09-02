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

// Package pathpattern is a port of the server's scope-path semantics
// (pkg/models/path_pattern.go) so Marshal judges coverage and overlap exactly
// as the board does. Keep the two in lockstep.
//
// Scope paths are repository-relative glob patterns: `pkg/models/tasks.go`,
// `pkg/models/*.go`, `frontend/src/**`. Segments are matched with path.Match
// and `**` matches any number of segments, which is the subset of glob syntax
// agents actually write and the one whose overlap can be decided cheaply.
package pathpattern

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const maxLength = 500

// Normalize canonicalises a pattern so equal intents compare equal: trims,
// strips a leading `./` or `/`, collapses repeated slashes and drops a
// trailing slash. It rejects anything that could escape the repository.
func Normalize(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", errors.New("invalid scope path: empty")
	}
	if len(p) > maxLength {
		return "", fmt.Errorf("invalid scope path %q: longer than %d characters", raw, maxLength)
	}
	if strings.ContainsAny(p, "\x00\n\r") {
		return "", fmt.Errorf("invalid scope path %q: contains control characters", raw)
	}
	repo, p := SplitRepo(p)
	if repo != "" && !validRepoName(repo) {
		return "", fmt.Errorf("invalid scope path %q: repository prefix may only contain letters, digits, '.', '_' and '-'", raw)
	}
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	p = strings.TrimPrefix(p, "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	p = strings.TrimSuffix(p, "/")
	if p == "" || p == "." {
		return "", fmt.Errorf("invalid scope path %q: does not name anything", raw)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid scope path %q: must not contain '..'", raw)
		}
		if seg == "." {
			return "", fmt.Errorf("invalid scope path %q: must not contain '.' segments", raw)
		}
	}
	if repo != "" {
		return repo + ":" + p, nil
	}
	return p, nil
}

// SplitRepo separates an optional `repo:` namespace from a pattern.
// Projects spanning several repositories prefix their paths so `src/index.ts`
// in one repo never collides with the same path in another; patterns without
// a prefix all live in the project's default repository. Only a prefix that
// looks like a name counts — a colon inside a path segment stays a path.
func SplitRepo(p string) (repo, rest string) {
	i := strings.Index(p, ":")
	if i <= 0 || strings.ContainsAny(p[:i], "/\\*?[") {
		return "", p
	}
	return p[:i], p[i+1:]
}

func validRepoName(repo string) bool {
	for _, r := range repo {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func segmentHasWildcard(seg string) bool {
	return strings.ContainsAny(seg, "*?[")
}

// literalPrefix returns the leading segments up to the first one containing a
// wildcard. Two patterns whose literal prefixes diverge can never match the
// same path.
func literalPrefix(segs []string) []string {
	for i, seg := range segs {
		if segmentHasWildcard(seg) {
			return segs[:i]
		}
	}
	return segs
}

func isPrefixOf(short, long []string) bool {
	if len(short) > len(long) {
		return false
	}
	for i := range short {
		if short[i] != long[i] {
			return false
		}
	}
	return true
}

// matchGlob reports whether a concrete path matches a pattern, with `**`
// spanning zero or more segments and every other segment going through
// path.Match.
func matchGlob(pattern, concrete []string) bool {
	if len(pattern) == 0 {
		return len(concrete) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(concrete); i++ {
			if matchGlob(pattern[1:], concrete[i:]) {
				return true
			}
		}
		return false
	}
	if len(concrete) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], concrete[0])
	if err != nil || !ok {
		return false
	}
	return matchGlob(pattern[1:], concrete[1:])
}

func hasWildcard(segs []string) bool {
	for _, s := range segs {
		if segmentHasWildcard(s) {
			return true
		}
	}
	return false
}

// coversSubtree is true for patterns that claim everything under their
// literal prefix: `dir/**`, `dir/**/*`, `**`.
func coversSubtree(segs []string) bool {
	lit := literalPrefix(segs)
	rest := segs[len(lit):]
	if len(rest) == 0 {
		return false
	}
	if rest[0] != "**" {
		return false
	}
	for _, s := range rest[1:] {
		if s != "*" && s != "**" {
			return false
		}
	}
	return true
}

// Covers reports whether a concrete, normalised file path falls under a scope
// pattern: the pattern names the file, names a directory above it, or is a
// glob it matches. Patterns and files in different repositories (repo:
// prefix) never cover each other.
func Covers(pattern, file string) bool {
	repoP, pattern := SplitRepo(pattern)
	repoF, file := SplitRepo(file)
	if repoP != repoF {
		return false
	}
	ps, fs := strings.Split(pattern, "/"), strings.Split(file, "/")
	if !hasWildcard(ps) {
		return isPrefixOf(ps, fs)
	}
	return matchGlob(ps, fs)
}

// MatchAny reports whether at least one pattern covers the file.
func MatchAny(patterns []string, file string) bool {
	for _, p := range patterns {
		if Covers(p, file) {
			return true
		}
	}
	return false
}

// Overlap reports whether two normalised scope patterns can name the same
// file. It is exact for the common shapes — concrete paths, a directory
// subtree, single-segment wildcards — and conservative for the rest: two
// wildcard patterns with a shared literal prefix and the same depth are
// treated as overlapping, so an exotic pair may be refused that a full
// intersection test would allow, but a real collision is never missed.
func Overlap(a, b string) bool {
	if a == b {
		return true
	}
	repoA, a := SplitRepo(a)
	repoB, b := SplitRepo(b)
	if repoA != repoB {
		return false
	}
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	la, lb := literalPrefix(as), literalPrefix(bs)
	if !isPrefixOf(la, lb) && !isPrefixOf(lb, la) {
		return false
	}

	aWild, bWild := hasWildcard(as), hasWildcard(bs)
	switch {
	case !aWild && !bWild:
		// Two concrete paths overlap only when equal (handled above) or when
		// one is a directory containing the other.
		return isPrefixOf(as, bs) || isPrefixOf(bs, as)
	case !aWild:
		return matchGlob(bs, as) || (isPrefixOf(as, lb))
	case !bWild:
		return matchGlob(as, bs) || (isPrefixOf(bs, la))
	}

	// Both have wildcards and share a literal prefix.
	if coversSubtree(as) || coversSubtree(bs) {
		return true
	}
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] == bs[i] || segmentHasWildcard(as[i]) || segmentHasWildcard(bs[i]) {
			continue
		}
		return false
	}
	return true
}
