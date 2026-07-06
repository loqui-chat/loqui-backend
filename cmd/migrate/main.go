// Command migrate applies schema migration: up | down | status
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/loqui-chat/loqui-backend/internal/config"
	"github.com/loqui-chat/loqui-backend/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status>")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer pool.Close()

	m := db.NewMigrator(pool)

	switch os.Args[1] {
	case "up":
		if err := m.Up(ctx); err != nil {
			fail(err)
		}
		fmt.Println("migrations up to date")
	case "down":
		if err := m.Down(ctx); err != nil {
			fail(err)
		}
		fmt.Println("rolled back on migration")
	case "status":
		all, applied, err := m.Status(ctx)
		if err != nil {
			fail(err)
		}
		for _, mig := range all {
			mark := "pending"
			if applied[mig.Version] {
				mark = "applied"
			}
			fmt.Printf("%-8s %04d %s\n", mark, mig.Version, mig.Name)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migrate:", err)
	os.Exit(1)
}
