// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package server exposes the JSON API, the calendar feed and the frontend.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mile-garage/mile/internal/notify"
	"github.com/mile-garage/mile/internal/store"
)

const cookieName = "mile_session"

type Server struct {
	version   string
	notifier  *notify.Runner
	smtpReady bool
	store     *store.Store
	static    fs.FS
	maxUpload int64
	oidc      *oidcAuth // nil when the login with OpenID Connect is off
}

// SetNotifier enables the notification settings and the test button.
func (s *Server) SetNotifier(r *notify.Runner, smtpReady bool) {
	s.notifier, s.smtpReady = r, smtpReady
}

// Types missing from minimal images (distroless has no /etc/mime.types).
func init() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	mime.AddExtensionType(".webp", "image/webp")
}

func New(version string, s *store.Store, static fs.FS, maxUploadMB int) *Server {
	return &Server{version: version, store: s, static: static, maxUpload: int64(maxUploadMB) << 20}
}

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/me", s.me)
	api.HandleFunc("PUT /api/me", s.updateMe)
	api.HandleFunc("POST /api/me/password", s.changePassword)
	api.HandleFunc("GET /api/me/calendar", s.calendarInfo)
	api.HandleFunc("POST /api/me/calendar", s.regenerateCalendar)
	api.HandleFunc("GET /api/me/notifications", s.getNotifications)
	api.HandleFunc("PUT /api/me/notifications", s.saveNotifications)
	api.HandleFunc("POST /api/me/notifications/test", s.testNotifications)
	api.HandleFunc("POST /api/me/sso", s.linkSSO)
	api.HandleFunc("DELETE /api/me/sso", s.unlinkSSO)
	api.HandleFunc("POST /api/logout", s.logout)

	api.HandleFunc("GET /api/users", s.admin(s.listUsers))
	api.HandleFunc("POST /api/users", s.admin(s.createUser))
	api.HandleFunc("DELETE /api/users/{id}", s.admin(s.deleteUser))
	api.HandleFunc("POST /api/users/{id}/password", s.admin(s.resetPassword))

	api.HandleFunc("GET /api/deadlines", s.listDeadlines)

	api.HandleFunc("GET /api/vehicles", s.listVehicles)
	api.HandleFunc("POST /api/vehicles", s.createVehicle)
	api.HandleFunc("GET /api/vehicles/{id}", s.vehicle(store.RoleViewer, s.getVehicle))
	api.HandleFunc("PUT /api/vehicles/{id}", s.vehicle(store.RoleEditor, s.updateVehicle))
	api.HandleFunc("DELETE /api/vehicles/{id}", s.vehicle(store.RoleOwner, s.deleteVehicle))
	api.HandleFunc("POST /api/vehicles/{id}/archive", s.vehicle(store.RoleOwner, s.archiveVehicle))
	api.HandleFunc("PUT /api/vehicles/{id}/cover", s.vehicle(store.RoleEditor, s.setCover))
	api.HandleFunc("GET /api/vehicles/{id}/deadlines", s.vehicle(store.RoleViewer, s.vehicleDeadlines))
	api.HandleFunc("GET /api/vehicles/{id}/stats", s.vehicle(store.RoleViewer, s.vehicleStats))
	api.HandleFunc("GET /api/vehicles/{id}/members", s.vehicle(store.RoleViewer, s.listMembers))
	api.HandleFunc("PUT /api/vehicles/{id}/members", s.vehicle(store.RoleOwner, s.setMember))
	api.HandleFunc("DELETE /api/vehicles/{id}/members/{user}", s.vehicle(store.RoleOwner, s.removeMember))

	api.HandleFunc("GET /api/vehicles/{id}/expenses", s.vehicle(store.RoleViewer, s.listExpenses))
	api.HandleFunc("POST /api/vehicles/{id}/expenses", s.vehicle(store.RoleEditor, s.createExpense))
	api.HandleFunc("PUT /api/expenses/{id}", s.record("expenses", s.updateExpense))
	api.HandleFunc("DELETE /api/expenses/{id}", s.record("expenses", s.deleteExpense))

	api.HandleFunc("GET /api/vehicles/{id}/refuels", s.vehicle(store.RoleViewer, s.listRefuels))
	api.HandleFunc("POST /api/vehicles/{id}/refuels", s.vehicle(store.RoleEditor, s.createRefuel))
	api.HandleFunc("PUT /api/refuels/{id}", s.record("refuels", s.updateRefuel))
	api.HandleFunc("DELETE /api/refuels/{id}", s.record("refuels", s.deleteRefuel))

	api.HandleFunc("GET /api/vehicles/{id}/odometer", s.vehicle(store.RoleViewer, s.listOdometer))
	api.HandleFunc("POST /api/vehicles/{id}/odometer", s.vehicle(store.RoleEditor, s.createOdometer))
	api.HandleFunc("DELETE /api/odometer/{id}", s.record("odometer_readings", s.deleteOdometer))

	api.HandleFunc("GET /api/vehicles/{id}/policies", s.vehicle(store.RoleViewer, s.listPolicies))
	api.HandleFunc("POST /api/vehicles/{id}/policies", s.vehicle(store.RoleEditor, s.createPolicy))
	api.HandleFunc("PUT /api/policies/{id}", s.record("policies", s.updatePolicy))
	api.HandleFunc("DELETE /api/policies/{id}", s.record("policies", s.deletePolicy))
	api.HandleFunc("POST /api/policies/{id}/suspend", s.record("policies", s.suspendPolicy))
	api.HandleFunc("POST /api/policies/{id}/resume", s.record("policies", s.resumePolicy))
	api.HandleFunc("DELETE /api/policies/{id}/suspensions/{sid}", s.record("policies", s.deleteSuspension))

	api.HandleFunc("GET /api/vehicles/{id}/tyres", s.vehicle(store.RoleViewer, s.listTyres))
	api.HandleFunc("POST /api/vehicles/{id}/tyre-sets", s.vehicle(store.RoleEditor, s.createTyreSet))
	api.HandleFunc("PUT /api/tyre-sets/{id}", s.record("tyre_sets", s.updateTyreSet))
	api.HandleFunc("DELETE /api/tyre-sets/{id}", s.record("tyre_sets", s.deleteTyreSet))
	api.HandleFunc("POST /api/vehicles/{id}/tyre-events", s.vehicle(store.RoleEditor, s.createTyreEvent))
	api.HandleFunc("DELETE /api/tyre-events/{id}", s.record("tyre_events", s.deleteTyreEvent))

	api.HandleFunc("GET /api/vehicles/{id}/photos", s.vehicle(store.RoleViewer, s.listPhotos))
	api.HandleFunc("POST /api/vehicles/{id}/attachments", s.vehicle(store.RoleEditor, s.uploadAttachment))
	api.HandleFunc("GET /api/attachments/{id}", s.getAttachment)
	api.HandleFunc("DELETE /api/attachments/{id}", s.record("attachments", s.deleteAttachment))

	api.HandleFunc("GET /api/reminders", s.listReminders)
	api.HandleFunc("POST /api/reminders", s.createReminder)
	api.HandleFunc("PUT /api/reminders/{id}", s.updateReminder)
	api.HandleFunc("DELETE /api/reminders/{id}", s.deleteReminder)
	api.HandleFunc("POST /api/reminders/{id}/done", s.completeReminder)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /auth/oidc/login", s.oidcLogin)
	mux.HandleFunc("GET /auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("GET /calendar/{token}", s.calendarFeed)
	mux.Handle("/api/", s.requireAuth(api))
	mux.Handle("/", s.spa())
	return logRequests(securityHeaders(mux))
}

