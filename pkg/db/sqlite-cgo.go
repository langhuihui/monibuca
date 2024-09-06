//go:build sqliteCGO

package db

import (
	sqlite "github.com/mattn/go-sqlite3"
)

func init() {
	Factory["sqlite"] = sqlite.Open
}
