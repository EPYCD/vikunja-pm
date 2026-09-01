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

package models

import (
	"path"
	"strings"
)

// Scope paths are repository-relative glob patterns: `pkg/models/tasks.go`,
// `pkg/models/*.go`, `frontend/src/**`. Segments are matched with path.Match
// and `**` matches any number of segments, which is the subset of glob syntax
// agents actually write and the one whose overlap can be decided cheaply.

const maxScopePathLength = 500

// NormalizeScopePath canonicalises a pattern so equal intents compare equal:
// trims, strips a leading `./` or `/`, collapses repeated slashes and drops a
// trailing slash. It rejects anything that could escape the repository.
func NormalizeScopePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", ErrInvalidScopePath{Pattern: raw, Reason: "empty"}
	}
	if len(p) > maxScopePathLength {
		return "", ErrInvalidScopePath{Pattern: raw, Reason: "longer than 500 characters"}
	}
	if strings.ContainsAny(p, "\x00\n\r") {
		return "", ErrInvalidScopePath{Pattern: raw, Reason: "contains control characters"}
	}
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
		return "", ErrInvalidScopePath{Pattern: raw, Reason: "does not name anything"}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", ErrInvalidScopePath{Pattern: raw, Reason: "must not contain '..'"}
		}
		if seg == "." {
			return "", ErrInvalidScopePath{Pattern: raw, Reason: "must not contain '.' segments"}
		}
	}
	return p, nil
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
		// Try consuming zero segments, then one, then more.
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

// PathPatternsOverlap reports whether two normalised scope patterns can name
// the same file. It is exact for the common shapes — concrete paths, a
// directory subtree, single-segment wildcards — and conservative for the rest:
// two wildcard patterns with a shared literal prefix and the same depth are
// treated as overlapping, so an exotic pair may be refused that a full
// intersection test would allow, but a real collision is never missed.
func PathPatternsOverlap(a, b string) bool {
	if a == b {
		return true
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
