// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"path/filepath"
	"testing"
)

// The oil change migration rebuilds the expenses table: attachments pointing
// to expenses must survive it, and cascades must still work afterwards.
func TestMigrationKeepsAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	all := migrations
	t.Cleanup(func() { migrations = all })

	migrations = all[:1]
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO vehicles (id, name, kind, fuel_type, created_at, updated_at) VALUES (1, 'Vespa', 'motorcycle', 'petrol', '', '')`,
		`INSERT INTO expenses (id, vehicle_id, date, category, amount_cents, created_at, updated_at) VALUES (1, 1, '2026-01-01', 'service', 100, '', '')`,
		`INSERT INTO attachments (vehicle_id, kind, expense_id, file_name, content_type, size, storage_key, created_at)
			VALUES (1, 'document', 1, 'a.pdf', 'application/pdf', 1, 'k1', '')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()

	migrations = all
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM attachments WHERE expense_id = 1`).Scan(&n)
	if n != 1 {
		t.Fatalf("attachment lost: %d", n)
	}
	if _, err := d.Exec(`INSERT INTO expenses (vehicle_id, date, category, amount_cents, created_at, updated_at) VALUES (1, '2026-02-01', 'oil_change', 30, '', '')`); err != nil {
		t.Fatalf("oil_change category: %v", err)
	}
	if _, err := d.Exec(`UPDATE vehicles SET oil_interval_km = 3000 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM expenses WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT COUNT(*) FROM attachments`).Scan(&n)
	if n != 0 {
		t.Fatalf("cascade after migration: %d attachments left", n)
	}
}
