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

// Mirrors models.TaskReceipt. Gates is a json column like task_scopes' lists.
type taskReceipts20260902120300 struct {
	ID                 int64     `xorm:"bigint autoincr not null unique pk"`
	TaskID             int64     `xorm:"bigint not null index"`
	ProjectID          int64     `xorm:"bigint not null index"`
	CommitSHA          string    `xorm:"varchar(64) not null"`
	Branch             string    `xorm:"varchar(255) null"`
	Gates              []byte    `xorm:"json null"`
	DocsAPIRequired    bool      `xorm:"not null default false"`
	DocsAPIRegenerated bool      `xorm:"not null default false"`
	CIRunURL           string    `xorm:"varchar(2048) null"`
	Merged             bool      `xorm:"not null default false"`
	MergeSHA           string    `xorm:"varchar(64) null"`
	Passed             bool      `xorm:"not null default false"`
	PostedByID         int64     `xorm:"bigint not null"`
	Created            time.Time `xorm:"created not null"`
}

func (taskReceipts20260902120300) TableName() string {
	return "task_receipts"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260902120300",
		Description: "Add task receipts: CI's record of the gates a change passed",
		Migrate: func(tx *xorm.Engine) error {
			return tx.Sync2(taskReceipts20260902120300{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return tx.DropTables(taskReceipts20260902120300{})
		},
	})
}
