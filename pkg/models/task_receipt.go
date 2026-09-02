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

package models

import (
	"regexp"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// Gate statuses a receipt may report.
const (
	GateStatusPassed  = "passed"
	GateStatusFailed  = "failed"
	GateStatusSkipped = "skipped"
)

var commitSHARe = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// GateResult is one gate of a CI run.
type GateResult struct {
	Name       string `json:"name" minLength:"1" maxLength:"50" doc:"The gate, e.g. typecheck, lint, test, build, docs:api."`
	Status     string `json:"status" enum:"passed,failed,skipped" doc:"passed, failed or skipped."`
	DurationMS int64  `json:"duration_ms" minimum:"0" doc:"How long the gate ran, in milliseconds."`
}

// TaskReceipt is CI's record that a change passed (or failed) the gates.
// Only the project's receipt bot can post one, nothing can change or delete
// it, and while a project names a receipt bot a task cannot be marked done
// without a merged, passing one.
type TaskReceipt struct {
	ID        int64 `xorm:"bigint autoincr not null unique pk" json:"id" param:"receipt" readOnly:"true" doc:"The unique, numeric id of this receipt."`
	TaskID    int64 `xorm:"bigint not null index" json:"task_id" param:"projecttask" doc:"The task the receipt belongs to. On /api/v2 this is taken from the URL."`
	ProjectID int64 `xorm:"bigint not null index" json:"project_id" readOnly:"true" doc:"The task's project. Set by the server."`

	CommitSHA string       `xorm:"varchar(64) not null" json:"commit_sha" required:"true" minLength:"7" maxLength:"64" doc:"The commit the gates ran on."`
	Branch    string       `xorm:"varchar(255) null" json:"branch" maxLength:"255" doc:"The branch the commit was on."`
	Gates     []GateResult `xorm:"json null" json:"gates" required:"true" minItems:"1" maxItems:"20" doc:"One entry per gate, with status and duration."`
	// Derived from the diff by CI, never declared by the worker.
	DocsAPIRequired    bool   `xorm:"not null default false" json:"docs_api_required" doc:"True when the diff touched files that require regenerating the API docs."`
	DocsAPIRegenerated bool   `xorm:"not null default false" json:"docs_api_regenerated" doc:"True when the API docs were regenerated in the change."`
	CIRunURL           string `xorm:"varchar(2048) null" json:"ci_run_url" maxLength:"2048" doc:"Link to the CI run."`
	Merged             bool   `xorm:"not null default false" json:"merged" doc:"True when the commit is on the merged branch. Only a merged, passing receipt lets a task be marked done."`
	MergeSHA           string `xorm:"varchar(64) null" json:"merge_sha" maxLength:"64" doc:"The merge commit, when merged."`
	Passed             bool   `xorm:"not null default false" json:"passed" readOnly:"true" doc:"True when every gate passed and, if required, the API docs were regenerated. Computed by the server."`

	PostedByID int64      `xorm:"bigint not null" json:"-"`
	PostedBy   *user.User `xorm:"-" json:"posted_by" readOnly:"true" doc:"The receipt bot that posted it."`
	Created    time.Time  `xorm:"created not null" json:"created" readOnly:"true" doc:"When the receipt was posted."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// TableName returns the table name for task receipts
func (tr *TaskReceipt) TableName() string {
	return "task_receipts"
}

// CanCreate: only the project's receipt bot. A project without one accepts
// no receipts at all.
func (tr *TaskReceipt) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	project, err := GetProjectSimpleByTaskID(s, tr.TaskID)
	if err != nil {
		return false, err
	}
	if project.ReceiptBotID == 0 || a.GetID() != project.ReceiptBotID {
		return false, nil
	}
	tr.ProjectID = project.ID
	return true, nil
}

// CanRead follows the task.
func (tr *TaskReceipt) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	return (&Task{ID: tr.TaskID}).CanRead(s, a)
}

// Create validates and stores the receipt. Receipts are append-only: there
// is no Update or Delete.
func (tr *TaskReceipt) Create(s *xorm.Session, a web.Auth) error {
	if err := tr.validate(); err != nil {
		return err
	}
	task, err := GetTaskByIDSimple(s, tr.TaskID)
	if err != nil {
		return err
	}
	tr.ID = 0
	tr.ProjectID = task.ProjectID
	tr.PostedByID = a.GetID()
	tr.Passed = tr.computePassed()
	if _, err := s.Insert(tr); err != nil {
		return err
	}
	doer, err := user.GetUserByID(s, a.GetID())
	if err != nil {
		return err
	}
	tr.PostedBy = doer
	events.DispatchOnCommit(s, &TaskReceiptCreatedEvent{Task: &task, Receipt: tr, Doer: doer})
	return nil
}

func (tr *TaskReceipt) validate() error {
	tr.CommitSHA = strings.TrimSpace(tr.CommitSHA)
	if !commitSHARe.MatchString(tr.CommitSHA) {
		return ErrReceiptInvalid{Reason: "commit_sha must be a 7 to 64 character hex sha"}
	}
	if tr.MergeSHA != "" && !commitSHARe.MatchString(strings.TrimSpace(tr.MergeSHA)) {
		return ErrReceiptInvalid{Reason: "merge_sha must be a 7 to 64 character hex sha"}
	}
	if len(tr.Gates) == 0 {
		return ErrReceiptInvalid{Reason: "at least one gate is required"}
	}
	for i := range tr.Gates {
		g := &tr.Gates[i]
		g.Name = strings.TrimSpace(g.Name)
		if g.Name == "" {
			return ErrReceiptInvalid{Reason: "every gate needs a name"}
		}
		switch g.Status {
		case GateStatusPassed, GateStatusFailed, GateStatusSkipped:
		default:
			return ErrReceiptInvalid{Reason: "gate " + g.Name + " has an unknown status; use passed, failed or skipped"}
		}
		if g.DurationMS < 0 {
			return ErrReceiptInvalid{Reason: "gate " + g.Name + " has a negative duration"}
		}
	}
	return nil
}

func (tr *TaskReceipt) computePassed() bool {
	for _, g := range tr.Gates {
		if g.Status != GateStatusPassed {
			return false
		}
	}
	return !tr.DocsAPIRequired || tr.DocsAPIRegenerated
}

// ReadAll lists the task's receipts, newest first. Not paginated: a task
// accumulates a handful over its life.
func (tr *TaskReceipt) ReadAll(s *xorm.Session, a web.Auth, _ string, _ int, _ int) (result interface{}, resultCount int, totalItems int64, err error) {
	can, _, err := tr.CanRead(s, a)
	if err != nil {
		return nil, 0, 0, err
	}
	if !can {
		return nil, 0, 0, ErrGenericForbidden{}
	}
	receipts := []*TaskReceipt{}
	if err := s.Where("task_id = ?", tr.TaskID).OrderBy("created DESC, id DESC").Find(&receipts); err != nil {
		return nil, 0, 0, err
	}
	if err := embedReceiptPosters(s, receipts); err != nil {
		return nil, 0, 0, err
	}
	return receipts, len(receipts), int64(len(receipts)), nil
}

func embedReceiptPosters(s *xorm.Session, receipts []*TaskReceipt) error {
	if len(receipts) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(receipts))
	for _, r := range receipts {
		ids = append(ids, r.PostedByID)
	}
	users, err := user.GetUsersByIDs(s, ids)
	if err != nil {
		return err
	}
	for _, r := range receipts {
		r.PostedBy = users[r.PostedByID]
	}
	return nil
}
