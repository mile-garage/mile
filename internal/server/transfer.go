// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"time"

	"github.com/mile-garage/mile/internal/export"
	"github.com/mile-garage/mile/internal/importer"
	"github.com/mile-garage/mile/internal/store"
)

// ---- export ----

// exportOptions reads ?format=spreadsheet|csv&files=1&lang=it|en.
func exportOptions(r *http.Request) export.Options {
	q := r.URL.Query()
	lang := q.Get("lang")
	if lang == "" {
		lang = userOf(r).Locale
	}
	return export.Options{Spreadsheet: q.Get("format") != "csv", Locale: lang, Files: q.Get("files") == "1"}
}

func (s *Server) exportVehicle(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.store.GetVehicle(userOf(r).ID, id)
	if err != nil {
		fail(w, err)
		return
	}
	s.sendExport(w, r, "mile-"+export.Slug(v.Name), []store.Vehicle{*v}, false)
}

// exportAll exports every vehicle the user can see, and their personal reminders.
func (s *Server) exportAll(w http.ResponseWriter, r *http.Request) {
	vs, err := s.store.ListVehicles(userOf(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.sendExport(w, r, "mile", vs, true)
}

func (s *Server) sendExport(w http.ResponseWriter, r *http.Request, name string, vs []store.Vehicle, personal bool) {
	name += "-" + time.Now().Format("2006-01-02") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	// The archive is streamed: an error halfway can only cut it short.
	if err := export.Write(w, s.store, userOf(r).ID, vs, personal, exportOptions(r)); err != nil {
		slog.Error("export", "file", name, "err", err)
	}
}

// ---- import ----

type importResponse struct {
	*store.ImportResult
	InvalidCount int                `json:"invalid_count"`
	Warnings     []importer.Warning `json:"warnings"`
}

// maxInvalid limits the rows listed in the response.
const maxInvalid = 20

// importRecords reads a file of another app into the vehicle. With dry_run=1
// nothing is saved: the frontend shows what would be imported first.
func (s *Server) importRecords(w http.ResponseWriter, r *http.Request, id int64) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("The file is larger than %d MB", s.maxUpload>>20))
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid data")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Missing file")
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		fail(w, err)
		return
	}

	var res *importer.Result
	switch r.FormValue("source") {
	case "fuelio":
		res, err = importer.Fuelio(data)
	case "lubelogger":
		res, err = importer.LubeLogger(data, importer.LubeOptions{
			Kind: r.FormValue("kind"), Units: r.FormValue("units"), DateOrder: r.FormValue("date_order"),
		})
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "Unknown source")
		return
	}
	var ie *importer.Error
	if errors.As(err, &ie) {
		writeError(w, http.StatusBadRequest, ie.Code, ie.Msg)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}

	dryRun := r.FormValue("dry_run") == "1"
	out, err := s.store.Import(id, res.Batch, dryRun)
	if err != nil {
		fail(w, err)
		return
	}
	out.Invalid = append(res.Invalid, out.Invalid...)
	slices.SortFunc(out.Invalid, func(a, b store.InvalidLine) int { return a.Line - b.Line })
	resp := importResponse{ImportResult: out, InvalidCount: len(out.Invalid), Warnings: res.Warnings}
	if resp.Warnings == nil {
		resp.Warnings = []importer.Warning{}
	}
	if len(out.Invalid) > maxInvalid {
		out.Invalid = out.Invalid[:maxInvalid]
	}
	if !dryRun {
		slog.Info("import", "vehicle", id, "source", r.FormValue("source"), "refuels", out.Refuels,
			"expenses", out.Expenses, "odometer", out.Odometer, "duplicates", out.Duplicates, "invalid", resp.InvalidCount)
	}
	writeJSON(w, resp)
}
