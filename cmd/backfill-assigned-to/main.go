// Command backfill-assigned-to is a one-off migration for the switch from the
// old client-side workaround — the customer create/edit form stuffed the
// picked team member's UUID into customers.notes as JSON ({"assigned_to":
// "<uuid>"}) — to the real customers.assigned_to column now that the API
// resolves and returns it directly.
//
// It only ever touches rows where assigned_to IS NULL, so it's safe to
// re-run. Defaults to a dry run; pass -apply to write changes.
package main

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/config"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	apply := flag.Bool("apply", false, "write changes (default: dry run, prints what would change)")
	flag.Parse()

	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := db.NewStore(pool)

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, notes
		FROM customers
		WHERE assigned_to IS NULL
		  AND notes IS NOT NULL
		  AND notes <> ''
	`)
	if err != nil {
		log.Fatalf("query candidates: %v", err)
	}

	type row struct {
		id, userID uuid.UUID
		notes      string
	}
	var candidates []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.userID, &r.notes); err != nil {
			rows.Close()
			log.Fatalf("scan: %v", err)
		}
		candidates = append(candidates, r)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		log.Fatalf("rows: %v", rowsErr)
	}

	var found, updated, skipped int
	for _, c := range candidates {
		var parsed struct {
			AssignedTo string `json:"assigned_to"`
		}
		if err := json.Unmarshal([]byte(c.notes), &parsed); err != nil || parsed.AssignedTo == "" {
			continue // notes isn't our JSON shape, or has no assigned_to key — not a candidate
		}
		found++

		targetID, err := uuid.Parse(parsed.AssignedTo)
		if err != nil {
			fmt.Printf("skip customer %s: notes.assigned_to %q is not a UUID\n", c.id, parsed.AssignedTo)
			skipped++
			continue
		}

		target, err := store.GetUserByID(ctx, targetID)
		if err != nil {
			fmt.Printf("skip customer %s: assigned_to user %s not found or inactive\n", c.id, targetID)
			skipped++
			continue
		}
		belongsToOrg := target.ID == c.userID || (target.OrgOwnerID != nil && *target.OrgOwnerID == c.userID)
		if !belongsToOrg {
			fmt.Printf("skip customer %s: user %s (%s) is not a member of this org\n", c.id, targetID, target.Name)
			skipped++
			continue
		}

		fmt.Printf("assign customer %s -> %s (%s)\n", c.id, targetID, target.Name)
		if *apply {
			if _, err := store.AssignCustomer(ctx, db.AssignCustomerParams{
				ID: c.id, UserID: c.userID, AssignedTo: &targetID,
			}); err != nil {
				fmt.Printf("  FAILED: %v\n", err)
				continue
			}
		}
		updated++
	}

	mode := "dry run — pass -apply to write"
	if *apply {
		mode = "applied"
	}
	fmt.Printf("\n%d candidates found, %d updated, %d skipped (%s)\n", found, updated, skipped, mode)
}
