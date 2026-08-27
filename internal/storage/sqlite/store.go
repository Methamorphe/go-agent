package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/storage"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Config struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        clock.Clock
}

type Store struct {
	db *sql.DB
}

func Open(
	ctx context.Context,
	cfg Config,
) (*Store, error) {
	if cfg.Path == "" {
		return nil, errs.New(
			errs.CodeInvalidArgument,
			"sqlite.open",
			"database path is empty",
		)
	}

	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = 5 * time.Second
	}

	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 8
	}

	if cfg.Clock == nil {
		cfg.Clock = clock.NewSystemClock()
	}

	path, err := filepath.Abs(
		filepath.Clean(cfg.Path),
	)
	if err != nil {
		return nil, errs.Wrap(
			errs.CodeInvalidArgument,
			"sqlite.open",
			"normalize database path",
			err,
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(path),
		0o700,
	); err != nil {
		return nil, errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.open",
			"create database directory",
			err,
		)
	}

	dsn, err := buildDSN(
		path,
		cfg.BusyTimeout,
	)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(
		"sqlite",
		dsn,
	)
	if err != nil {
		return nil, errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.open",
			"open database",
			err,
		)
	}

	db.SetMaxOpenConns(
		cfg.MaxOpenConns,
	)

	db.SetMaxIdleConns(
		min(cfg.MaxOpenConns, 4),
	)

	store := &Store{
		db: db,
	}

	cleanup := true

	defer func() {
		if cleanup {
			_ = db.Close()
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return nil, errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.open",
			"ping database",
			err,
		)
	}

	if err := applyMigrations(
		ctx,
		db,
		cfg.Clock,
	); err != nil {
		return nil, err
	}

	if err := verifyPragmas(
		ctx,
		db,
		cfg.BusyTimeout,
	); err != nil {
		return nil, err
	}

	cleanup = false

	return store, nil
}

func buildDSN(
	path string,
	busyTimeout time.Duration,
) (string, error) {
	slash := filepath.ToSlash(path)

	if runtime.GOOS == "windows" &&
		len(slash) >= 2 &&
		slash[1] == ':' {
		slash = "/" + slash
	}

	uri := url.URL{
		Scheme: "file",
		Path:   slash,
	}

	query := uri.Query()

	query.Add(
		"_pragma",
		"foreign_keys(1)",
	)

	query.Add(
		"_pragma",
		"journal_mode(WAL)",
	)

	query.Add(
		"_pragma",
		"synchronous(FULL)",
	)

	query.Add(
		"_pragma",
		fmt.Sprintf(
			"busy_timeout(%d)",
			busyTimeout.Milliseconds(),
		),
	)

	query.Set(
		"_defensive",
		"1",
	)

	uri.RawQuery = query.Encode()

	return uri.String(), nil
}

func (s *Store) Ping(
	ctx context.Context,
) error {
	if err := s.db.PingContext(ctx); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.ping",
			"database ping failed",
			err,
		)
	}

	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Stats() storage.Stats {
	stats := s.db.Stats()

	return storage.Stats{
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
		WaitCount:       stats.WaitCount,
	}
}

func (s *Store) Checkpoint(
	ctx context.Context,
) error {
	var busy int
	var logFrames int
	var checkpointed int

	err := s.db.QueryRowContext(
		ctx,
		"PRAGMA wal_checkpoint(TRUNCATE)",
	).Scan(
		&busy,
		&logFrames,
		&checkpointed,
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.checkpoint",
			"checkpoint WAL",
			err,
		)
	}

	if busy != 0 {
		return errs.New(
			errs.CodeUnavailable,
			"sqlite.checkpoint",
			"checkpoint could not acquire all required locks",
		)
	}

	return nil
}

