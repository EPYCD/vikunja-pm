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

package pool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func testConfig(dbs, ports int) Config {
	var c Config
	for i := range dbs {
		c.Databases = append(c.Databases, fmt.Sprintf("mongodb://127.0.0.1:27101/w%d", i+1))
	}
	for i := range ports {
		c.Ports = append(c.Ports, 3100+i)
	}
	return c
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []int
		err  string
	}{
		{name: "ints and ranges", in: "ports: [3100, \"3105-3107\", \" 3110 \"]", want: []int{3100, 3105, 3106, 3107, 3110}},
		{name: "scalar range", in: "ports: 3100-3102", want: []int{3100, 3101, 3102}},
		{name: "scalar int", in: "ports: 3100", want: []int{3100}},
		{name: "absent", in: "databases: [a]", want: nil},
		{name: "null", in: "ports:", want: nil},
		{name: "reversed range", in: "ports: [3110-3100]", err: "start is after end"},
		{name: "garbage", in: "ports: [abc]", err: "invalid port"},
		{name: "out of range", in: "ports: [70000]", err: "out of range"},
		{name: "mapping", in: "ports: {a: 1}", err: "expected a port"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			err := yaml.Unmarshal([]byte(tc.in), &c)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want containing %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(c.Ports, tc.want) {
				t.Fatalf("ports = %v, want %v", c.Ports, tc.want)
			}
		})
	}

	var c Config
	if err := yaml.Unmarshal([]byte("databases: [x, y]\nports: [1]"), &c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Databases, []string{"x", "y"}) {
		t.Fatalf("databases = %v", c.Databases)
	}
}

func TestStore_MissingFileIsEmpty(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "nope", "pool.json"))
	list, err := s.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %v, %v", list, err)
	}
	conflicts, err := s.Conflicts()
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("Conflicts = %v, %v", conflicts, err)
	}
	freed, err := s.ReleaseTask(1)
	if err != nil || len(freed) != 0 {
		t.Fatalf("ReleaseTask = %v, %v", freed, err)
	}
}

func TestStore_AllocateTwoWorkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "pool.json")
	s := Open(path)
	cfg := testConfig(2, 2)

	a, err := s.Allocate(cfg, Allocation{Worker: "bot-a", TaskID: 1, Story: "E5.3", Branch: "e5.3-x", Checkout: mkdir(t, dir, "a")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Allocate(cfg, Allocation{Worker: "bot-b", TaskID: 2, Story: "E5.4", Branch: "e5.4-y", Checkout: mkdir(t, dir, "b")})
	if err != nil {
		t.Fatal(err)
	}
	if a.Database != cfg.Databases[0] || a.Port != cfg.Ports[0] {
		t.Fatalf("first allocation got %s/%d", a.Database, a.Port)
	}
	if b.Database != cfg.Databases[1] || b.Port != cfg.Ports[1] {
		t.Fatalf("second allocation got %s/%d", b.Database, b.Port)
	}
	if a.Since.IsZero() || !filepath.IsAbs(a.Checkout) {
		t.Fatalf("allocation not normalized: %+v", a)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || !reflect.DeepEqual(list[0], a) || !reflect.DeepEqual(list[1], b) {
		t.Fatalf("List = %+v", list)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	if _, ok := shape["allocations"]; !ok || len(shape) != 1 {
		t.Fatalf("file shape = %s", raw)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
		}
	}
}

func TestStore_Exhausted(t *testing.T) {
	dir := t.TempDir()

	s := Open(filepath.Join(dir, "db.json"))
	cfg := testConfig(1, 2)
	if _, err := s.Allocate(cfg, Allocation{Worker: "a", TaskID: 1, Checkout: mkdir(t, dir, "a")}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Allocate(cfg, Allocation{Worker: "b", TaskID: 2, Checkout: mkdir(t, dir, "b")})
	if !errors.Is(err, ErrPoolExhausted) || !strings.Contains(err.Error(), "database") {
		t.Fatalf("err = %v, want ErrPoolExhausted naming database", err)
	}

	s = Open(filepath.Join(dir, "port.json"))
	cfg = testConfig(2, 1)
	if _, err := s.Allocate(cfg, Allocation{Worker: "a", TaskID: 1, Checkout: mkdir(t, dir, "c")}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Allocate(cfg, Allocation{Worker: "b", TaskID: 2, Checkout: mkdir(t, dir, "d")})
	if !errors.Is(err, ErrPoolExhausted) || !strings.Contains(err.Error(), "port") {
		t.Fatalf("err = %v, want ErrPoolExhausted naming port", err)
	}

	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("failed allocation must not persist: %v, %v", list, err)
	}
}

func TestStore_CheckoutInUse(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "pool.json"))
	cfg := testConfig(2, 2)
	shared := mkdir(t, dir, "shared")

	if _, err := s.Allocate(cfg, Allocation{Worker: "a", TaskID: 1, Checkout: shared}); err != nil {
		t.Fatal(err)
	}
	// Same directory spelled differently must still be recognised.
	_, err := s.Allocate(cfg, Allocation{Worker: "b", TaskID: 2, Checkout: filepath.Join(shared, "..", "shared")})
	if !errors.Is(err, ErrCheckoutInUse) {
		t.Fatalf("err = %v, want ErrCheckoutInUse", err)
	}
	if !strings.Contains(err.Error(), "held by a") {
		t.Fatalf("error should name the holder: %v", err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("refused allocation must not be merged: %v, %v", list, err)
	}
}

func TestStore_AllocateIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "pool.json"))
	cfg := testConfig(2, 2)
	req := Allocation{Worker: "a", TaskID: 1, Story: "E1.1", Checkout: mkdir(t, dir, "a")}

	first, err := s.Allocate(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Allocate(cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("re-allocate changed the record:\n%+v\n%+v", first, again)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}
	// Same worker, new task: a second, distinct allocation.
	other, err := s.Allocate(cfg, Allocation{Worker: "a", TaskID: 2, Checkout: mkdir(t, dir, "b")})
	if err != nil {
		t.Fatal(err)
	}
	if other.Database == first.Database || other.Port == first.Port {
		t.Fatalf("second task reused resources: %+v", other)
	}
}

