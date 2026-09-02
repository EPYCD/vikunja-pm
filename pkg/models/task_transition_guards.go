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
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// claimOnBucketEntry is the ownership lock on the board itself. Whatever
// moved the task into the view's claim bucket — a drag, a v1 bucket move,
// a project transfer — gets the same refusals as POST /claim and leaves the
// task holding its paths. The holder is the task's sole assignee when it has
// one, so a lead can start a teammate's task for them; otherwise the mover.
func claimOnBucketEntry(s *xorm.Session, a web.Auth, task *Task) error {
	blockers, err := unfinishedBlockers(s, task.ID)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return ErrTaskBlocked{TaskID: task.ID, BlockerIDs: blockers}
	}

	assignees := []*TaskAssginee{}
	if err := s.Where("task_id = ?", task.ID).Find(&assignees); err != nil {
		return err
	}
	holderID := a.GetID()
	switch len(assignees) {
	case 0:
		doer, err := user.GetFromAuth(a)
		if err != nil {
			return err
		}
		project, err := GetProjectSimpleByID(s, task.ProjectID)
		if err != nil {
			return err
		}
		if err := task.addNewAssigneeByID(s, doer.ID, project, a); err != nil {
			return err
		}
	case 1:
		holderID = assignees[0].UserID
	default:
		mine := false
		for _, as := range assignees {
			if as.UserID == a.GetID() {
				mine = true
			}
		}
		if !mine {
			return ErrTaskAlreadyClaimed{TaskID: task.ID, UserID: assignees[0].UserID}
		}
	}

	scope, err := getTaskScope(s, task.ID)
	if err != nil {
		return err
	}
	if scope != nil && len(scope.PathsOwned) > 0 {
		if err := acquirePathLeasesForTask(s, task, holderID, scope.PathsOwned); err != nil {
			return err
		}
	}
	if err := touchLeasesHeldBy(s, task.ID, holderID); err != nil {
		return err
	}

	holder, err := user.GetUserByID(s, holderID)
	if err != nil {
		return err
	}
	events.DispatchOnCommit(s, &TaskClaimedEvent{Task: task, Doer: holder})
	return nil
}

// checkDoneAllowed is the gate on "done" for projects that name a receipt
// bot: only a merged, passing receipt from CI ends a task, and never the
// person who put it up for review. current is the bucket row the task is
// leaving when the transition is a bucket move; nil for a plain update, in
// which case every kanban row of the task is consulted.
func checkDoneAllowed(s *xorm.Session, a web.Auth, task *Task, current *TaskBucket) error {
	project, err := GetProjectSimpleByID(s, task.ProjectID)
	if err != nil {
		return err
	}
	if project.ReceiptBotID == 0 {
		return nil
	}

	has, err := s.Where("task_id = ? AND merged = ? AND passed = ?", task.ID, true, true).Exist(&TaskReceipt{})
	if err != nil {
		return err
	}
	if !has {
		return ErrTaskDoneRequiresReceipt{TaskID: task.ID}
	}

	// CI is the gate, not a worker; the submitter rule is about people.
	if a.GetID() == project.ReceiptBotID {
		return nil
	}
	rows := []*TaskBucket{}
	if current != nil {
		rows = append(rows, current)
	} else if err := s.Where("task_id = ?", task.ID).Find(&rows); err != nil {
		return err
	}
	for _, r := range rows {
		if r.MovedByID != 0 && r.MovedByID == a.GetID() {
			return ErrTaskDoneBySubmitter{TaskID: task.ID, UserID: a.GetID()}
		}
	}
	return nil
}
