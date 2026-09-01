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

type taskPathLeases20260901233318 struct {
	LastActive time.Time `xorm:"datetime null"`
}

func (taskPathLeases20260901233318) TableName() string {
	return "task_path_leases"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260901233318",
		Description: "Add last_active to task_path_leases so stale leases left by crashed agents can be spotted",
		Migrate: func(tx *xorm.Engine) error {
			if err := partialSync(tx, taskPathLeases20260901233318{}); err != nil {
				return err
			}
			// Existing leases were active when they were taken; without a
			// value they would all read as stale the moment the column lands.
			_, err := tx.Table("task_path_leases").Where("last_active IS NULL").Update(map[string]interface{}{"last_active": time.Now()})
			return err
		},
		Rollback: func(tx *xorm.Engine) error {
			return dropTableColum(tx, "task_path_leases", "last_active")
		},
	})
}
