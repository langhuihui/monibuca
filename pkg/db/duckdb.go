//go:build duckdb

package db

import "github.com/c0deltin/duckdb-driver/duckdb"

func init() {
	Factory["duckdb"] = duckdb.Open
}