// ---- authentication ----

type ctxKey struct{}

type session struct {
	user  *store.User
	token string
}

func current(r *http.Request) *session { return r.Context().Value(ctxKey{}).(*session) }

func userOf(r *http.Request) *store.User { return current(r).user }

// requireAuth protects the API. Requests that change data must carry the
// X-Requested-With header, which another site cannot set without CORS.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Login required")
			return
		}
		u, err := s.store.SessionUser(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Login required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Requested-With") == "" {
			writeError(w, http.StatusForbidden, "bad_request", "Invalid request")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, &session{user: u, token: c.Value})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !userOf(r).IsAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "Administrators only")
			return
		}
		h(w, r)
	}
}

// vehicle checks that the user has at least the given role on the vehicle in the path.
func (s *Server) vehicle(min store.Role, h func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if !s.allowed(w, r, id, min) {
			return
		}
		h(w, r, id)
	}
}

// record resolves the vehicle of a record and requires the editor role.
func (s *Server) record(table string, h func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		vid, err := s.store.RecordVehicle(table, id)
		if err != nil {
			fail(w, err)
			return
		}
		if !s.allowed(w, r, vid, store.RoleEditor) {
			return
		}
		h(w, r, id)
	}
}

func (s *Server) allowed(w http.ResponseWriter, r *http.Request, vehicleID int64, min store.Role) bool {
	role, err := s.store.VehicleRole(userOf(r).ID, vehicleID)
	if err != nil {
		fail(w, err)
		return false
	}
	if !role.Allows(min) {
		fail(w, store.ErrForbidden)
		return false
	}
	return true
}

func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", MaxAge: 365 * 24 * 3600,
		HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) {
	token, err := s.store.CreateSession(u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.setCookie(w, r, token)
	writeJSON(w, u)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers()
	if err != nil {
		fail(w, err)
		return
	}
	sso := "" // name of the OpenID Connect provider, if enabled
	if s.oidc != nil {
		sso = s.oidc.Name
	}
	writeJSON(w, map[string]any{"setup_required": n == 0, "version": s.version, "sso": sso})
}

// setup creates the first user (administrator) on a new installation.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Requested-With") == "" {
		writeError(w, http.StatusForbidden, "bad_request", "Invalid request")
		return
	}
	var in store.UserInput
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.store.CreateFirstUser(in)
	if err != nil {
		fail(w, err)
		return
	}
	slog.Info("administrator created", "username", u.Username)
	s.startSession(w, r, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Requested-With") == "" {
		writeError(w, http.StatusForbidden, "bad_request", "Invalid request")
		return
	}
	var in struct{ Username, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.store.Authenticate(in.Username, in.Password)
	if errors.Is(err, store.ErrNotFound) {
		time.Sleep(500 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "wrong_credentials", "Wrong username or password")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	s.startSession(w, r, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.store.DeleteSession(current(r).token)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode})
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- frontend ----

// spa serves the built frontend; unknown paths get index.html.
func (s *Server) spa() http.Handler {
	files := http.FileServerFS(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(s.static, p); err == nil && !st.IsDir() {
				switch {
				case strings.HasPrefix(p, "assets/"):
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				case p == "sw.js":
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		idx, err := fs.ReadFile(s.static, "index.html")
		if err != nil {
			http.Error(w, "Frontend not built: run npm run build in web/", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(idx)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if sw.status >= 400 || r.Method != http.MethodGet {
			slog.Info("api", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": code, "message": text}: the frontend translates
// the code and falls back to the English message.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

func fail(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Code, ve.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, store.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You are not allowed to do this")
	default:
		slog.Error("internal error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Internal server error")
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid data")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "Not found")
		return 0, false
	}
	return id, true
}
