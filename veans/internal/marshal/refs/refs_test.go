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

func TestResolve(t *testing.T) {
	anchor := Anchor{ID: "FR-161", File: "prd.md", Line: 7, Title: "t", Text: "x", Hash: "h"}
	anchors := map[string]Anchor{"FR-161": anchor}
	refs := []Ref{{ID: "FR-161", Prefix: "FR-", Offset: 3}, {ID: "FR-999", Prefix: "FR-", Offset: 9}}

	tests := []struct {
		name     string
		ix       *Index
		wantProv string
	}{
		{
			name: "rev wins over blob",
			ix: &Index{
				Rev:     "0123456789abcdef0123456789abcdef01234567",
				Anchors: anchors,
				Files:   map[string]string{"prd.md": "fedcba9876543210fedcba9876543210fedcba98"},
			},
			wantProv: "prd.md@0123456789ab",
		},
		{
			name:     "blob sha when rev is empty",
			ix:       &Index{Anchors: anchors, Files: map[string]string{"prd.md": "fedcba9876543210fedcba9876543210fedcba98"}},
			wantProv: "prd.md@fedcba987654",
		},
		{
			name:     "short rev is used whole",
			ix:       &Index{Rev: "abc123", Anchors: anchors},
			wantProv: "prd.md@abc123",
		},
		{
			name:     "neither known",
			ix:       &Index{Anchors: anchors},
			wantProv: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ix.Resolve(refs)
			if len(got) != 2 {
				t.Fatalf("got %d resolutions, want 2", len(got))
			}
			found := got[0]
			if !found.Found || found.Anchor != anchor || found.Ref != refs[0] || found.Provenance != tc.wantProv {
				t.Errorf("found = %+v, want anchor %+v with provenance %q", found, anchor, tc.wantProv)
			}
			missing := got[1]
			if missing.Found || missing.Anchor != (Anchor{}) || missing.Ref != refs[1] || missing.Provenance != "" {
				t.Errorf("missing = %+v, want zero anchor and empty provenance", missing)
			}
		})
	}

	var nilIx *Index
	for _, r := range nilIx.Resolve(refs) {
		if r.Found {
			t.Errorf("nil index resolved %+v", r)
		}
	}
}

func TestDiff(t *testing.T) {
	prev := &Index{Anchors: map[string]Anchor{
		"FR-1": {Hash: "same"}, "FR-2": {Hash: "before"}, "FR-3": {Hash: "gone"}, "AD-1": {Hash: "same"},
	}}
	next := &Index{Anchors: map[string]Anchor{
		"FR-1": {Hash: "same"}, "FR-2": {Hash: "after"}, "FR-4": {Hash: "new"}, "AD-1": {Hash: "same"}, "AD-2": {Hash: "new"},
	}}

	tests := []struct {
		name       string
		prev, next *Index
		want       Drift
	}{
		{"both", prev, next, Drift{Changed: []string{"FR-2"}, Vanished: []string{"FR-3"}, Appeared: []string{"AD-2", "FR-4"}}},
		{"same", prev, prev, Drift{}},
		{"nil old", nil, next, Drift{Appeared: []string{"AD-1", "AD-2", "FR-1", "FR-2", "FR-4"}}},
		{"nil new", prev, nil, Drift{Vanished: []string{"AD-1", "FR-1", "FR-2", "FR-3"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.prev, tc.next)
			if !slices.Equal(got.Changed, tc.want.Changed) ||
				!slices.Equal(got.Vanished, tc.want.Vanished) ||
				!slices.Equal(got.Appeared, tc.want.Appeared) {
				t.Errorf("Diff() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLoadSources(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    []Source
		wantErr string
	}{
		{
			name: "valid",
			yaml: "- prefix: FR-\n  file: ./docs/prd.md\n- prefix: AD-\n  file: docs/spine.md\n  pattern: '^### (AD-\\d+)'\n",
			want: []Source{
				{Prefix: "FR-", File: "docs/prd.md"},
				{Prefix: "AD-", File: "docs/spine.md", Pattern: `^### (AD-\d+)`},
			},
		},
		{name: "empty", yaml: "", want: nil},
		{name: "unknown field", yaml: "- prefix: FR-\n  file: prd.md\n  glob: x\n", wantErr: "field glob not found"},
		{name: "missing file", yaml: "- prefix: FR-\n", wantErr: "sources[0]: source FR-: file is required"},
		{name: "missing prefix", yaml: "- file: prd.md\n", wantErr: "prefix is required"},
		{name: "no capture group", yaml: "- prefix: FR-\n  file: prd.md\n  pattern: '^FR-\\d+'\n", wantErr: "capture group"},
		{name: "bad regexp", yaml: "- prefix: FR-\n  file: prd.md\n  pattern: '('\n", wantErr: "error parsing regexp"},
		{name: "not a list", yaml: "prefix: FR-\n", wantErr: "parse sources"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadSources([]byte(tc.yaml))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("LoadSources() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
