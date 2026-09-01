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

package commands

import (
	"reflect"
	"testing"

	"code.vikunja.io/veans/internal/client"
)

func TestScopeFlags_ApplyKeepsUnsetFields(t *testing.T) {
	stored := &client.TaskScope{
		PathsOwned:    []string{"pkg/models/**"},
		PathsAffected: []string{"docs/api.md"},
		Endpoints:     []string{"GET /x"},
		Notes:         "<p>no frontend</p>",
	}

	f := &scopeFlags{owned: []string{"pkg/routes/**"}, ownedSet: true, notes: "", notesSet: true}
	got := f.apply(stored)

	want := &client.TaskScope{
		PathsOwned:    []string{"pkg/routes/**"},
		PathsAffected: []string{"docs/api.md"},
		Endpoints:     []string{"GET /x"},
		Notes:         "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("apply = %+v, want %+v", got, want)
	}
}

func TestScopeFlags_ApplyFromNothingSendsEmptyLists(t *testing.T) {
	f := &scopeFlags{endpoints: []string{"POST /y"}, endpSet: true}
	got := f.apply(nil)
	// Empty slices, not nil: the PUT body must carry `[]` so the server
	// validator sees lists, and a later read compares equal.
	if got.PathsOwned == nil || got.PathsAffected == nil {
		t.Fatalf("unset lists must be empty, not nil: %+v", got)
	}
	if !reflect.DeepEqual(got.Endpoints, []string{"POST /y"}) {
		t.Fatalf("endpoints = %v", got.Endpoints)
	}
}

func TestWithRepoPrefix(t *testing.T) {
	got := withRepoPrefix("api", []string{"pkg/models/**", "web:src/App.vue", " docs/x.md ", ""})
	want := []string{"api:pkg/models/**", "web:src/App.vue", "api:docs/x.md", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("withRepoPrefix = %v, want %v", got, want)
	}
	if got := withRepoPrefix("", []string{"pkg/x"}); !reflect.DeepEqual(got, []string{"pkg/x"}) {
		t.Fatalf("no repository configured must leave paths alone: %v", got)
	}
}
