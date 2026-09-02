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
	"html"
	"regexp"
	"slices"
	"strings"
)

// Extract finds references in text (task titles, descriptions, comments). The
// text may be HTML: tags are stripped and common entities decoded first. A
// reference is a configured prefix followed by an id body ([A-Z]?\d+[a-z]?)
// with a word boundary on both sides, so "FR-161" inside "NFR-161" does not
// count; letters and digits are word characters, "_" is not, matching the
// anchor patterns. Results are deduplicated by ID, keeping the first offset,
// in order of appearance; offsets index the original text.
func Extract(text string, prefixes []string) []Ref {
	re := refPattern(prefixes)
	if re == nil {
		return nil
	}
	plain, offs := stripHTML(text)
	var out []Ref
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatchIndex(plain, -1) {
		start, end := m[0], m[1]
		if !boundaryBefore(plain, start) || !boundaryAfter(plain, end) {
			continue
		}
		id := plain[start:end]
		if seen[id] {
			continue
		}
		seen[id] = true
		off := start
		if offs != nil {
			off = offs[start]
		}
		out = append(out, Ref{ID: id, Prefix: plain[m[2]:m[3]], Offset: off})
	}
	return out
}

// refPattern builds `(P1|P2|…)idBody` with longer prefixes first so "NFR-"
// wins over "FR-" at the same position. Nil when no usable prefix remains.
func refPattern(prefixes []string) *regexp.Regexp {
	uniq := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p != "" && !slices.Contains(uniq, p) {
			uniq = append(uniq, p)
		}
	}
	if len(uniq) == 0 {
		return nil
	}
	slices.SortStableFunc(uniq, func(a, b string) int { return len(b) - len(a) })
	quoted := make([]string, len(uniq))
	for i, p := range uniq {
		quoted[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(` + strings.Join(quoted, "|") + `)` + idBody)
}

func isWordByte(c byte) bool {
	return (c >= '0' && c <= '9') || isASCIILetter(c)
}

func boundaryBefore(s string, i int) bool {
	return i == 0 || !isWordByte(s[i-1])
}

func boundaryAfter(s string, i int) bool {
	return i == len(s) || !isWordByte(s[i])
}

// blockTags are replaced by a newline when stripping so words on either side
// stay separate; inline tags vanish so "verb<em>atim</em>" stays one word.
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "ul": true, "ol": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "tr": true, "td": true, "th": true,
	"table": true, "hr": true, "section": true, "article": true,
	"header": true, "footer": true,
}

const maxEntityLen = 32

// stripHTML removes tags and decodes entities. The returned slice maps each
// byte of the result to the byte offset in s it came from; it is nil when s
// needed no rewriting, in which case offsets are identical.
func stripHTML(s string) (string, []int) {
	if !strings.ContainsAny(s, "<&") {
		return s, nil
	}
	var b strings.Builder
	offs := make([]int, 0, len(s))
	emit := func(str string, at int) {
		b.WriteString(str)
		for range len(str) {
			offs = append(offs, at)
		}
	}
	for i := 0; i < len(s); {
		switch s[i] {
		case '<':
			if end, name, ok := scanTag(s, i); ok {
				if blockTags[name] {
					emit("\n", i)
				}
				i = end
				continue
			}
		case '&':
			if dec, n := decodeEntity(s, i); n > 0 {
				emit(dec, i)
				i += n
				continue
			}
		}
		emit(s[i:i+1], i)
		i++
	}
	return b.String(), offs
}

// scanTag recognizes a tag or comment starting at s[i]=='<' and returns the
// index just past it plus the lowercased tag name. A lone or unterminated '<'
// is literal text, so "a<b" never swallows the rest of the string.
func scanTag(s string, i int) (int, string, bool) {
	if strings.HasPrefix(s[i:], "<!--") {
		end := strings.Index(s[i+4:], "-->")
		if end < 0 {
			return 0, "", false
		}
		return i + 4 + end + 3, "", true
	}
	j := i + 1
	if j < len(s) && s[j] == '/' {
		j++
	}
	if j >= len(s) || !isASCIILetter(s[j]) && s[j] != '!' {
		return 0, "", false
	}
	nameStart := j
	for j < len(s) && (isASCIILetter(s[j]) || s[j] >= '0' && s[j] <= '9') {
		j++
	}
	name := strings.ToLower(s[nameStart:j])
	var quote byte
	for ; j < len(s); j++ {
		c := s[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j + 1, name, true
		}
	}
	return 0, "", false
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// decodeEntity decodes a well-formed entity ("&amp;", "&#39;", "&#x27;", …)
// starting at s[i]=='&'. A non-breaking space becomes a plain space so word
// splitting treats it like any other whitespace. n is 0 when nothing decoded.
func decodeEntity(s string, i int) (string, int) {
	limit := min(len(s), i+maxEntityLen)
	semi := strings.IndexByte(s[i:limit], ';')
	if semi <= 0 {
		return "", 0
	}
	raw := s[i : i+semi+1]
	dec := html.UnescapeString(raw)
	if dec == raw {
		return "", 0
	}
	return strings.ReplaceAll(dec, "\u00a0", " "), len(raw)
}
