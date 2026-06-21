package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type RebuildRecord struct {
	ID               int
	StartedAt        time.Time
	FinishedAt       time.Time
	DurationMs       int64
	Success          bool
	ExitCode         sql.NullInt64
	Command          string
	ErrorMessage     sql.NullString
	DiffFilesChanged int
	DiffAddedLines   int
	DiffDeletedLines int
	DiffTotalLines   int
}

func StatsCommand() error {
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		return err
	}
	defer db.Close()

	records, err := loadRebuildRecords(db)
	if err != nil {
		return err
	}

	printStatistics(records)
	return nil
}

func loadRebuildRecords(db *sql.DB) ([]RebuildRecord, error) {
	rows, err := db.Query(`
		SELECT
			id,
			started_at,
			finished_at,
			duration_ms,
			success,
			exit_code,
			command,
			error_message,
			diff_files_changed,
			diff_added_lines,
			diff_deleted_lines,
			diff_total_lines
		FROM rebuilds
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RebuildRecord
	for rows.Next() {
		var record RebuildRecord
		var startedAt string
		var finishedAt string
		var successInt int

		err := rows.Scan(
			&record.ID,
			&startedAt,
			&finishedAt,
			&record.DurationMs,
			&successInt,
			&record.ExitCode,
			&record.Command,
			&record.ErrorMessage,
			&record.DiffFilesChanged,
			&record.DiffAddedLines,
			&record.DiffDeletedLines,
			&record.DiffTotalLines,
		)
		if err != nil {
			return nil, err
		}

		record.StartedAt, err = time.Parse(time.RFC3339, startedAt)
		if err != nil {
			return nil, err
		}
		record.FinishedAt, err = time.Parse(time.RFC3339, finishedAt)
		if err != nil {
			return nil, err
		}
		record.Success = successInt != 0

		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func printStatistics(records []RebuildRecord) {
	total := len(records)
	successful := 0
	var totalDuration time.Duration
	var lastRebuild time.Time
	var totalChangedFiles int
	var totalAddedLines int
	var totalDeletedLines int

	for i := range records {
		record := records[i]
		if record.Success {
			successful++
		}
		totalDuration += time.Duration(record.DurationMs) * time.Millisecond
		totalChangedFiles += record.DiffFilesChanged
		totalAddedLines += record.DiffAddedLines
		totalDeletedLines += record.DiffDeletedLines
		if record.FinishedAt.After(lastRebuild) {
			lastRebuild = record.FinishedAt
		}
	}

	failed := total - successful
	successRate := 0.0
	if total > 0 {
		successRate = float64(successful) * 100 / float64(total)
	}

	averageDuration := 0 * time.Second
	if total > 0 {
		averageDuration = totalDuration / time.Duration(total)
	}

	fmt.Println("VNix rebuild statistics")
	fmt.Println()
	fmt.Printf("Total rebuilds:      %d\n", total)
	fmt.Printf("Successful:          %d\n", successful)
	fmt.Printf("Failed:              %d\n", failed)
	fmt.Printf("Success rate:        %.1f%%\n", successRate)
	fmt.Printf("Average duration:    %.1fs\n", averageDuration.Seconds())
	if !lastRebuild.IsZero() {
		fmt.Printf("Last rebuild:        %s\n", lastRebuild.Format("2006-01-02 15:04"))
	} else {
		fmt.Printf("Last rebuild:        -\n")
	}
	fmt.Printf("Total changed files: %d\n", totalChangedFiles)
	fmt.Printf("Total added lines:   %d\n", totalAddedLines)
	fmt.Printf("Total deleted lines: %d\n", totalDeletedLines)

	fmt.Println()
	fmt.Println("Recent rebuilds:")
	fmt.Println()
	printRecentRebuilds(records, 5)
}

func printRecentRebuilds(records []RebuildRecord, limit int) {
	if len(records) == 0 {
		return
	}

	start := len(records) - limit
	if start < 0 {
		start = 0
	}

	for i := len(records) - 1; i >= start; i-- {
		record := records[i]
		marker := "✗"
		if record.Success {
			marker = "✓"
		}

		duration := fmt.Sprintf("%.1fs", float64(record.DurationMs)/1000)
		timestamp := record.StartedAt.Format("2006-01-02 15:04")

		if record.Success {
			fmt.Printf("%s %s | %s | +%d -%d | files: %d\n",
				marker,
				timestamp,
				duration,
				record.DiffAddedLines,
				record.DiffDeletedLines,
				record.DiffFilesChanged,
			)
			continue
		}

		exitCode := "unknown"
		if record.ExitCode.Valid {
			exitCode = fmt.Sprintf("%d", record.ExitCode.Int64)
		}
		fmt.Printf("%s %s | %s | exit code: %s\n", marker, timestamp, duration, exitCode)
	}
}
