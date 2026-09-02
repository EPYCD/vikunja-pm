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

func TestFindPastes(t *testing.T) {
	corpus, err := NewSpecCorpus(t.Context(), WorktreeReader("testdata"), []string{"prd.md", "spine.md", "epics.md"}, 12)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		text string
		want []PasteMatch
	}{
		{
			name: "twelve verbatim words inside other prose",
			text: "<p>As stated, files uploaded to a work item stay with it when the item is archived.</p>",
			want: []PasteMatch{{
				File: "prd.md", Line: 26, Words: 12,
				Excerpt: "files uploaded to a work item stay with it when the item",
			}},
		},
		{
			name: "eleven verbatim words are below the threshold",
			text: "Note that files uploaded to a work item stay with it when the deadline passes and more words follow.",
			want: nil,
		},
		{
			name: "a run made only of ids is never a paste",
			text: "FR-1 FR-2 FR-3 FR-4 FR-5 FR-6 FR-7 FR-8 FR-9 FR-10 FR-11 FR-12 FR-13 FR-14",
			want: nil,
		},
		{
			name: "two pastes come back longest first",
			text: "<p>Anyone with a verified email address can create an account. The account</p>" +
				"<p>Meanwhile: files uploaded to a work item stay with it when the item moves between projects.</p>",
			want: []PasteMatch{
				{File: "prd.md", Line: 26, Words: 15, Excerpt: "files uploaded to a work item stay with it when the item moves between projects."},
				{File: "prd.md", Line: 9, Words: 12, Excerpt: "Anyone with a verified email address can create an account. The account"},
			},
		},
		{
			// The bare "-" normalizes to nothing, so it drops out of the excerpt too.
			name: "case and punctuation do not matter",
			text: `FILES, uploaded - to a "work" item; stay with it when the ITEM moves!`,
			want: []PasteMatch{{
				File: "prd.md", Line: 26, Words: 13,
				Excerpt: `FILES, uploaded to a "work" item; stay with it when the ITEM moves!`,
			}},
		},
		{name: "empty text", text: "", want: nil},
		{name: "a single reference id", text: "FR-161", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := corpus.FindPastes(tc.text)
			if !slices.Equal(got, tc.want) {
				t.Errorf("FindPastes() = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestFindPastesMergesOverlaps(t *testing.T) {
	words := strings.Fields("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima " +
		"mike november oscar papa quebec romeo sierra tango uniform victor whiskey xray yankee zulu apple banana")
	if len(words) != 28 {
		t.Fatalf("fixture has %d words", len(words))
	}
	// Line 3 holds words 0-19, line 7 holds words 8-27; the task text holds
	// all 28, so two 20-word runs overlap on words 8-19.
	spec := "intro line\n\n" + strings.Join(words[:20], " ") + "\n\nfiller text here\n\n" + strings.Join(words[8:], " ") + "\n"
	corpus, err := NewSpecCorpus(t.Context(), memReader{"merge.md": spec}, []string{"merge.md"}, 12)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(words, " ")
	got := corpus.FindPastes(joined)
	want := []PasteMatch{{File: "merge.md", Line: 3, Words: 28, Excerpt: joined[:120]}}
	if !slices.Equal(got, want) {
		t.Errorf("FindPastes() = %+v\nwant %+v", got, want)
	}
}

func TestFindPastesRepeatedPassageReportedOnce(t *testing.T) {
	sentence := "one two three four five six seven eight nine ten eleven twelve thirteen"
	spec := sentence + "\n\nother\n\n" + sentence + "\n"
	corpus, err := NewSpecCorpus(t.Context(), memReader{"dup.md": spec}, []string{"dup.md"}, 12)
	if err != nil {
		t.Fatal(err)
	}
	got := corpus.FindPastes(sentence)
	want := []PasteMatch{{File: "dup.md", Line: 1, Words: 13, Excerpt: sentence}}
	if !slices.Equal(got, want) {
		t.Errorf("FindPastes() = %+v\nwant %+v", got, want)
	}
}

func TestNewSpecCorpusDefaultsAndErrors(t *testing.T) {
	prd := string(readFixture(t, "prd.md"))
	corpus, err := NewSpecCorpus(t.Context(), memReader{"prd.md": prd}, []string{"prd.md", "prd.md"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.minWords != 12 {
		t.Errorf("minWords = %d, want default 12", corpus.minWords)
	}
	if got := corpus.FindPastes("files uploaded to a work item stay with it when the item moves"); len(got) != 1 || got[0].Words != 13 {
		t.Errorf("duplicate file listing should still yield one match, got %+v", got)
	}

	if _, err := NewSpecCorpus(t.Context(), memReader{}, []string{"docs/missing.md"}, 12); err == nil || !strings.Contains(err.Error(), "docs/missing.md") {
		t.Errorf("err = %v, want one naming the file", err)
	}

	var nilCorpus *SpecCorpus
	if got := nilCorpus.FindPastes("anything"); got != nil {
		t.Errorf("nil corpus returned %+v", got)
	}
}
