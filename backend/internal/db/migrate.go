package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending SQL migrations. It is idempotent and safe to
// run on every boot (api + worker). Dev/test SQLite flows keep using GORM
// AutoMigrate; this path is Postgres-only.
func Migrate(sqlDB *sql.DB) error {
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
