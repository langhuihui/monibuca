//go:build sqlite

package db

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func init() {
	Factory["sqlite"] = func(dsn string) gorm.Dialector {
		return sqlite.Open(dsn)
	}
}
