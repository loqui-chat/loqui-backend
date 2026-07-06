package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migration files are named NNNN_name.sql and split on a down marker:
//
// -- +migrate up
// ...
// -- +migrate down
// ...
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

const (
	upMarker   = "-- +migrate up"
	downMarker = "-- +migrate down"
)

type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
}

type Migrator struct {
	pool *pgxpool.Pool
}

func NewMigrator(pool *pgxpool.Pool) *Migrator { return &Migrator{pool: pool} }

// Up applies all unapplies migrations in order, each in its own tx
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return err
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, mig := range all {
		if applied[mig.Version] {
			continue
		}
		if err := m.runTx(ctx, mig.Up, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `insert into schema_migrations (version, name) values ($1, $2)`, mig.Version, mig.Name)
			return err
		}); err != nil {
			return fmt.Errorf("apply %04d_%s: %w", mig.Version, mig.Name, err)
		}
	}
	return nil
}

// Down rolls back most recently applies migration
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return err
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}

	for i := len(all) - 1; i >= 0; i-- {
		mig := all[i]
		if !applied[mig.Version] {
			continue
		}
		if strings.TrimSpace(mig.Down) == "" {
			return fmt.Errorf("migration %04d_%s has no down script", mig.Version, mig.Name)
		}
		return m.runTx(ctx, mig.Down, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `delete from schema_migrations where version = $1`, mig.Version)
			return err
		})
	}
	return nil
}

// Status returns all migrations and set of applied versions
func (m *Migrator) Status(ctx context.Context) ([]Migration, map[int64]bool, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, nil, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return nil, nil, err
	}
	all, err := loadMigrations()
	if err != nil {
		return nil, nil, err
	}
	return all, applied, nil
}

func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		create table if not exists schema_migrations (
			version    bigint primary key,
			name       text not null,
			applied_at timestamptz not null default now()
		)`)
	return err
}

func (m *Migrator) appliedVersions(ctx context.Context) (map[int64]bool, error) {
	rows, err := m.pool.Query(ctx, `select version from schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// runTx runs a script and its bookkeeping step in one transaction
func (m *Migrator) runTx(ctx context.Context, script string, book func(pgx.Tx) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if strings.TrimSpace(script) != "" {
		if _, err := tx.Exec(ctx, script); err != nil {
			return err
		}
	}
	if err := book(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func loadMigrations() ([]Migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		mig, err := parseMigration(e.Name(), string(raw))
		if err != nil {
			return nil, err
		}
		out = append(out, mig)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("duplicate migration version %d", out[i].Version)
		}
	}
	return out, nil
}

func parseMigration(filename, content string) (Migration, error) {
	base := strings.TrimSuffix(filename, ".sql")
	idx := strings.IndexByte(base, '_')
	if idx <= 0 {
		return Migration{}, fmt.Errorf("bad migration filename %q, want NNNN_name.sql", filename)
	}
	version, err := strconv.ParseInt(base[:idx], 10, 64)
	if err != nil {
		return Migration{}, fmt.Errorf("bad version in %q: %w", filename, err)
	}

	up, down := splitSection(content)
	return Migration{Version: version, Name: base[idx+1:], Up: up, Down: down}, nil
}

func splitSection(content string) (up, down string) {
	var upLines, downLines []string
	inDown := false

	for line := range strings.SplitSeq(content, "\n") {
		switch strings.ToLower(strings.TrimSpace(line)) {
		case upMarker:
			continue
		case downMarker:
			inDown = true
			continue
		}
		if inDown {
			downLines = append(downLines, line)
		} else {
			upLines = append(upLines, line)
		}
	}
	return strings.TrimSpace(strings.Join(upLines, "\n")),
		strings.TrimSpace(strings.Join(downLines, "\n"))
}
