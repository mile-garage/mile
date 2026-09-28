// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mile-garage/mile/internal/ical"
	"github.com/mile-garage/mile/internal/store"
)

// ---- profile ----

func (s *Server) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, userOf(r)) }

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
		Locale      string `json:"locale"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.store.UpdateProfile(userOf(r).ID, in.DisplayName, in.Locale)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	sess := current(r)
	if !s.store.CheckPassword(sess.user.ID, in.Current) {
		time.Sleep(500 * time.Millisecond)
		writeError(w, http.StatusBadRequest, "wrong_password", "The current password is wrong")
		return
	}
	if err := s.store.SetPassword(sess.user.ID, in.New, sess.token); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) calendarInfo(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.ICalToken(userOf(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"path": "/calendar/" + t + ".ics"})
}

func (s *Server) regenerateCalendar(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.RegenerateICalToken(userOf(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"path": "/calendar/" + t + ".ics"})
}

// calendarFeed serves the user's deadlines as an iCalendar feed. The secret
// token in the URL replaces the login, since calendar apps cannot log in.
func (s *Server) calendarFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("token"), ".ics")
	u, err := s.store.UserByICalToken(token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ds, err := s.store.Deadlines(u.ID, nil)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(ical.Feed(ds, u.Locale, time.Now()))
}

// ---- users (admin) ----

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.store.ListUsers()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, us)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in store.UserInput
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.store.CreateUser(in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if id == userOf(r).ID {
		writeError(w, http.StatusBadRequest, "delete_self", "You cannot delete your own account")
		return
	}
	if err := s.store.DeleteUser(id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.store.SetPassword(id, in.Password, current(r).token); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- deadlines ----

func (s *Server) listDeadlines(w http.ResponseWriter, r *http.Request) {
	ds, err := s.store.Deadlines(userOf(r).ID, nil)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, ds)
}

func (s *Server) vehicleDeadlines(w http.ResponseWriter, r *http.Request, id int64) {
	ds, err := s.store.Deadlines(userOf(r).ID, &id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, ds)
}

// ---- vehicles ----

func (s *Server) listVehicles(w http.ResponseWriter, r *http.Request) {
	vs, err := s.store.ListVehicles(userOf(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, vs)
}

func (s *Server) createVehicle(w http.ResponseWriter, r *http.Request) {
	var in store.VehicleInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := s.store.CreateVehicle(userOf(r).ID, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) getVehicle(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.store.GetVehicle(userOf(r).ID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) updateVehicle(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.VehicleInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := s.store.UpdateVehicle(userOf(r).ID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) deleteVehicle(w http.ResponseWriter, r *http.Request, id int64) {
	if err := s.store.DeleteVehicle(id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) archiveVehicle(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Archived bool `json:"archived"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.store.SetArchived(id, in.Archived); err != nil {
		fail(w, err)
		return
	}
	s.getVehicle(w, r, id)
}

func (s *Server) setCover(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		AttachmentID *int64 `json:"attachment_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.store.SetCover(id, in.AttachmentID); err != nil {
		fail(w, err)
		return
	}
	s.getVehicle(w, r, id)
}

func (s *Server) vehicleStats(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.store.GetVehicle(userOf(r).ID, id)
	if err != nil {
		fail(w, err)
		return
	}
	st, err := s.store.VehicleStats(v)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, st)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, id int64) {
	ms, err := s.store.ListMembers(id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, ms)
}

func (s *Server) setMember(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Username string     `json:"username"`
		Role     store.Role `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.store.SetMember(id, in.Username, in.Role); err != nil {
		fail(w, err)
		return
	}
	s.listMembers(w, r, id)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, id int64) {
	uid, ok := pathID(w, r, "user")
	if !ok {
		return
	}
	if err := s.store.RemoveMember(id, uid); err != nil {
		fail(w, err)
		return
	}
	s.listMembers(w, r, id)
}

// ---- expenses, refuels, odometer ----

func (s *Server) listExpenses(w http.ResponseWriter, r *http.Request, id int64) {
	respond(w)(s.store.ListExpenses(id))
}

func (s *Server) createExpense(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.ExpenseInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.CreateExpense(id, in))
	}
}

func (s *Server) updateExpense(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.ExpenseInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.UpdateExpense(id, in))
	}
}

func (s *Server) deleteExpense(w http.ResponseWriter, r *http.Request, id int64) {
	done(w, s.store.DeleteExpense(id))
}

func (s *Server) listRefuels(w http.ResponseWriter, r *http.Request, id int64) {
	respond(w)(s.store.ListRefuels(id))
}

func (s *Server) createRefuel(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.RefuelInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.CreateRefuel(id, in))
	}
}

func (s *Server) updateRefuel(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.RefuelInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.UpdateRefuel(id, in))
	}
}