func TestStore_Release(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "pool.json"))
	cfg := testConfig(3, 3)
	for i, w := range []string{"a", "a", "b"} {
		if _, err := s.Allocate(cfg, Allocation{Worker: w, TaskID: int64(i + 1), Checkout: mkdir(t, dir, fmt.Sprint("wt", i))}); err != nil {
			t.Fatal(err)
		}
	}

	freed, err := s.ReleaseTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 1 || freed[0].TaskID != 1 || freed[0].Database != cfg.Databases[0] {
		t.Fatalf("ReleaseTask freed %+v", freed)
	}
	// The freed slot is the first free one again.
	c, err := s.Allocate(cfg, Allocation{Worker: "c", TaskID: 9, Checkout: mkdir(t, dir, "wt9")})
	if err != nil {
		t.Fatal(err)
	}
	if c.Database != cfg.Databases[0] || c.Port != cfg.Ports[0] {
		t.Fatalf("freed slot not reused: %+v", c)
	}

	freed, err = s.ReleaseWorker("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 1 || freed[0].TaskID != 2 {
		t.Fatalf("ReleaseWorker freed %+v", freed)
	}
	freed, err = s.ReleaseWorker("nobody")
	if err != nil || len(freed) != 0 {
		t.Fatalf("ReleaseWorker(nobody) = %v, %v", freed, err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Worker != "b" || list[1].Worker != "c" {
		t.Fatalf("List = %+v", list)
	}
}

func TestStore_Conflicts(t *testing.T) {
	dir := t.TempDir()
	shared := mkdir(t, dir, "shared")
	solo := mkdir(t, dir, "solo")
	gone := filepath.Join(dir, "gone")
	path := filepath.Join(dir, "pool.json")
	raw := fmt.Sprintf(`{"allocations":[
  {"worker":"alice","task_id":1,"checkout":%q,"database":"db1","port":1},
  {"worker":"bob","task_id":2,"checkout":%q,"database":"db2","port":2},
  {"worker":"carol","task_id":3,"checkout":%q,"database":"db3","port":3},
  {"worker":"dave","task_id":4,"checkout":%q,"database":"db4","port":4}
]}`, shared, shared, gone, solo)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Open(path).Conflicts()
	if err != nil {
		t.Fatal(err)
	}
	want := []Conflict{
		{Checkout: shared, Workers: []string{"alice", "bob"}},
		{Checkout: gone, Workers: []string{"carol"}, Missing: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Conflicts = %+v, want %+v", got, want)
	}
}

func TestStore_ConcurrentAllocate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pool.json")
	cfg := testConfig(8, 8)
	results := make([]Allocation, 8)
	errs := make([]error, 8)

	var wg sync.WaitGroup
	for i := range 8 {
		checkout := mkdir(t, dir, fmt.Sprint("wt", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A fresh Store per goroutine so only the file lock serializes them.
			results[i], errs[i] = Open(path).Allocate(cfg, Allocation{Worker: fmt.Sprint("w", i), TaskID: int64(i), Checkout: checkout})
		}()
	}
	wg.Wait()

	dbs := map[string]bool{}
	ports := map[int]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		dbs[results[i].Database] = true
		ports[results[i].Port] = true
	}
	if len(dbs) != 8 || len(ports) != 8 {
		t.Fatalf("double assignment: %d distinct databases, %d distinct ports", len(dbs), len(ports))
	}
	list, err := Open(path).List()
	if err != nil || len(list) != 8 {
		t.Fatalf("List = %d entries, %v", len(list), err)
	}
}
