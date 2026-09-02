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
	"slices"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	htmlText := `<p>Implements <strong>FR-161</strong> &amp; FR-161a; see <a href="x>y">AD-14</a>.&nbsp;` +
		`Also FR-161 again, NFR-N3 but not nfr-n3, FR-161abc or FR-.</p><!-- FR-7 in a comment -->`
	tests := []struct {
		name     string
		text     string
		prefixes []string
		want     []Ref
	}{
		{
			name:     "plain text",
			text:     "Implements FR-161 and FR-161a; NFR-N3 is unrelated to FR- and AD-14.",
			prefixes: []string{"FR-", "AD-"},
			want: []Ref{
				{ID: "FR-161", Prefix: "FR-", Offset: 11},
				{ID: "FR-161a", Prefix: "FR-", Offset: 22},
				{ID: "AD-14", Prefix: "AD-", Offset: 62},
			},
		},
		{
			// FR-7 sits inside an HTML comment and must not surface.
			name:     "html with entities, dedup, offsets into the original",
			text:     htmlText,
			prefixes: []string{"FR-", "AD-", "NFR-"},
			want: []Ref{
				{ID: "FR-161", Prefix: "FR-", Offset: strings.Index(htmlText, "FR-161")},
				{ID: "FR-161a", Prefix: "FR-", Offset: strings.Index(htmlText, "FR-161a")},
				{ID: "AD-14", Prefix: "AD-", Offset: strings.Index(htmlText, "AD-14")},
				{ID: "NFR-N3", Prefix: "NFR-", Offset: strings.Index(htmlText, "NFR-N3")},
			},
		},
		{
			name:     "longest prefix wins at the same position",
			text:     "NFR-N3",
			prefixes: []string{"FR-", "NFR-"},
			want:     []Ref{{ID: "NFR-N3", Prefix: "NFR-", Offset: 0}},
		},
		{
			name:     "prefix or id glued to other word characters is not a reference",
			text:     "NFR-N3 XFR-1 FR-1xy FR-1X",
			prefixes: []string{"FR-"},
			want:     nil,
		},
		{
			name:     "underscore emphasis is a boundary",
			text:     "_FR-5_ and _FR-6a_",
			prefixes: []string{"FR-"},
			want:     []Ref{{ID: "FR-5", Prefix: "FR-", Offset: 1}, {ID: "FR-6a", Prefix: "FR-", Offset: 12}},
		},
		{
			name:     "single lowercase suffix only",
			text:     "FR-161a FR-161ab FR-161b",
			prefixes: []string{"FR-"},
			want:     []Ref{{ID: "FR-161a", Prefix: "FR-", Offset: 0}, {ID: "FR-161b", Prefix: "FR-", Offset: 17}},
		},
		{
			name:     "entities separate words and keep offsets",
			text:     "FR-1&nbsp;FR-2&lt;FR-3&#39;",
			prefixes: []string{"FR-"},
			want: []Ref{
				{ID: "FR-1", Prefix: "FR-", Offset: 0},
				{ID: "FR-2", Prefix: "FR-", Offset: 10},
				{ID: "FR-3", Prefix: "FR-", Offset: 18},
			},
		},
		{
			name:     "lone or unterminated angle bracket is text",
			text:     "1 < 2 FR-3 a<b FR-4",
			prefixes: []string{"FR-"},
			want:     []Ref{{ID: "FR-3", Prefix: "FR-", Offset: 6}, {ID: "FR-4", Prefix: "FR-", Offset: 15}},
		},
		{
			name:     "block tags split words",
			text:     "<ul><li>FR-1</li><li>FR-2</li></ul>",
			prefixes: []string{"FR-"},
			want:     []Ref{{ID: "FR-1", Prefix: "FR-", Offset: 8}, {ID: "FR-2", Prefix: "FR-", Offset: 21}},
		},
		{name: "no prefixes", text: "FR-1", prefixes: nil, want: nil},
		{name: "empty prefix ignored", text: "FR-1", prefixes: []string{""}, want: nil},
		{name: "empty text", text: "", prefixes: []string{"FR-"}, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Extract(tc.text, tc.prefixes)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Extract() = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"<p>a</p><p>b</p>", "\na\n\nb\n"},
		{"verb<em>atim</em>", "verbatim"},
		{"x &amp; y &lt;z&gt; &quot;q&quot; &#39;s&#39; &#x27;t&#x27;", `x & y <z> "q" 's' 't'`},
		{"a&nbsp;b", "a b"},
		{"a & b &notanentity", "a & b &notanentity"},
		{`<a href="1>2" title='3>4'>t</a>`, "t"},
		{"<!-- gone -->kept", "kept"},
		{"<!-- open", "<!-- open"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, _ := stripHTML(tc.in)
			if got != tc.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
