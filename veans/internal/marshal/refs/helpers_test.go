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
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// memReader serves spec files from memory so tests can shape edge cases
// without touching testdata.
type memReader map[string]string

func (m memReader) ReadFile(_ context.Context, path string) ([]byte, error) {
	s, ok := m[path]
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, fs.ErrNotExist)
	}
	return []byte(s), nil
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // fixed fixture names under testdata
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var testSources = []Source{
	{Prefix: "FR-", File: "prd.md"},
	{Prefix: "NFR-", File: "prd.md"},
	{Prefix: "AD-", File: "spine.md"},
	{Prefix: "D-", File: "epics.md"},
}
