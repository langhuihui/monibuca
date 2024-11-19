//go:build mysql

package db

import (
	"database/sql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func init() {
	Factory["mysql"] = func(s string) gorm.Dialector {
		sqlDB, _ := sql.Open("mysql", s)
		return mysql.New(mysql.Config{
			Conn: sqlDB,
		})
	}
}