func (s *Store) RegisterObject(
	ctx context.Context,
	object storage.ObjectRecord,
) error {
	const query = `
INSERT OR IGNORE INTO objects
(
    ref,
    algorithm,
    digest,
    size_bytes,
    media_type,
    compression,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := s.db.ExecContext(
		ctx,
		query,
		object.Ref,
		object.Algorithm,
		object.Digest,
		object.SizeBytes,
		object.MediaType,
		object.Compression,
		object.CreatedAt.UTC().
			Format(time.RFC3339Nano),
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.register_object",
			"insert object metadata",
			err,
		)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(
			errs.CodeInternal,
			"sqlite.register_object",
			"read affected rows",
			err,
		)
	}

	if rows == 1 {
		return nil
	}

	existing, err := s.Object(
		ctx,
		object.Ref,
	)
	if err != nil {
		return err
	}

	if existing.Algorithm != object.Algorithm ||
		existing.Digest != object.Digest ||
		existing.SizeBytes != object.SizeBytes {
		return errs.New(
			errs.CodeCorruption,
			"sqlite.register_object",
			"existing object metadata conflicts with immutable object identity",
		)
	}

	return nil
}

func (s *Store) Object(
	ctx context.Context,
	ref string,
) (storage.ObjectRecord, error) {
	const query = `
SELECT
    ref,
    algorithm,
    digest,
    size_bytes,
    media_type,
    compression,
    created_at
FROM objects
WHERE ref = ?`

	var object storage.ObjectRecord
	var createdAt string

	err := s.db.QueryRowContext(
		ctx,
		query,
		ref,
	).Scan(
		&object.Ref,
		&object.Algorithm,
		&object.Digest,
		&object.SizeBytes,
		&object.MediaType,
		&object.Compression,
		&createdAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return storage.ObjectRecord{},
			errs.Wrap(
				errs.CodeNotFound,
				"sqlite.object",
				"object metadata not found",
				err,
			)
	}

	if err != nil {
		return storage.ObjectRecord{},
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.object",
				"query object metadata",
				err,
			)
	}

	object.CreatedAt, err =
		time.Parse(
			time.RFC3339Nano,
			createdAt,
		)

	if err != nil {
		return storage.ObjectRecord{},
			errs.Wrap(
				errs.CodeCorruption,
				"sqlite.object",
				"invalid object timestamp",
				err,
			)
	}

	return object, nil
}

func applyMigrations(
	ctx context.Context,
	db *sql.DB,
	clock clock.Clock,
) error {
	_, err := db.ExecContext(
		ctx,
		`
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    checksum_sha256 TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`,
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.migrate",
			"create migration table",
			err,
		)
	}

	type appliedMigration struct {
		name     string
		checksum string
	}

	applied := map[int]appliedMigration{}

	rows, err := db.QueryContext(
		ctx,
		`
SELECT
    version,
    name,
    checksum_sha256
FROM schema_migrations`,
	)
	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.migrate",
			"read applied migrations",
			err,
		)
	}

	for rows.Next() {
		var version int
		var migration appliedMigration

		if err := rows.Scan(
			&version,
			&migration.name,
			&migration.checksum,
		); err != nil {
			rows.Close()

			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.migrate",
				"scan applied migration",
				err,
			)
		}

		applied[version] = migration
	}

	if err := rows.Close(); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.migrate",
			"close migration rows",
			err,
		)
	}

	if err := rows.Err(); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.migrate",
			"iterate applied migrations",
			err,
		)
	}

	entries, err := migrationFS.ReadDir(
		"migrations",
	)
	if err != nil {
		return errs.Wrap(
			errs.CodeInternal,
			"sqlite.migrate",
			"read embedded migrations",
			err,
		)
	}

	sort.Slice(
		entries,
		func(i, j int) bool {
			return entries[i].Name() <
				entries[j].Name()
		},
	)

	seen := map[int]struct{}{}

	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(
				entry.Name(),
				".sql",
			) {
			continue
		}

		version, err :=
			migrationVersion(entry.Name())

		if err != nil {
			return err
		}

		if _, exists := seen[version]; exists {
			return errs.New(
				errs.CodeConflict,
				"sqlite.migrate",
				fmt.Sprintf(
					"duplicate migration version %d",
					version,
				),
			)
		}

		seen[version] = struct{}{}

		body, err := migrationFS.ReadFile(
			"migrations/" + entry.Name(),
		)
		if err != nil {
			return errs.Wrap(
				errs.CodeInternal,
				"sqlite.migrate",
				"read migration",
				err,
			)
		}

		sum := sha256.Sum256(body)

		checksum :=
			hex.EncodeToString(sum[:])

		if previous, exists :=
			applied[version]; exists {
			if previous.name != entry.Name() ||
				previous.checksum != checksum {
				return errs.New(
					errs.CodeCorruption,
					"sqlite.migrate",
					fmt.Sprintf(
						"applied migration %d no longer matches embedded migration",
						version,
					),
				)
			}

			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.migrate",
				"begin migration transaction",
				err,
			)
		}

		if _, err := tx.ExecContext(
			ctx,
			string(body),
		); err != nil {
			_ = tx.Rollback()

			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.migrate",
				"execute migration "+entry.Name(),
				err,
			)
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO schema_migrations
(
    version,
    name,
    checksum_sha256,
    applied_at
)
VALUES (?, ?, ?, ?)`,
			version,
			entry.Name(),
			checksum,
			clock.Now().UTC().
				Format(time.RFC3339Nano),
		)

		if err != nil {
			_ = tx.Rollback()

			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.migrate",
				"record migration "+entry.Name(),
				err,
			)
		}

		if err := tx.Commit(); err != nil {
			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.migrate",
				"commit migration "+entry.Name(),
				err,
			)
		}
	}

	return nil
}

func migrationVersion(
	name string,
) (int, error) {
	prefix, _, ok := strings.Cut(
		name,
		"_",
	)

	if !ok {
		return 0, errs.New(
			errs.CodeInvalidArgument,
			"sqlite.migrate",
			"migration name must start with numeric version",
		)
	}

	version, err := strconv.Atoi(prefix)

	if err != nil ||
		version <= 0 {
		return 0, errs.Wrap(
			errs.CodeInvalidArgument,
			"sqlite.migrate",
			"invalid migration version in "+name,
			err,
		)
	}

	return version, nil
}

func verifyPragmas(
	ctx context.Context,
	db *sql.DB,
	busyTimeout time.Duration,
) error {
	var foreignKeys int

	err := db.QueryRowContext(
		ctx,
		"PRAGMA foreign_keys",
	).Scan(&foreignKeys)

	if err != nil ||
		foreignKeys != 1 {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.verify",
			"foreign_keys is not enabled",
			err,
		)
	}

	var journalMode string

	err = db.QueryRowContext(
		ctx,
		"PRAGMA journal_mode",
	).Scan(&journalMode)

	if err != nil ||
		!strings.EqualFold(
			journalMode,
			"wal",
		) {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.verify",
			"journal_mode is not WAL",
			err,
		)
	}

	var synchronous int

	err = db.QueryRowContext(
		ctx,
		"PRAGMA synchronous",
	).Scan(&synchronous)

	if err != nil ||
		synchronous != 2 {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.verify",
			"synchronous is not FULL",
			err,
		)
	}

	var timeout int

	err = db.QueryRowContext(
		ctx,
		"PRAGMA busy_timeout",
	).Scan(&timeout)

	if err != nil ||
		timeout !=
			int(busyTimeout.Milliseconds()) {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.verify",
			"busy_timeout is not configured",
			err,
		)
	}

	return nil
}
