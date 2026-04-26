package typeless

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type AppContext struct {
	AppName  string
	BundleID string
	Title    string
}

type TranscriptRecord struct {
	ID        string
	Text      string
	CreatedAt string
	AppName   string
	BundleID  string
	Title     string
	WebDomain string
	WebURL    string
}

type TranscriptQueryOptions struct {
	Limit   int
	Keyword string
	Regex   *regexp.Regexp
}

func CurrentAppContext(ctx context.Context) (AppContext, error) {
	if runtime.GOOS != "darwin" {
		return AppContext{}, fmt.Errorf("当前应用上下文检测只支持 macOS")
	}

	script := `
tell application "System Events"
	set frontApp to first application process whose frontmost is true
	set appName to name of frontApp
	set bundleID to bundle identifier of frontApp
	set winTitle to ""
	try
		set winTitle to name of front window of frontApp
	end try
	return appName & "\t" & bundleID & "\t" & winTitle
end tell`
	output, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return AppContext{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "\t", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return AppContext{
		AppName:  parts[0],
		BundleID: parts[1],
		Title:    parts[2],
	}, nil
}

func LatestTranscriptContext(ctx context.Context, dbPath, userID string) (AppContext, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return AppContext{}, err
	}
	defer db.Close()

	query := `
SELECT focused_app_name, focused_app_bundle_id, focused_app_window_title
FROM history
WHERE COALESCE(NULLIF(edited_text, ''), NULLIF(refined_text, '')) IS NOT NULL`
	args := []any{}
	if userID != "" {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	query += " ORDER BY created_at DESC LIMIT 1"

	var appName, bundleID, title sql.NullString
	if err := db.QueryRowContext(ctx, query, args...).Scan(&appName, &bundleID, &title); err != nil {
		return AppContext{}, err
	}
	return AppContext{
		AppName:  appName.String,
		BundleID: bundleID.String,
		Title:    title.String,
	}, nil
}

func QueryRecentTranscripts(ctx context.Context, dbPath, userID string, appCtx AppContext, options TranscriptQueryOptions) ([]TranscriptRecord, error) {
	limit, fetchLimit := normalizeTranscriptQueryOptions(options)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
SELECT
	id,
	COALESCE(NULLIF(edited_text, ''), NULLIF(refined_text, '')) AS transcript_text,
	created_at,
	focused_app_name,
	focused_app_bundle_id,
	focused_app_window_title,
	focused_app_window_web_domain,
	focused_app_window_web_url
FROM history
WHERE COALESCE(NULLIF(edited_text, ''), NULLIF(refined_text, '')) IS NOT NULL`
	args := []any{}
	if userID != "" {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	if appCtx.BundleID != "" {
		query += " AND focused_app_bundle_id = ?"
		args = append(args, appCtx.BundleID)
	} else if appCtx.AppName != "" {
		query += " AND focused_app_name = ?"
		args = append(args, appCtx.AppName)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, fetchLimit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]TranscriptRecord, 0, limit)
	for rows.Next() {
		var record TranscriptRecord
		var text, createdAt, appName, bundleID, title, webDomain, webURL sql.NullString
		if err := rows.Scan(
			&record.ID,
			&text,
			&createdAt,
			&appName,
			&bundleID,
			&title,
			&webDomain,
			&webURL,
		); err != nil {
			return nil, err
		}
		record.Text = text.String
		record.CreatedAt = createdAt.String
		record.AppName = appName.String
		record.BundleID = bundleID.String
		record.Title = title.String
		record.WebDomain = webDomain.String
		record.WebURL = webURL.String
		if !matchTranscriptRecord(record, options) {
			continue
		}
		records = append(records, record)
		if len(records) >= limit {
			break
		}
	}
	return records, rows.Err()
}

func normalizeTranscriptQueryOptions(options TranscriptQueryOptions) (limit int, fetchLimit int) {
	limit = options.Limit
	if limit <= 0 {
		limit = 10
	}
	fetchLimit = limit
	if options.Keyword != "" || options.Regex != nil {
		fetchLimit = max(limit*50, 500)
	}
	return limit, fetchLimit
}

func matchTranscriptRecord(record TranscriptRecord, options TranscriptQueryOptions) bool {
	if options.Keyword != "" {
		if !strings.Contains(strings.ToLower(record.Text), strings.ToLower(options.Keyword)) {
			return false
		}
	}
	if options.Regex != nil && !options.Regex.MatchString(record.Text) {
		return false
	}
	return true
}

func CopyToClipboard(ctx context.Context, text string) error {
	if runtime.GOOS == "darwin" {
		cmd := exec.CommandContext(ctx, "pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return fmt.Errorf("当前系统未实现剪贴板写入")
}

func OneLine(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if maxRunes > 0 && len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return text
}
