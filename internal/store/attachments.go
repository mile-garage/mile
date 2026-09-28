// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Attachment struct {
	ID          int64  `json:"id"`
	VehicleID   int64  `json:"vehicle_id"`
	Kind        string `json:"kind"` // photo, document
	ExpenseID   *int64 `json:"expense_id,omitempty"`
	RefuelID    *int64 `json:"refuel_id,omitempty"`
	PolicyID    *int64 `json:"policy_id,omitempty"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	StorageKey  string `json:"-"`
	CreatedAt   string `json:"created_at"`
}

// AllowedTypes are the accepted file types, detected from the content.
// Photos from phones (HEIC) are converted to JPEG in the browser.
var AllowedTypes = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
}

const attachmentCols = `id, vehicle_id, kind, expense_id, refuel_id, policy_id, file_name, content_type, size, storage_key, created_at`

func scanAttachment(row interface{ Scan(...any) error }) (*Attachment, error) {
	var a Attachment
	err := row.Scan(&a.ID, &a.VehicleID, &a.Kind, &a.ExpenseID, &a.RefuelID, &a.PolicyID, &a.FileName, &a.ContentType, &a.Size,
		&a.StorageKey, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

// filePath spreads the files over subdirectories named after the key's first characters.
func (s *Store) filePath(key string) string {
	return filepath.Join(s.FilesDir, key[:2], key)
}

func (s *Store) GetAttachment(id int64) (*Attachment, error) {
	return scanAttachment(s.DB.QueryRow(`SELECT `+attachmentCols+` FROM attachments WHERE id = ?`, id))
}

func (s *Store) OpenAttachment(a *Attachment) (*os.File, error) {
	return os.Open(s.filePath(a.StorageKey))
}

// ListPhotos returns the vehicle's photo gallery.
func (s *Store) ListPhotos(vehicleID int64) ([]Attachment, error) {
	rows, err := s.DB.Query(`SELECT `+attachmentCols+` FROM attachments WHERE vehicle_id = ? AND kind = 'photo' ORDER BY id DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// attachmentsBy groups the vehicle's documents by the record they belong to.
func (s *Store) attachmentsBy(vehicleID int64, fk string) (map[int64][]Attachment, error) {
	rows, err := s.DB.Query(`SELECT `+attachmentCols+` FROM attachments WHERE vehicle_id = ? AND `+fk+` IS NOT NULL ORDER BY id`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		var k int64
		switch fk {
		case "expense_id":
			k = *a.ExpenseID
		case "refuel_id":
			k = *a.RefuelID
		case "policy_id":
			k = *a.PolicyID
		}
		out[k] = append(out[k], *a)
	}
	return out, rows.Err()
}

type NewAttachment struct {
	VehicleID int64
	Kind      string
	ExpenseID *int64
	RefuelID  *int64
	PolicyID  *int64
	FileName  string
	UserID    int64
}

// SaveAttachment stores the file and its record. The content type is
// detected from the first bytes, never taken from the client.
func (s *Store) SaveAttachment(in NewAttachment, r io.Reader) (*Attachment, error) {
	if in.Kind != "photo" && in.Kind != "document" {
		return nil, invalid("kind_invalid", "Invalid attachment type")
	}
	links := 0
	for table, id := range map[string]*int64{"expenses": in.ExpenseID, "refuels": in.RefuelID, "policies": in.PolicyID} {
		if id == nil {
			continue
		}
		links++
		v, err := s.RecordVehicle(table, *id)
		if err != nil || v != in.VehicleID {
			return nil, invalid("record_not_found", "The record to attach the file to was not found")
		}
	}
	if links > 1 {
		return nil, invalid("kind_invalid", "An attachment belongs to one record at most")
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		if errors.Is(err, io.EOF) {
			return nil, invalid("file_empty", "The file is empty")
		}
		return nil, err
	}
	head = head[:n]
	ctype := http.DetectContentType(head)
	if i := strings.IndexByte(ctype, ';'); i >= 0 {
		ctype = ctype[:i]
	}
	ext, ok := AllowedTypes[ctype]
	if !ok || (in.Kind == "photo" && !strings.HasPrefix(ctype, "image/")) {
		return nil, invalid("file_type", "Unsupported file type: use PDF, JPEG, PNG or WebP")
	}

	key := randomHex(16) + ext
	path := s.filePath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(f, io.MultiReader(strings.NewReader(string(head)), r))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return nil, err
	}

	name := Clean(filepath.Base(strings.ReplaceAll(in.FileName, `\`, "/")))
	if name == "" || name == "." || name == "/" {
		name = "file" + ext
	}
	if len(name) > 200 {
		name = name[len(name)-200:]
	}
	res, err := s.DB.Exec(`INSERT INTO attachments (vehicle_id, kind, expense_id, refuel_id, policy_id, file_name, content_type,
		size, storage_key, created_by, created_at) VALUES (`+placeholders(11)+`)`,
		in.VehicleID, in.Kind, in.ExpenseID, in.RefuelID, in.PolicyID, name, ctype, size, key, in.UserID, now())
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAttachment(id)
}

func (s *Store) DeleteAttachment(id int64) error {
	a, err := s.GetAttachment(id)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE vehicles SET cover_id = NULL WHERE cover_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM attachments WHERE id = ?`, id); err != nil {
		return err
	}
	s.removeFiles([]string{a.StorageKey})
	return nil
}
