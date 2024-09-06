//go:build sqlite

package db

import (
	//"github.com/glebarez/sqlite"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
)

func init() {
	Factory["sqlite"] = gormlite.Open
}
