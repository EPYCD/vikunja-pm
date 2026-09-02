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
package migration

import (
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type taskBucketsMovedBy20260902120100 struct {
	MovedByID int64     `xorm:"bigint null"`
	MovedAt   time.Time `xorm:"datetime null"`
}

func (taskBucketsMovedBy20260902120100) TableName() string {
	return "task_buckets"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260902120100",
		Description: "Record who moved a task into its bucket, so the reviewer rule can be checked",
		Migrate: func(tx *xorm.Engine) error {
			return partialSync(tx, taskBucketsMovedBy20260902120100{})
		},
		Rollback: func(tx *xorm.Engine) error {
			if err := dropTableColum(tx, "task_buckets", "moved_by_id"); err != nil {
				return err
			}
			return dropTableColum(tx, "task_buckets", "moved_at")
		},
	})
}
