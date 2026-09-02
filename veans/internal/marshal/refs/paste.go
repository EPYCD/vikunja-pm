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
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultMinWords = 12
	maxPasteMatches = 20
	excerptLen      = 120
)

// PasteMatch is a verbatim run shared between task text and a spec file.
type PasteMatch struct {
	File    string
	Line    int    // 1-based line in the spec file where the run starts
	Words   int    // length of the run in words
	Excerpt string // first 120 chars of the run
}

// SpecCorpus indexes the text of configured spec files for paste detection.
type SpecCorpus struct {
	minWords int
	files    []corpusFile
	shingles map[string][]shinglePos
}

type corpusFile struct {
	name   string
	tokens []string
	lines  []int // 1-based source line of each token
}

type shinglePos struct {
	file, word int
}

// NewSpecCorpus tokenises each file into words (lowercased, punctuation
// stripped, whitespace-split) and builds a shingle index of length minWords.
// minWords <= 0 selects 12.
func NewSpecCorpus(ctx context.Context, r Reader, files []string, minWords int) (*SpecCorpus, error) {
	if r == nil {
		return nil, errNilReader
	}
	if minWords <= 0 {
		minWords = defaultMinWords
	}
	c := &SpecCorpus{minWords: minWords, shingles: map[string][]shinglePos{}}
	for _, name := range files {
		if slices.ContainsFunc(c.files, func(f corpusFile) bool { return f.name == name }) {
			continue
		}
		b, err := r.ReadFile(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("spec file %s: %w", name, err)
		}
		tokens, _, lines := tokenize(string(b))
		fi := len(c.files)
		c.files = append(c.files, corpusFile{name: name, tokens: tokens, lines: lines})
		for w := 0; w+minWords <= len(tokens); w++ {
			key := shingleKey(tokens[w : w+minWords])
			c.shingles[key] = append(c.shingles[key], shinglePos{file: fi, word: w})
		}
	}
	return c, nil
}

type pasteRun struct {
	file, start, end, line int
}

// FindPastes strips HTML from text, tokenises it like the corpus, and reports
// every maximal run of >= minWords consecutive words that also occurs
// consecutively in a corpus file. Runs overlapping in the text are merged per
// file; at most 20 matches are returned, longest first. A run made only of
// ids or numbers is never a match.
func (c *SpecCorpus) FindPastes(text string) []PasteMatch {
	if c == nil || len(c.shingles) == 0 {
		return nil
	}
	plain, _ := stripHTML(text)
	toks, words, _ := tokenize(plain)
	k := c.minWords
	var runs []pasteRun
	for i := 0; i+k <= len(toks); i++ {
		for _, p := range c.shingles[shingleKey(toks[i:i+k])] {
			ft := c.files[p.file].tokens
			// A match whose predecessors also agree is the tail of a run
			// already recorded from an earlier start.
			if i > 0 && p.word > 0 && toks[i-1] == ft[p.word-1] {
				continue
			}
			n := k
			for i+n < len(toks) && p.word+n < len(ft) && toks[i+n] == ft[p.word+n] {
				n++
			}
			runs = append(runs, pasteRun{file: p.file, start: i, end: i + n, line: c.files[p.file].lines[p.word]})
		}
	}

	var out []PasteMatch
	for _, r := range mergeRuns(runs) {
		if allIDs(toks[r.start:r.end]) {
			continue
		}
		out = append(out, PasteMatch{
			File:    c.files[r.file].name,
			Line:    r.line,
			Words:   r.end - r.start,
			Excerpt: excerpt(words[r.start:r.end]),
		})
	}
	slices.SortStableFunc(out, func(a, b PasteMatch) int {
		if a.Words != b.Words {
			return b.Words - a.Words
		}
		if a.File != b.File {
			return strings.Compare(a.File, b.File)
		}
		return a.Line - b.Line
	})
	if len(out) > maxPasteMatches {
		out = out[:maxPasteMatches]
	}
	return out
}

// mergeRuns joins runs of the same file that overlap in the text. The merged
// run reports the line of its longest constituent, which is where the bulk of
// the paste came from.
func mergeRuns(runs []pasteRun) []pasteRun {
	slices.SortFunc(runs, func(a, b pasteRun) int {
		if a.file != b.file {
			return a.file - b.file
		}
		if a.start != b.start {
			return a.start - b.start
		}
		return b.end - a.end
	})
	var merged []pasteRun
	bestLen := 0 // length of the constituent supplying the tail's line
	for _, r := range runs {
		n := len(merged)
		if n == 0 || r.file != merged[n-1].file || r.start >= merged[n-1].end {
			merged = append(merged, r)
			bestLen = r.end - r.start
			continue
		}
		cur := &merged[n-1]
		if l := r.end - r.start; l > bestLen {
			bestLen = l
			cur.line = r.line
		}
		cur.end = max(cur.end, r.end)
	}
	return merged
}

// tokenize lowercases each whitespace-separated field and drops everything
// but letters and digits, so "Files," and "files" agree and markdown marks
// vanish. It returns the normalized tokens, the original fields (for
// excerpts), and the 1-based line of each token.
func tokenize(s string) (tokens, words []string, lines []int) {
	for ln, line := range strings.Split(s, "\n") {
		for _, field := range strings.Fields(line) {
			tok := normalizeToken(field)
			if tok == "" {
				continue
			}
			tokens = append(tokens, tok)
			words = append(words, field)
			lines = append(lines, ln+1)
		}
	}
	return tokens, words, lines
}

func normalizeToken(field string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, field)
}

func shingleKey(tokens []string) string {
	return strings.Join(tokens, " ")
}

// idTokenRE matches a normalised reference id ("fr161a", "n3") or a bare number.
var idTokenRE = regexp.MustCompile(`^[a-z]*\d+[a-z]?$`)

func allIDs(tokens []string) bool {
	for _, t := range tokens {
		if !idTokenRE.MatchString(t) {
			return false
		}
	}
	return true
}

func excerpt(words []string) string {
	s := strings.Join(words, " ")
	if utf8.RuneCountInString(s) <= excerptLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:excerptLen])
}
