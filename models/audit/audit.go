// Package audit is the log of changes made in the admin pages.
package audit

import (
	"context"
	"log/slog"

	"git.cu3rd.ru/cu3rd/dash/models/db"
)

// Audit is one change.
type Audit struct {
	ID          int64  `xorm:"pk autoincr"`
	Actor       string `xorm:"VARCHAR(64) NOT NULL"`
	Action      string `xorm:"VARCHAR(64) NOT NULL"`
	Details     string `xorm:"VARCHAR(1000) NOT NULL DEFAULT ''"`
	CreatedUnix int64  `xorm:"INDEX created"`
}

func init() {
	db.RegisterModel(new(Audit))
}

// Record stores a change; a failure is logged and does not undo the change itself.
func Record(ctx context.Context, actor, action, details string) {
	if len(details) > 1000 {
		details = details[:1000]
	}
	if _, err := db.GetEngine(ctx).Insert(&Audit{Actor: actor, Action: action, Details: details}); err != nil {
		slog.Error("record audit", "action", action, "err", err)
	}
	slog.Info("audit", "actor", actor, "action", action, "details", details)
}

// List returns the latest changes, newest first.
func List(ctx context.Context, limit int) ([]*Audit, error) {
	list := make([]*Audit, 0, limit)
	return list, db.GetEngine(ctx).Desc("id").Limit(limit).Find(&list)
}
