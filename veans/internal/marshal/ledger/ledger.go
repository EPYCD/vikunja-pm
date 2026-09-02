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

// Package ledger is Marshal's append-only record of every acquire, release
// and refusal it sees or issues. Each entry carries the sha256 of the
// previous line, so a deleted or edited line breaks the chain and Verify
// says where.
package ledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one ledger line.
type Entry struct {
	Seq      int64          `json:"seq"`
	At       time.Time      `json:"at"`
	Action   string         `json:"action"` // acquire, release, refuse, receipt, drift, stale, allocate, free
	Actor    string         `json:"actor"`  // bot username, human login, "ci", "marshal"
	ActorID  int64          `json:"actor_id,omitempty"`
	TaskID   int64          `json:"task_id,omitempty"`
	Subject  string         `json:"subject,omitempty"` // path pattern, reference id, checkout path
	Outcome  string         `json:"outcome"`           // ok, refused, error
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Prev     string         `json:"prev"` // sha256 of the previous line, "" for the first
	Hash     string         `json:"hash"` // sha256 of this line with Hash empty
}

// Ledger appends to one file, serialised in-process; cross-process writers
// are serialised by the O_APPEND single-write guarantee for lines under the
// pipe buffer size, which every entry here is.
type Ledger struct {
	path string
	mu   sync.Mutex
}

// Open prepares a ledger at path, creating the directory.
func Open(path string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ledger dir: %w", err)
	}
	return &Ledger{path: path}, nil
}

// Path returns the file the ledger writes to.
func (l *Ledger) Path() string { return l.path }

// Append writes one entry, filling Seq, At (when zero), Prev and Hash.
func (l *Ledger) Append(e Entry) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	last, err := l.last()
	if err != nil {
		return e, err
	}
	e.Seq = last.Seq + 1
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	e.Prev = last.Hash
	e.Hash = ""
	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	e.Hash = hashLine(line)
	line, err = json.Marshal(e)
	if err != nil {
		return e, err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return e, fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return e, fmt.Errorf("append ledger: %w", err)
	}
	return e, nil
}

// Read returns every entry, oldest first.
func (l *Ledger) Read() ([]Entry, error) {
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Entry{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return out, fmt.Errorf("ledger line %d: %w", len(out)+1, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Verify walks the chain and returns the first sequence number whose hash or
// prev link does not match, or 0 when the ledger is intact.
func (l *Ledger) Verify() (int64, error) {
	entries, err := l.Read()
	if err != nil {
		return 0, err
	}
	prev := ""
	for _, e := range entries {
		if e.Prev != prev {
			return e.Seq, nil
		}
		want := e.Hash
		e.Hash = ""
		line, err := json.Marshal(e)
		if err != nil {
			return e.Seq, err
		}
		if hashLine(line) != want {
			return e.Seq, nil
		}
		prev = want
	}
	return 0, nil
}

func (l *Ledger) last() (Entry, error) {
	entries, err := l.Read()
	if err != nil {
		return Entry{}, err
	}
	if len(entries) == 0 {
		return Entry{}, nil
	}
	return entries[len(entries)-1], nil
}

func hashLine(line []byte) string {
	sum := sha256.Sum256(line)
	return hex.EncodeToString(sum[:])
}
