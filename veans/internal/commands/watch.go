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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/output"
)

// watchEvent is one line of 'veans watch' output.
type watchEvent struct {
	Event string                `json:"event"`
	Task  *client.Task          `json:"task,omitempty"`
	Lease *client.TaskPathLease `json:"lease,omitempty"`
	At    time.Time             `json:"at"`
}

func newWatchCmd() *cobra.Command {
	var (
		interval time.Duration
		execCmd  string
		once     bool
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Follow the ready queue and leases; one JSON line per change",
		Long: `Polls the server and prints a JSON line whenever the queue changes:

  {"event":"task.ready",   "task":{...}}   a task became claimable
  {"event":"task.unready", "task":{...}}   a ready task was claimed, blocked or removed
  {"event":"lease.stale",  "lease":{...}}  a holder went quiet on its files

This is the orchestrator hook: pipe it into whatever wakes an idle agent, or
use --exec to run a command for every event (the event JSON arrives on its
stdin and as $VEANS_EVENT). --once prints the current ready tasks as
task.ready events and exits, for cron.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			if interval < 5*time.Second {
				return output.New(output.CodeValidation, "--interval must be at least 5s")
			}
			w := &watcher{rt: rt, out: cmd.OutOrStdout(), execCmd: execCmd, readyTasks: map[int64]bool{}, staleLeases: map[int64]bool{}}
			if err := w.tick(cmd.Context(), true); err != nil {
				return err
			}
			if once {
				return nil
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-ticker.C:
					if err := w.tick(cmd.Context(), false); err != nil {
						// A transient server error must not kill a long-running
						// watcher; report it as an event and keep polling.
						_ = w.emit(cmd.Context(), watchEvent{Event: "error", At: time.Now()}, err.Error())
					}
				}
			}
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "how often to poll")
	cmd.Flags().StringVar(&execCmd, "exec", "", "shell command to run for every event (event JSON on stdin and in $VEANS_EVENT)")
	cmd.Flags().BoolVar(&once, "once", false, "emit the current ready tasks and exit")
	return cmd
}

type watcher struct {
	rt          *runtime
	out         interface{ Write([]byte) (int, error) }
	execCmd     string
	readyTasks  map[int64]bool
	staleLeases map[int64]bool
	prevTasks   map[int64]*client.Task
}

// tick diffs the queue and leases against the previous poll. The first
// tick reports everything currently ready so a fresh watcher wakes agents
// for work that was already waiting.
func (w *watcher) tick(ctx context.Context, first bool) error {
	rows, err := readyQueue(ctx, w.rt)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, r := range rows {
		if r.Task == nil || !r.Ready {
			continue
		}
		seen[r.Task.ID] = true
		if !w.readyTasks[r.Task.ID] {
			w.readyTasks[r.Task.ID] = true
			if err := w.emit(ctx, watchEvent{Event: "task.ready", Task: r.Task, At: time.Now()}, ""); err != nil {
				return err
			}
		}
	}
	for id := range w.readyTasks {
		if !seen[id] {
			delete(w.readyTasks, id)
			if !first {
				if err := w.emit(ctx, watchEvent{Event: "task.unready", Task: w.prevTasks[id], At: time.Now()}, ""); err != nil {
					return err
				}
			}
		}
	}
	w.prevTasks = map[int64]*client.Task{}
	for _, r := range rows {
		if r.Task != nil {
			w.prevTasks[r.Task.ID] = r.Task
		}
	}

	leases, err := w.rt.client.ListProjectLeases(ctx, w.rt.cfg.ProjectID)
	if err != nil {
		return err
	}
	current := map[int64]bool{}
	for _, l := range leases {
		if !l.Stale {
			continue
		}
		current[l.ID] = true
		if !w.staleLeases[l.ID] {
			w.staleLeases[l.ID] = true
			if err := w.emit(ctx, watchEvent{Event: "lease.stale", Lease: l, At: time.Now()}, ""); err != nil {
				return err
			}
		}
	}
	for id := range w.staleLeases {
		if !current[id] {
			delete(w.staleLeases, id)
		}
	}
	return nil
}

func (w *watcher) emit(ctx context.Context, ev watchEvent, detail string) error {
	line, err := json.Marshal(struct {
		watchEvent
		Detail string `json:"detail,omitempty"`
	}{ev, detail})
	if err != nil {
		return err
	}
	if _, err := w.out.Write(append(line, '\n')); err != nil {
		return err
	}
	if w.execCmd == "" || ev.Event == "error" {
		return nil
	}
	c := exec.CommandContext(ctx, "sh", "-c", w.execCmd) //nolint:gosec // --exec is the operator's own command line
	c.Stdin = bytes.NewReader(line)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	c.Env = append(os.Environ(), "VEANS_EVENT="+string(line))
	if err := c.Run(); err != nil {
		// The hook's failure is its own business; the watcher keeps going.
		_, _ = os.Stderr.WriteString("veans watch: --exec failed: " + err.Error() + "\n")
	}
	return nil
}
