//go:build sqlite

package db

import (
	"github.com/glebarez/sqlite"
)

func init() {
	Factory["sqlite"] = sqlite.Open
}
