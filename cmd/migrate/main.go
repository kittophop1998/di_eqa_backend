// Command migrate applies pending database migrations. It is idempotent and
// non-destructive; on a duplicate-username conflict it stops and reports.
//
//	MONGO_URI=... MONGO_DB=... go run ./cmd/migrate
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/di-eqa/backend/internal/infrastructure/db"
	"github.com/di-eqa/backend/internal/infrastructure/migrate"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "list pending migrations without applying them")
	flag.Parse()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI must be set explicitly (there is no default, to avoid migrating the wrong database)")
	}
	name := os.Getenv("MONGO_DB")
	if name == "" {
		name = "di_eqa"
	}
	conn, err := db.Connect(uri, name)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	store := migrate.NewMongoStore(conn.DB)
	steps := migrate.Steps(conn.DB)

	if *dryRun {
		pending, err := migrate.Pending(ctx, store, steps)
		if err != nil {
			log.Fatal(err)
		}
		for _, s := range pending {
			log.Printf("pending: %d %s", s.Version, s.Name)
		}
		log.Printf("%d pending", len(pending))
		return
	}
	done, err := migrate.Run(ctx, store, steps, log.Printf)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	log.Printf("migrations applied: %d (database %q)", len(done), name)
}
