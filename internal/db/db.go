package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/app"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{DB: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS admins (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT NOT NULL UNIQUE,
            password_hash TEXT NOT NULL,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        );`,
		`CREATE TABLE IF NOT EXISTS sessions (
            id TEXT PRIMARY KEY,
            admin_username TEXT NOT NULL,
            csrf_token TEXT NOT NULL,
            theme TEXT NOT NULL,
            expires_at DATETIME NOT NULL,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        );`,
		`CREATE TABLE IF NOT EXISTS metric_samples (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            ts DATETIME NOT NULL,
            cpu_percent REAL NOT NULL,
            load1 REAL NOT NULL,
            memory_used INTEGER NOT NULL,
            memory_total INTEGER NOT NULL,
            memory_used_pct REAL NOT NULL,
            network_rx REAL NOT NULL,
            network_tx REAL NOT NULL,
            payload_json TEXT NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            kind TEXT NOT NULL,
            source TEXT NOT NULL,
            status TEXT NOT NULL,
            title TEXT NOT NULL,
            details TEXT NOT NULL,
            created_at DATETIME NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            action TEXT NOT NULL,
            subject TEXT NOT NULL,
            details TEXT NOT NULL,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        );`,
		`CREATE INDEX IF NOT EXISTS idx_metric_samples_ts ON metric_samples(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnsureAdmin(ctx context.Context, username, passwordHash string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO admins (username, password_hash) VALUES (?, ?) ON CONFLICT(username) DO UPDATE SET password_hash=excluded.password_hash`, username, passwordHash)
	return err
}

func (s *Store) GetAdminHash(ctx context.Context, username string) (string, error) {
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM admins WHERE username = ?`, username).Scan(&hash)
	return hash, err
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&count)
	return count, err
}

func (s *Store) CreateSession(ctx context.Context, id, username, csrfToken, theme string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (id, admin_username, csrf_token, theme, expires_at) VALUES (?, ?, ?, ?, ?)`, id, username, csrfToken, theme, expiresAt.UTC())
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (username, csrfToken, theme string, expiresAt time.Time, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT admin_username, csrf_token, theme, expires_at FROM sessions WHERE id = ?`, id).Scan(&username, &csrfToken, &theme, &expiresAt)
	return
}

func (s *Store) UpdateSessionTheme(ctx context.Context, id, theme string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET theme = ? WHERE id = ?`, theme, id)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.UTC())
	return err
}

func (s *Store) InsertSample(ctx context.Context, snap app.HostSnapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO metric_samples (ts, cpu_percent, load1, memory_used, memory_total, memory_used_pct, network_rx, network_tx, payload_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.CollectedAt.UTC(), snap.CPUPercent, snap.CPULoad1, snap.MemoryUsed, snap.MemoryTotal, snap.MemoryUsedPct, snap.Network.RXBytesPerS, snap.Network.TXBytesPerS, string(payload))
	return err
}

func (s *Store) LatestSample(ctx context.Context) (app.HostSnapshot, error) {
	var payload string
	err := s.DB.QueryRowContext(ctx, `SELECT payload_json FROM metric_samples ORDER BY ts DESC LIMIT 1`).Scan(&payload)
	if err != nil {
		return app.HostSnapshot{}, err
	}
	var snap app.HostSnapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		return app.HostSnapshot{}, err
	}
	return snap, nil
}

func (s *Store) History(ctx context.Context, since time.Time) ([]app.HostSnapshot, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT payload_json FROM metric_samples WHERE ts >= ? ORDER BY ts ASC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.HostSnapshot
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var snap app.HostSnapshot
		if err := json.Unmarshal([]byte(payload), &snap); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) AddEvent(ctx context.Context, event app.Event) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO events (kind, source, status, title, details, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		event.Kind, event.Source, string(event.Status), event.Title, event.Details, event.CreatedAt.UTC())
	return err
}

func (s *Store) RecentEvents(ctx context.Context, limit int) ([]app.Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, kind, source, status, title, details, created_at FROM events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.Event
	for rows.Next() {
		var ev app.Event
		var status string
		if err := rows.Scan(&ev.ID, &ev.Kind, &ev.Source, &status, &ev.Title, &ev.Details, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Status = app.Status(status)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) AddAudit(ctx context.Context, action, subject, details string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO audit_logs (action, subject, details) VALUES (?, ?, ?)`, action, subject, details)
	return err
}

func (s *Store) Prune(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM metric_samples WHERE ts < ?`, cutoff.UTC()); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM events WHERE created_at < ?`, cutoff.UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Store) ExportBackup(ctx context.Context, configPath, dbPath, destPath string) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	return CreateTarGz(destPath, map[string]string{
		filepath.Base(configPath): configPath,
		filepath.Base(dbPath):     dbPath,
	})
}

func (s *Store) Health(ctx context.Context) error {
	var one int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		return fmt.Errorf("unexpected health response %d", one)
	}
	return nil
}
