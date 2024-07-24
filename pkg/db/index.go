package db

import "gorm.io/gorm"

var Factory = map[string]func(string) gorm.Dialector{}
