// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package db opens the SQLite database and applies the migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the database and brings it to the latest schema version.
func Open(path string) (*sql.DB, error) {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	d, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(4)
	d.SetConnMaxIdleTime(5 * time.Minute)
	if err := migrate(d); err != nil {
		d.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}
	return d, nil
}

// Each element is a migration; the current version is stored in PRAGMA user_version.
// Dates are TEXT 'YYYY-MM-DD', timestamps TEXT 'YYYY-MM-DD HH:MM:SS' in UTC,
// money INTEGER cents.
var migrations = []string{
	`
CREATE TABLE users (
	id             INTEGER PRIMARY KEY,
	username       TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name   TEXT NOT NULL DEFAULT '',
	password_hash  TEXT NOT NULL,
	is_admin       INTEGER NOT NULL DEFAULT 0,
	locale         TEXT NOT NULL DEFAULT '',
	ical_token     TEXT NOT NULL UNIQUE,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);

-- Only the SHA-256 of the session token is stored.
CREATE TABLE sessions (
	token_hash    TEXT PRIMARY KEY,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at    TEXT NOT NULL,
	expires_at    TEXT NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE vehicles (
	id                       INTEGER PRIMARY KEY,
	name                     TEXT NOT NULL,
	kind                     TEXT NOT NULL CHECK (kind IN ('car','motorcycle','moped','van','truck','other')),
	make                     TEXT NOT NULL DEFAULT '',
	model                    TEXT NOT NULL DEFAULT '',
	plate                    TEXT NOT NULL DEFAULT '',
	vin                      TEXT NOT NULL DEFAULT '',
	fuel_type                TEXT NOT NULL CHECK (fuel_type IN ('petrol','diesel','lpg','cng','hybrid','electric','other')),
	registration_date        TEXT,
	purchase_date            TEXT,
	initial_odometer         INTEGER,
	tank_capacity            REAL,
	-- standard: first inspection after 4 years, then every 2; annual: every year; none
	inspection_rule          TEXT NOT NULL DEFAULT 'standard' CHECK (inspection_rule IN ('standard','annual','none')),
	-- month (1-12) in which the road tax (bollo) expires
	tax_month                INTEGER CHECK (tax_month BETWEEN 1 AND 12),
	tax_exempt_until         TEXT,
	service_interval_km      INTEGER,
	service_interval_months  INTEGER,
	cover_id                 INTEGER,
	notes                    TEXT NOT NULL DEFAULT '',
	archived_at              TEXT,
	created_at               TEXT NOT NULL,
	updated_at               TEXT NOT NULL
);

CREATE TABLE vehicle_users (
	vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role        TEXT NOT NULL CHECK (role IN ('owner','editor','viewer')),
	PRIMARY KEY (vehicle_id, user_id)
);
CREATE INDEX vehicle_users_user ON vehicle_users(user_id);

CREATE TABLE odometer_readings (
	id          INTEGER PRIMARY KEY,
	vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	date        TEXT NOT NULL,
	km          INTEGER NOT NULL,
	notes       TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL
);
CREATE INDEX odometer_readings_vehicle ON odometer_readings(vehicle_id, date);

CREATE TABLE expenses (
	id            INTEGER PRIMARY KEY,
	vehicle_id    INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	date          TEXT NOT NULL,
	category      TEXT NOT NULL CHECK (category IN ('road_tax','inspection','service','maintenance','repair','tyres','parking','tolls','fine','wash','accessories','other')),
	description   TEXT NOT NULL DEFAULT '',
	amount_cents  INTEGER NOT NULL,
	odometer      INTEGER,
	vendor        TEXT NOT NULL DEFAULT '',
	-- road tax only: last month (YYYY-MM) covered by this payment
	valid_until   TEXT,
	notes         TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);
CREATE INDEX expenses_vehicle ON expenses(vehicle_id, date);

CREATE TABLE refuels (
	id                INTEGER PRIMARY KEY,
	vehicle_id        INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	date              TEXT NOT NULL,
	odometer          INTEGER NOT NULL,
	-- litres, kWh or kg depending on the vehicle's fuel type
	quantity          REAL NOT NULL,
	total_cents       INTEGER NOT NULL,
	full_tank         INTEGER NOT NULL DEFAULT 1,
	-- a refuel before this one was not recorded: the consumption segment is not reliable
	missed_previous   INTEGER NOT NULL DEFAULT 0,
	station           TEXT NOT NULL DEFAULT '',
	notes             TEXT NOT NULL DEFAULT '',
	created_at        TEXT NOT NULL,
	updated_at        TEXT NOT NULL
);
CREATE INDEX refuels_vehicle ON refuels(vehicle_id, odometer);

CREATE TABLE policies (
	id             INTEGER PRIMARY KEY,
	vehicle_id     INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	insurer        TEXT NOT NULL DEFAULT '',
	policy_number  TEXT NOT NULL DEFAULT '',
	start_date     TEXT NOT NULL,
	end_date       TEXT NOT NULL,
	premium_cents  INTEGER,
	notes          TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);
CREATE INDEX policies_vehicle ON policies(vehicle_id, end_date);

-- end_date NULL: suspension in progress.
CREATE TABLE policy_suspensions (
	id          INTEGER PRIMARY KEY,
	policy_id   INTEGER NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
	start_date  TEXT NOT NULL,
	end_date    TEXT
);
CREATE INDEX policy_suspensions_policy ON policy_suspensions(policy_id);
CREATE UNIQUE INDEX policy_suspensions_one_open ON policy_suspensions(policy_id) WHERE end_date IS NULL;

-- Custom deadlines. vehicle_id NULL: personal (e.g. driving licence), visible only to user_id.
CREATE TABLE reminders (
	id             INTEGER PRIMARY KEY,
	user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	vehicle_id     INTEGER REFERENCES vehicles(id) ON DELETE CASCADE,
	title          TEXT NOT NULL,
	due_date       TEXT NOT NULL,
	repeat_months  INTEGER,
	notes          TEXT NOT NULL DEFAULT '',
	done_at        TEXT,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);
CREATE INDEX reminders_user ON reminders(user_id);
CREATE INDEX reminders_vehicle ON reminders(vehicle_id);

-- Files live in the files directory under storage_key.
CREATE TABLE attachments (
	id            INTEGER PRIMARY KEY,
	vehicle_id    INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	kind          TEXT NOT NULL CHECK (kind IN ('photo','document')),
	expense_id    INTEGER REFERENCES expenses(id) ON DELETE CASCADE,
	refuel_id     INTEGER REFERENCES refuels(id) ON DELETE CASCADE,
	policy_id     INTEGER REFERENCES policies(id) ON DELETE CASCADE,
	file_name     TEXT NOT NULL,
	content_type  TEXT NOT NULL,
	size          INTEGER NOT NULL,
	storage_key   TEXT NOT NULL UNIQUE,
	created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at    TEXT NOT NULL
);
CREATE INDEX attachments_vehicle ON attachments(vehicle_id, kind);
CREATE INDEX attachments_expense ON attachments(expense_id);
CREATE INDEX attachments_refuel ON attachments(refuel_id);
CREATE INDEX attachments_policy ON attachments(policy_id);
`,
	`
-- Oil change: its own interval and expense category. SQLite cannot alter a
-- CHECK constraint, so the expenses table is rebuilt.
ALTER TABLE vehicles ADD COLUMN oil_interval_km INTEGER;
ALTER TABLE vehicles ADD COLUMN oil_interval_months INTEGER;

CREATE TABLE expenses_new (
	id            INTEGER PRIMARY KEY,
	vehicle_id    INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	date          TEXT NOT NULL,
	category      TEXT NOT NULL CHECK (category IN ('road_tax','inspection','service','oil_change','maintenance','repair','tyres','parking','tolls','fine','wash','accessories','other')),
	description   TEXT NOT NULL DEFAULT '',
	amount_cents  INTEGER NOT NULL,
	odometer      INTEGER,
	vendor        TEXT NOT NULL DEFAULT '',
	valid_until   TEXT,
	notes         TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);
INSERT INTO expenses_new SELECT * FROM expenses;
DROP TABLE expenses;
ALTER TABLE expenses_new RENAME TO expenses;
CREATE INDEX expenses_vehicle ON expenses(vehicle_id, date);
`,
	`
-- Notifications: per-user channels, and what was already sent.
CREATE TABLE notification_settings (
	user_id        INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	email          TEXT NOT NULL DEFAULT '',
	email_enabled  INTEGER NOT NULL DEFAULT 0,
	ntfy_url       TEXT NOT NULL DEFAULT 'https://ntfy.sh',
	ntfy_topic     TEXT NOT NULL DEFAULT '',
	ntfy_token     TEXT NOT NULL DEFAULT '',
	ntfy_enabled   INTEGER NOT NULL DEFAULT 0,
	-- days before a deadline to send a reminder, e.g. '30,7,1'
	days           TEXT NOT NULL DEFAULT '30,7,1',
	updated_at     TEXT NOT NULL
);

-- deadline_key changes when the deadline moves (new due date or km), so a
-- renewed deadline is notified again; stage is 'd30', 'd7', 'soon', 'overdue'...
CREATE TABLE notification_log (
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	deadline_key  TEXT NOT NULL,
	stage         TEXT NOT NULL,
	sent_at       TEXT NOT NULL,
	PRIMARY KEY (user_id, deadline_key, stage)
);
`,
	`
-- Tyres: the sets of a vehicle, and when they were fitted or rotated.
ALTER TABLE vehicles ADD COLUMN tyre_rotation_km INTEGER;

CREATE TABLE tyre_sets (
	id          INTEGER PRIMARY KEY,
	vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	season      TEXT NOT NULL CHECK (season IN ('summer','winter','all_season')),
	brand       TEXT NOT NULL DEFAULT '',
	model       TEXT NOT NULL DEFAULT '',
	size        TEXT NOT NULL DEFAULT '',
	-- DOT date code, week and year of production: '2322'
	dot         TEXT NOT NULL DEFAULT '',
	-- where the set is kept when not fitted, e.g. the tyre shop
	storage     TEXT NOT NULL DEFAULT '',
	notes       TEXT NOT NULL DEFAULT '',
	retired     INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX tyre_sets_vehicle ON tyre_sets(vehicle_id);

-- mount: the set is fitted, and the one fitted before goes to storage;
-- rotate: front and rear tyres of the set are swapped.
CREATE TABLE tyre_events (
	id          INTEGER PRIMARY KEY,
	vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
	set_id      INTEGER NOT NULL REFERENCES tyre_sets(id) ON DELETE CASCADE,
	kind        TEXT NOT NULL CHECK (kind IN ('mount','rotate')),
	date        TEXT NOT NULL,
	odometer    INTEGER NOT NULL,
	notes       TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL
);
CREATE INDEX tyre_events_vehicle ON tyre_events(vehicle_id, date);
`,
	`
-- Login with OpenID Connect: the identity linked to the user (issuer and
-- subject of the ID token). password_hash is '' for users without a password.
ALTER TABLE users ADD COLUMN oidc_issuer TEXT;
ALTER TABLE users ADD COLUMN oidc_subject TEXT;
CREATE UNIQUE INDEX users_oidc ON users(oidc_issuer, oidc_subject) WHERE oidc_subject IS NOT NULL;
`,
}

// migrate applies the pending migrations on a single connection with
// foreign keys off, so that tables can be rebuilt without cascading deletes
// (the procedure recommended by SQLite); integrity is checked before commit.
func migrate(d *sql.DB) error {
	ctx := context.Background()
	c, err := d.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	var v int
	if err := c.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v >= len(migrations) {
		return nil
	}
	if _, err := c.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer c.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
	for i := v; i < len(migrations); i++ {
		if err := migrateOne(ctx, c, i); err != nil {
			return fmt.Errorf("version %d: %w", i+1, err)
		}
	}
	return nil
}

func migrateOne(ctx context.Context, c *sql.Conn, i int) error {
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	broken := rows.Next()
	rows.Close()
	if broken {
		return fmt.Errorf("foreign key check failed")
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
		return err
	}
	return tx.Commit()
}

// Now returns the current UTC timestamp in the format stored in the database.
func Now() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }
