package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func StatsCommand() error {
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		return err
	}
	defer db.Close()

	rebuildCount, err := getRebuildCount(db)

	fmt.Printf("Total rebuilds: %d\n", rebuildCount)

	return nil
}

func getRebuildCount(db *sql.DB) (int, error) {
	var rebuildCount int
	err := db.QueryRow("SELECT COUNT(*) FROM rebuilds").Scan(&rebuildCount)
	if err != nil {
		return 0, err
	}
	return rebuildCount, nil
}

