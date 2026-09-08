// Package migrations содержит SQL-миграции goose
package migrations

import "embed"

// FS - файловая система с миграциями для goose
//
//go:embed *.sql
var FS embed.FS
