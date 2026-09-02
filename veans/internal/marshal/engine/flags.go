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

package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// flagStore remembers what Marshal already told people about, so a drift or
// a stale claim is announced once per state, not once per poll.
type flagStore struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

func openFlags(path string) (*flagStore, error) {
	f := &flagStore{path: path, data: map[string]string{}}
	b, err := os.ReadFile(path) //nolint:gosec // Marshal's own state file
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f.data); err != nil {
		return nil, err
	}
	return f, nil
}

// Seen reports whether key already carries value; when not, it records it.
// Returns true when the caller should stay quiet.
func (f *flagStore) Seen(key, value string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data[key] == value {
		return true
	}
	f.data[key] = value
	f.persist()
	return false
}

// Clear forgets a key, so the next occurrence is announced again.
func (f *flagStore) Clear(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.data[key]; !ok {
		return
	}
	delete(f.data, key)
	f.persist()
}

func (f *flagStore) persist() {
	b, err := json.MarshalIndent(f.data, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(f.path), 0o700)
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, f.path)
}