func (s *Server) deleteRefuel(w http.ResponseWriter, r *http.Request, id int64) {
	done(w, s.store.DeleteRefuel(id))
}

func (s *Server) listOdometer(w http.ResponseWriter, r *http.Request, id int64) {
	respond(w)(s.store.ListOdometer(id))
}

func (s *Server) createOdometer(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.OdometerInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.CreateOdometer(id, in))
	}
}

func (s *Server) deleteOdometer(w http.ResponseWriter, r *http.Request, id int64) {
	done(w, s.store.DeleteOdometer(id))
}

// ---- policies ----

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request, id int64) {
	respond(w)(s.store.ListPolicies(id))
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.PolicyInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.CreatePolicy(id, in))
	}
}

func (s *Server) updatePolicy(w http.ResponseWriter, r *http.Request, id int64) {
	var in store.PolicyInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.UpdatePolicy(id, in))
	}
}

func (s *Server) deletePolicy(w http.ResponseWriter, r *http.Request, id int64) {
	done(w, s.store.DeletePolicy(id))
}

type dateBody struct {
	Date string `json:"date"`
}

func (s *Server) suspendPolicy(w http.ResponseWriter, r *http.Request, id int64) {
	var in dateBody
	if readJSON(w, r, &in) {
		respond(w)(s.store.Suspend(id, in.Date))
	}
}

func (s *Server) resumePolicy(w http.ResponseWriter, r *http.Request, id int64) {
	var in dateBody
	if readJSON(w, r, &in) {
		respond(w)(s.store.Resume(id, in.Date))
	}
}

func (s *Server) deleteSuspension(w http.ResponseWriter, r *http.Request, id int64) {
	sid, ok := pathID(w, r, "sid")
	if ok {
		respond(w)(s.store.DeleteSuspension(id, sid))
	}
}

// ---- attachments ----

func (s *Server) listPhotos(w http.ResponseWriter, r *http.Request, id int64) {
	respond(w)(s.store.ListPhotos(id))
}

// uploadAttachment receives a multipart form with "file", "kind" and
// optionally one of expense_id, refuel_id, policy_id. The file is streamed to
// disk without buffering it in memory.
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid data")
		return
	}
	in := store.NewAttachment{VehicleID: id, UserID: userOf(r).ID}
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Missing file")
			return
		}
		if part.FormName() != "file" {
			b, _ := io.ReadAll(io.LimitReader(part, 256))
			fields[part.FormName()] = string(b)
			continue
		}
		in.Kind = fields["kind"]
		for key, dst := range map[string]**int64{"expense_id": &in.ExpenseID, "refuel_id": &in.RefuelID, "policy_id": &in.PolicyID} {
			if v, err := strconv.ParseInt(fields[key], 10, 64); err == nil && v > 0 {
				*dst = &v
			}
		}
		in.FileName = part.FileName()
		a, err := s.store.SaveAttachment(in, part)
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("The file is larger than %d MB", s.maxUpload>>20))
			return
		}
		respond(w)(a, err)
		return
	}
}

func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	a, err := s.store.GetAttachment(id)
	if err != nil {
		fail(w, err)
		return
	}
	if !s.allowed(w, r, a.VehicleID, store.RoleViewer) {
		return
	}
	f, err := s.store.OpenAttachment(a)
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	disp := "inline"
	if r.URL.Query().Has("download") {
		disp = "attachment"
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": a.FileName}))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	// PDFs are shown by the browser's viewer: no scripts, no embedding elsewhere.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'self'")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	done(w, s.store.DeleteAttachment(id))
}

// ---- reminders ----

func (s *Server) listReminders(w http.ResponseWriter, r *http.Request) {
	var vid *int64
	if v, err := strconv.ParseInt(r.URL.Query().Get("vehicle"), 10, 64); err == nil {
		vid = &v
	}
	respond(w)(s.store.ListReminders(userOf(r).ID, vid, r.URL.Query().Has("done")))
}

func (s *Server) createReminder(w http.ResponseWriter, r *http.Request) {
	var in store.ReminderInput
	if readJSON(w, r, &in) {
		respond(w)(s.store.CreateReminder(userOf(r).ID, in))
	}
}

func (s *Server) updateReminder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.ReminderInput
	if ok && readJSON(w, r, &in) {
		respond(w)(s.store.UpdateReminder(userOf(r).ID, id, in))
	}
}

func (s *Server) deleteReminder(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		done(w, s.store.DeleteReminder(userOf(r).ID, id))
	}
}

func (s *Server) completeReminder(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		respond(w)(s.store.CompleteReminder(userOf(r).ID, id))
	}
}

// ---- helpers ----

// respond writes the value, or the error.
func respond(w http.ResponseWriter) func(any, error) {
	return func(v any, err error) {
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, v)
	}
}

func done(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
