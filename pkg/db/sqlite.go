//go:build sqlite

package db

import "github.com/glebarez/sqlite"

func init() {
	Factory["sqlite"] = func(dsn string) gorm.Dialector {
		return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	}
}
