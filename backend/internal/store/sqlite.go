package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/palpal/gphoto/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type SQLite struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (store *SQLite) Close() error { return store.db.Close() }

func (store *SQLite) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS google_accounts (
  id TEXT PRIMARY KEY,
  google_subject TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL,
  name TEXT NOT NULL,
  media_type TEXT NOT NULL DEFAULT 'all',
  token_ciphertext BLOB NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_states (
  state TEXT PRIMARY KEY,
  account_name TEXT NOT NULL,
	expected_email TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS picker_sessions (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES google_accounts(id),
  picker_uri TEXT NOT NULL,
  status TEXT NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS picker_sessions_status_idx ON picker_sessions(status, updated_at);
CREATE TABLE IF NOT EXISTS downloaded_media (
  account_id TEXT NOT NULL REFERENCES google_accounts(id),
  provider_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  source_created_at INTEGER NOT NULL,
  downloaded_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, provider_id)
);`
	if _, err := store.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return store.ensureColumn(ctx, "oauth_states", "expected_email", "TEXT NOT NULL DEFAULT ''")
}

func (store *SQLite) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&id, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		found = found || name == column
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = store.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
}

func (store *SQLite) PutOAuthState(ctx context.Context, state, accountName, expectedEmail string, expiresAt time.Time) error {
	_, err := store.db.ExecContext(ctx, `INSERT INTO oauth_states(state, account_name, expected_email, expires_at) VALUES (?, ?, ?, ?)`, state, accountName, expectedEmail, expiresAt.Unix())
	return err
}

func (store *SQLite) ConsumeOAuthState(ctx context.Context, state string, now time.Time) (string, string, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var name, expectedEmail string
	var expiresAt int64
	if err := tx.QueryRowContext(ctx, `SELECT account_name, expected_email, expires_at FROM oauth_states WHERE state = ?`, state).Scan(&name, &expectedEmail, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	if now.Unix() > expiresAt {
		return "", "", ErrNotFound
	}
	return name, expectedEmail, nil
}

func (store *SQLite) UpsertGoogleAccount(ctx context.Context, account domain.GoogleAccount) error {
	_, err := store.SaveGoogleAccount(ctx, account)
	return err
}

func (store *SQLite) SaveGoogleAccount(ctx context.Context, account domain.GoogleAccount) (domain.GoogleAccount, error) {
	var createdAt int64
	err := store.db.QueryRowContext(ctx, `
INSERT INTO google_accounts(id, google_subject, email, name, media_type, token_ciphertext, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email, name=excluded.name, token_ciphertext=excluded.token_ciphertext
RETURNING id, google_subject, email, name, media_type, token_ciphertext, created_at`,
		account.ID, account.GoogleSubject, account.Email, account.Name, account.MediaType, account.TokenCiphertext, account.CreatedAt.Unix()).
		Scan(&account.ID, &account.GoogleSubject, &account.Email, &account.Name, &account.MediaType, &account.TokenCiphertext, &createdAt)
	account.CreatedAt = time.Unix(createdAt, 0).UTC()
	return account, err
}

func (store *SQLite) UpdateGoogleAccountMediaType(ctx context.Context, id string, mediaType domain.MediaType) error {
	result, err := store.db.ExecContext(ctx, `UPDATE google_accounts SET media_type = ? WHERE id = ?`, mediaType, id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *SQLite) GetGoogleAccount(ctx context.Context, id string) (domain.GoogleAccount, error) {
	var account domain.GoogleAccount
	var createdAt int64
	err := store.db.QueryRowContext(ctx, `SELECT id, google_subject, email, name, media_type, token_ciphertext, created_at FROM google_accounts WHERE id = ?`, id).
		Scan(&account.ID, &account.GoogleSubject, &account.Email, &account.Name, &account.MediaType, &account.TokenCiphertext, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GoogleAccount{}, ErrNotFound
	}
	account.CreatedAt = time.Unix(createdAt, 0).UTC()
	return account, err
}

func (store *SQLite) ListGoogleAccounts(ctx context.Context) ([]domain.GoogleAccount, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, google_subject, email, name, media_type, created_at FROM google_accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]domain.GoogleAccount, 0)
	for rows.Next() {
		var account domain.GoogleAccount
		var createdAt int64
		if err := rows.Scan(&account.ID, &account.GoogleSubject, &account.Email, &account.Name, &account.MediaType, &createdAt); err != nil {
			return nil, err
		}
		account.CreatedAt = time.Unix(createdAt, 0).UTC()
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (store *SQLite) CreatePickerSession(ctx context.Context, session domain.PickerSession) error {
	_, err := store.db.ExecContext(ctx, `INSERT INTO picker_sessions(id, account_id, picker_uri, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, session.AccountID, session.PickerURI, session.Status, session.CreatedAt.Unix(), session.UpdatedAt.Unix())
	return err
}

func (store *SQLite) ListPickerSessions(ctx context.Context, pendingOnly bool) ([]domain.PickerSession, error) {
	query := `SELECT id, account_id, picker_uri, status, last_error, created_at, updated_at FROM picker_sessions`
	if pendingOnly {
		query += ` WHERE status IN ('waiting', 'importing')`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := store.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]domain.PickerSession, 0)
	for rows.Next() {
		var session domain.PickerSession
		var createdAt, updatedAt int64
		if err := rows.Scan(&session.ID, &session.AccountID, &session.PickerURI, &session.Status, &session.LastError, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		session.CreatedAt = time.Unix(createdAt, 0).UTC()
		session.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (store *SQLite) UpdatePickerSession(ctx context.Context, id, status, lastError string, now time.Time) error {
	result, err := store.db.ExecContext(ctx, `UPDATE picker_sessions SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`, status, lastError, now.Unix(), id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *SQLite) RecordDownloaded(ctx context.Context, accountID string, item domain.GoogleMediaItem, relativePath string, now time.Time) error {
	_, err := store.db.ExecContext(ctx, `INSERT OR IGNORE INTO downloaded_media(account_id, provider_id, filename, mime_type, relative_path, source_created_at, downloaded_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, item.ProviderID, item.Filename, item.MimeType, relativePath, item.CreatedAt.Unix(), now.Unix())
	return err
}

func (store *SQLite) IsDownloaded(ctx context.Context, accountID, providerID string) (bool, error) {
	var exists int
	err := store.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM downloaded_media WHERE account_id = ? AND provider_id = ?)`, accountID, providerID).Scan(&exists)
	return exists == 1, err
}
