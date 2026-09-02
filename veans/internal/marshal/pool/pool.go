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

// Package pool hands out per-worker resources (a database URI and a dev-server
// port) and remembers which checkout each worker occupies. State lives in one
// JSON file so every marshal process on the machine sees the same registry.
package pool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	// ErrPoolExhausted is returned when no database or port is free; the
	// wrapped message names which one ran out.
	ErrPoolExhausted = errors.New("pool exhausted")
	// ErrCheckoutInUse is returned when another worker already occupies the
	// requested checkout.
	ErrCheckoutInUse = errors.New("checkout already registered to another worker")
)

// Config is the set of resources Allocate may hand out.
type Config struct {
	// Databases are complete connection strings, e.g. "mongodb://127.0.0.1:27101/capyard_w1".
	Databases []string `yaml:"databases"`
	// Ports are dev-server ports; a range "3100-3110" or single values are accepted in the YAML.
	Ports []int `yaml:"ports"`
}

// UnmarshalYAML accepts ports as ints or "a-b" strings, either in a list or
// as a single scalar.
func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Databases []string  `yaml:"databases"`
		Ports     yaml.Node `yaml:"ports"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	ports, err := parsePorts(&raw.Ports)
	if err != nil {
		return fmt.Errorf("pool ports: %w", err)
	}
	c.Databases = raw.Databases
	c.Ports = ports
	return nil
}

func parsePorts(n *yaml.Node) ([]int, error) {
	if n.Kind == 0 || n.ShortTag() == "!!null" {
		return nil, nil
	}
	if n.Kind == yaml.ScalarNode {
		return parsePortSpec(n.Value)
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: expected a port, a range or a list of them", n.Line)
	}
	var ports []int
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: expected a port or a range", item.Line)
		}
		ps, err := parsePortSpec(item.Value)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", item.Line, err)
		}
		ports = append(ports, ps...)
	}
	return ports, nil
}

func parsePortSpec(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	lo, hi, isRange := strings.Cut(spec, "-")
	if !isRange {
		p, err := parsePort(spec)
		if err != nil {
			return nil, err
		}
		return []int{p}, nil
	}
	from, err := parsePort(lo)
	if err != nil {
		return nil, fmt.Errorf("range %q: %w", spec, err)
	}
	to, err := parsePort(hi)
	if err != nil {
		return nil, fmt.Errorf("range %q: %w", spec, err)
	}
	if from > to {
		return nil, fmt.Errorf("range %q: start is after end", spec)
	}
	ports := make([]int, 0, to-from+1)
	for p := from; p <= to; p++ {
		ports = append(ports, p)
	}
	return ports, nil
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", s, err)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port %d out of range 1-65535", p)
	}
	return p, nil
}

// Allocation records what one worker holds for one task.
type Allocation struct {
	Worker   string    `json:"worker"` // bot username or human login
	TaskID   int64     `json:"task_id"`
	Story    string    `json:"story"` // "E5.3"
	Branch   string    `json:"branch"`
	Checkout string    `json:"checkout"` // absolute path of the worktree
	Database string    `json:"database"`
	Port     int       `json:"port"`
	Since    time.Time `json:"since"`
}

// Conflict is a checkout whose registry entries are inconsistent with rule 1
// or with the filesystem.
type Conflict struct {
	Checkout string
	Workers  []string
	Missing  bool
}

// Store is the on-disk registry. Every mutating method takes the file lock,
// reloads, mutates and writes atomically, so concurrent marshal processes
// never double-assign a resource.
type Store struct {
	path string
	// mu serializes callers inside one process; the flock on <path>.lock
	// covers other processes (and is a no-op on Windows).
	mu sync.Mutex
}

// Open returns a Store backed by path. A missing file is an empty store.
func Open(path string) *Store { return &Store{path: path} }

// Path returns the registry file location.
func (s *Store) Path() string { return s.path }

type fileSchema struct {
	Allocations []Allocation `json:"allocations"`
}

func (s *Store) load() (*fileSchema, error) {
	buf, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &fileSchema{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", s.path, err)
	}
	var f fileSchema
	if len(bytes.TrimSpace(buf)) == 0 {
		return &f, nil
	}
	if err := json.Unmarshal(buf, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return &f, nil
}

// save writes the registry atomically (tmp file + rename) at mode 0600.
func (s *Store) save(f *fileSchema) (rerr error) {
	buf, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.path, err)
	}
	buf = append(buf, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".pool-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if rerr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if _, err := tmp.Write(buf); err != nil {
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	// A pre-existing destination keeps its (possibly wider) mode across
	// Rename on some filesystems; narrow it.
	return os.Chmod(s.path, 0o600)
}

// update runs mutate under both locks on a freshly loaded registry and
// persists the result when mutate reports a change.
func (s *Store) update(mutate func(f *fileSchema) (changed bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	lockF, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer lockF.Close()
	if err := flockExclusive(lockF); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer flockUnlock(lockF) //nolint:errcheck // unlock-on-close is sufficient

	f, err := s.load()
	if err != nil {
		return err
	}
	changed, err := mutate(f)
	if err != nil || !changed {
		return err
	}
	return s.save(f)
}

// Allocate gives the worker the first free database and port. It is
// idempotent: an existing allocation for the same (worker, task) is returned
// unchanged. A checkout already registered to a different worker is refused
// with ErrCheckoutInUse rather than merged.
func (s *Store) Allocate(cfg Config, a Allocation) (Allocation, error) {
	if a.Worker == "" {
		return Allocation{}, errors.New("pool: allocation needs a worker")
	}
	if a.Checkout == "" {
		return Allocation{}, errors.New("pool: allocation needs a checkout path")
	}
	checkout, err := filepath.Abs(a.Checkout)
	if err != nil {
		return Allocation{}, fmt.Errorf("resolve checkout %q: %w", a.Checkout, err)
	}
	a.Checkout = checkout

	var out Allocation
	err = s.update(func(f *fileSchema) (bool, error) {
		for _, e := range f.Allocations {
			if e.Worker == a.Worker && e.TaskID == a.TaskID {
				out = e
				return false, nil
			}
		}
		usedDB := map[string]bool{}
		usedPort := map[int]bool{}
		for _, e := range f.Allocations {
			if e.Checkout == a.Checkout && e.Worker != a.Worker {
				return false, fmt.Errorf("%w: %s is held by %s (task %d)", ErrCheckoutInUse, a.Checkout, e.Worker, e.TaskID)
			}
			usedDB[e.Database] = true
			usedPort[e.Port] = true
		}
		db, ok := firstFree(cfg.Databases, usedDB)
		if !ok {
			return false, fmt.Errorf("%w: no free database (%d configured)", ErrPoolExhausted, len(cfg.Databases))
		}
		port, ok := firstFree(cfg.Ports, usedPort)
		if !ok {
			return false, fmt.Errorf("%w: no free port (%d configured)", ErrPoolExhausted, len(cfg.Ports))
		}
		a.Database = db
		a.Port = port
		if a.Since.IsZero() {
			a.Since = time.Now().UTC().Truncate(time.Second)
		}
		f.Allocations = append(f.Allocations, a)
		out = a
		return true, nil
	})
	if err != nil {
		return Allocation{}, err
	}
	return out, nil
}

func firstFree[T comparable](pool []T, used map[T]bool) (T, bool) {
	for _, v := range pool {
		if !used[v] {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// ReleaseTask frees every allocation for the task and returns what was freed.
func (s *Store) ReleaseTask(taskID int64) ([]Allocation, error) {
	return s.release(func(a Allocation) bool { return a.TaskID == taskID })
}

// ReleaseWorker frees every allocation held by the worker and returns what
// was freed.
func (s *Store) ReleaseWorker(worker string) ([]Allocation, error) {
	return s.release(func(a Allocation) bool { return a.Worker == worker })
}

func (s *Store) release(match func(Allocation) bool) ([]Allocation, error) {
	freed := []Allocation{}
	err := s.update(func(f *fileSchema) (bool, error) {
		kept := f.Allocations[:0]
		for _, e := range f.Allocations {
			if match(e) {
				freed = append(freed, e)
			} else {
				kept = append(kept, e)
			}
		}
		f.Allocations = kept
		return len(freed) > 0, nil
	})
	if err != nil {
		return nil, err
	}
	return freed, nil
}

// List returns every current allocation in registration order.
func (s *Store) List() ([]Allocation, error) {
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	if f.Allocations == nil {
		return []Allocation{}, nil
	}
	return f.Allocations, nil
}

// Conflicts lists checkouts registered to more than one worker (impossible
// through Allocate; exists for imported or hand-edited state) and
// allocations whose checkout no longer exists on disk.
func (s *Store) Conflicts() ([]Conflict, error) {
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	var order []string
	workers := map[string][]string{}
	for _, a := range f.Allocations {
		ws, seen := workers[a.Checkout]
		if !seen {
			order = append(order, a.Checkout)
		}
		if !slices.Contains(ws, a.Worker) {
			ws = append(ws, a.Worker)
		}
		workers[a.Checkout] = ws
	}
	var out []Conflict
	for _, checkout := range order {
		ws := workers[checkout]
		missing := checkoutMissing(checkout)
		if len(ws) > 1 || missing {
			out = append(out, Conflict{Checkout: checkout, Workers: ws, Missing: missing})
		}
	}
	return out, nil
}

func checkoutMissing(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return !info.IsDir()
}
