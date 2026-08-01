package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	dbMu sync.Mutex
)

func initDB() error {
	dbDir := filepath.Join("data")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "usage.db")
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("setting WAL mode: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS api_calls (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now')),
			endpoint    TEXT    NOT NULL,
			mode        TEXT    NOT NULL,
			file_count  INTEGER NOT NULL DEFAULT 0,
			title       TEXT,
			font        TEXT,
			dot_leaders INTEGER NOT NULL DEFAULT 1,
			show_lines  INTEGER NOT NULL DEFAULT 1,
			line_color  TEXT,
			merge_only  INTEGER NOT NULL DEFAULT 0,
			lang        TEXT,
			status      TEXT    NOT NULL DEFAULT 'success',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			error       TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("creating table: %w", err)
	}

	// Migration: add bg_color column if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE api_calls ADD COLUMN bg_color TEXT`)
	// Migration: add font_color column if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE api_calls ADD COLUMN font_color TEXT`)

	return nil
}

type LogCallParams struct {
	Endpoint   string
	Mode       string
	FileCount  int
	Title      *string
	Font       *string
	DotLeaders bool
	ShowLines  bool
	LineColor  *string
	BgColor    *string
	FontColor  *string
	MergeOnly  bool
	Lang       *string
	Status     string
	DurationMs int
	Error      *string
}

func logCall(p LogCallParams) {
	dbMu.Lock()
	defer dbMu.Unlock()

	if db == nil {
		return
	}

	_, _ = db.Exec(`
		INSERT INTO api_calls
			(endpoint, mode, file_count, title, font, dot_leaders, show_lines,
			 line_color, bg_color, font_color, merge_only, lang, status, duration_ms, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		p.Endpoint,
		p.Mode,
		p.FileCount,
		p.Title,
		p.Font,
		boolToInt(p.DotLeaders),
		boolToInt(p.ShowLines),
		p.LineColor,
		p.BgColor,
		p.FontColor,
		boolToInt(p.MergeOnly),
		p.Lang,
		p.Status,
		p.DurationMs,
		p.Error,
	)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func getStats() (map[string]interface{}, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	result := make(map[string]interface{})

	var total, success, errors int
	db.QueryRow("SELECT COUNT(*) FROM api_calls").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM api_calls WHERE status = 'success'").Scan(&success)
	db.QueryRow("SELECT COUNT(*) FROM api_calls WHERE status = 'error'").Scan(&errors)
	result["total_calls"] = total
	result["successful"] = success
	result["errors"] = errors

	// By mode
	byMode := make(map[string]int)
	rows, err := db.Query("SELECT mode, COUNT(*) AS cnt FROM api_calls GROUP BY mode")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mode string
			var cnt int
			rows.Scan(&mode, &cnt)
			byMode[mode] = cnt
		}
	}
	result["by_mode"] = byMode

	// By lang
	byLang := make(map[string]int)
	rows, err = db.Query("SELECT COALESCE(lang, '') AS lang, COUNT(*) AS cnt FROM api_calls GROUP BY lang")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var lang string
			var cnt int
			rows.Scan(&lang, &cnt)
			byLang[lang] = cnt
		}
	}
	result["by_lang"] = byLang

	// By font
	byFont := make(map[string]int)
	rows, err = db.Query("SELECT font, COUNT(*) AS cnt FROM api_calls WHERE font IS NOT NULL GROUP BY font")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var font string
			var cnt int
			rows.Scan(&font, &cnt)
			byFont[font] = cnt
		}
	}
	result["by_font"] = byFont

	var avgDuration float64
	db.QueryRow("SELECT COALESCE(AVG(duration_ms), 0) FROM api_calls WHERE status = 'success'").Scan(&avgDuration)
	result["avg_duration_ms"] = int(avgDuration + 0.5)

	var totalFiles int
	db.QueryRow("SELECT COALESCE(SUM(file_count), 0) FROM api_calls").Scan(&totalFiles)
	result["total_files_processed"] = totalFiles

	// Daily calls (last 30 days)
	type dailyEntry struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	var daily []dailyEntry
	rows, err = db.Query(`
		SELECT date(timestamp) AS d, COUNT(*) AS cnt
		FROM api_calls
		WHERE timestamp >= date('now', '-30 days')
		GROUP BY d
		ORDER BY d
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d dailyEntry
			rows.Scan(&d.Date, &d.Count)
			daily = append(daily, d)
		}
	}
	if daily == nil {
		daily = []dailyEntry{}
	}
	result["daily_calls"] = daily

	return result, nil
}

func getRecentCalls(limit int) ([]map[string]interface{}, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	rows, err := db.Query(`
		SELECT id, timestamp, endpoint, mode, file_count, title, font,
		       dot_leaders, show_lines, line_color, bg_color, font_color,
		       merge_only, lang, status, duration_ms, error
		FROM api_calls
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var (
			id, fileCount, dotLeaders, showLines, mergeOnly, durationMs int
			timestamp, endpoint, mode, status                           string
			title, font, lineColor, bgColor, fontColor, lang, errMsg    sql.NullString
		)
		err := rows.Scan(&id, &timestamp, &endpoint, &mode, &fileCount,
			&title, &font, &dotLeaders, &showLines, &lineColor, &bgColor,
			&fontColor, &mergeOnly, &lang, &status, &durationMs, &errMsg)
		if err != nil {
			continue
		}
		row := map[string]interface{}{
			"id":          id,
			"timestamp":   timestamp,
			"endpoint":    endpoint,
			"mode":        mode,
			"file_count":  fileCount,
			"title":       nullStr(title),
			"font":        nullStr(font),
			"dot_leaders": dotLeaders,
			"show_lines":  showLines,
			"line_color":  nullStr(lineColor),
			"bg_color":    nullStr(bgColor),
			"font_color":  nullStr(fontColor),
			"merge_only":  mergeOnly,
			"lang":        nullStr(lang),
			"status":      status,
			"duration_ms": durationMs,
			"error":       nullStr(errMsg),
		}
		result = append(result, row)
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return result, nil
}

func nullStr(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}
