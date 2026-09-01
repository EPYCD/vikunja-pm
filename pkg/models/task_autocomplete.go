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
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// completeParentsIfSubtasksDone marks every parent of a just-finished task
// done once none of that parent's subtasks are open, and walks up the tree.
// Off unless service.autocompleteparenttasks is set: on a human board a
// parent often carries work of its own, on an agent board it is a container.
// Repeating parents are skipped — closing them would schedule the next run.
func completeParentsIfSubtasksDone(s *xorm.Session, a web.Auth, task *Task) error {
	if !config.ServiceAutoCompleteParentTasks.GetBool() {
		return nil
	}
	parents := []*TaskRelation{}
	if err := s.Where("task_id = ? AND relation_kind = ?", task.ID, RelationKindParenttask).Find(&parents); err != nil {
		return err
	}
	if len(parents) == 0 {
		return nil
	}
	doer, err := user.GetFromAuth(a)
	if err != nil {
		return err
	}
	for _, rel := range parents {
		parent, err := GetTaskByIDSimple(s, rel.OtherTaskID)
		if err != nil {
			if IsErrTaskDoesNotExist(err) {
				continue
			}
			return err
		}
		if parent.Done || parent.isRepeating() {
			continue
		}
		open, err := openSubtaskCount(s, parent.ID)
		if err != nil {
			return err
		}
		if open > 0 {
			continue
		}
		if err := completeTask(s, a, doer, &parent); err != nil {
			return err
		}
		if err := completeParentsIfSubtasksDone(s, a, &parent); err != nil {
			return err
		}
	}
	return nil
}

func openSubtaskCount(s *xorm.Session, parentID int64) (int64, error) {
	relations := []*TaskRelation{}
	if err := s.Where("task_id = ? AND relation_kind = ?", parentID, RelationKindSubtask).Find(&relations); err != nil {
		return 0, err
	}
	if len(relations) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(relations))
	for _, r := range relations {
		ids = append(ids, r.OtherTaskID)
	}
	return s.In("id", ids).Where("done = ?", false).Count(&Task{})
}

// completeTask does the minimal done transition — flag, timestamp, done
// buckets, leases, event — without re-entering Task.Update, which would
// also rewrite assignees and reminders it never loaded.
func completeTask(s *xorm.Session, a web.Auth, doer *user.User, task *Task) error {
	task.Done = true
	task.DoneAt = time.Now()
	if _, err := s.ID(task.ID).Cols("done", "done_at").Update(task); err != nil {
		return err
	}
	if err := releasePathLeasesForTask(s, task.ID); err != nil {
		return err
	}
	views := []*ProjectView{}
	if err := s.
		Where("project_id = ? AND view_kind = ? AND bucket_configuration_mode = ?",
			task.ProjectID, ProjectViewKindKanban, BucketConfigurationModeManual).
		Find(&views); err != nil {
		return err
	}
	if err := task.moveTaskToDoneBuckets(s, a, views); err != nil {
		return err
	}
	events.DispatchOnCommit(s, &TaskUpdatedEvent{Task: task, Doer: doer})
	return nil
}
