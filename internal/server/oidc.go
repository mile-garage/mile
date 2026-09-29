// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/mile-garage/mile/internal/store"
)

// oidcCookie keeps state, nonce, PKCE verifier and the user who is linking
// the account (0 when logging in) during the round trip to the provider.
const oidcCookie = "mile_oidc"

// OIDCConfig configures the login with an OpenID Connect provider
// (Authentik, Authelia, Keycloak…).
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string // empty for public clients
	RedirectURL  string // <base URL>/auth/oidc/callback
	Name         string // shown on the login button
	AutoRegister bool   // create the users that log in for the first time
	AdminGroup   string // members of this group are administrators
}

type oidcAuth struct {
	OIDCConfig
	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

type oidcClaims struct {
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
}

// SetOIDC enables the login with OpenID Connect.
func (s *Server) SetOIDC(c OIDCConfig) { s.oidc = &oidcAuth{OIDCConfig: c} }

// ready discovers the provider on first use, so that MILE starts even when
// the provider is down.
func (a *oidcAuth) ready(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.provider != nil {
		return nil
	}
	p, err := oidc.NewProvider(ctx, a.Issuer)
	if err != nil {
		return err
	}
	a.provider = p
	a.verifier = p.Verifier(&oidc.Config{ClientID: a.ClientID})
	a.oauth = oauth2.Config{
		ClientID:     a.ClientID,
		ClientSecret: a.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  a.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return nil
}

// identify redeems the code and returns the verified identity of the user.
func (a *oidcAuth) identify(ctx context.Context, code, nonce, verifier string) (store.OIDCIdentity, oidcClaims, error) {
	var c oidcClaims
	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return store.OIDCIdentity{}, c, fmt.Errorf("code exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return store.OIDCIdentity{}, c, errors.New("no id_token in the token response")
	}
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return store.OIDCIdentity{}, c, fmt.Errorf("id_token: %w", err)
	}
	if idt.Nonce != nonce {
		return store.OIDCIdentity{}, c, errors.New("id_token: wrong nonce")
	}
	if err := idt.Claims(&c); err != nil {
		return store.OIDCIdentity{}, c, err
	}
	// Some providers put the profile only in the userinfo response.
	if c.PreferredUsername == "" || (a.AdminGroup != "" && c.Groups == nil) {
		if ui, err := a.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil && ui.Subject == idt.Subject {
			ui.Claims(&c)
		}
	}
	return store.OIDCIdentity{Issuer: idt.Issuer, Subject: idt.Subject}, c, nil
}

// start prepares the round trip to the provider and returns its URL.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request, linkUser int64) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.oidc.ready(ctx); err != nil {
		return "", err
	}
	state, nonce, verifier := rand.Text(), rand.Text(), oauth2.GenerateVerifier()
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: strings.Join([]string{state, nonce, verifier, strconv.FormatInt(linkUser, 10)}, "|"),
		Path: "/auth/oidc", MaxAge: 600, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode,
	})
	return s.oidc.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	url, err := s.oidcStart(w, r, 0)
	if err != nil {
		slog.Error("sso: provider unreachable", "issuer", s.oidc.Issuer, "err", err)
		ssoFail(w, r, "/", "sso_unavailable")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// linkSSO starts linking the identity at the provider to the current user.
func (s *Server) linkSSO(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		writeError(w, http.StatusNotFound, "not_found", "Not found")
		return
	}
	url, err := s.oidcStart(w, r, userOf(r).ID)
	if err != nil {
		slog.Error("sso: provider unreachable", "issuer", s.oidc.Issuer, "err", err)
		writeError(w, http.StatusBadGateway, "sso_unavailable", "The login provider is unreachable")
		return
	}
	writeJSON(w, map[string]string{"url": url})
}

func (s *Server) unlinkSSO(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if err := s.store.UnlinkOIDC(u.ID); err != nil {
		fail(w, err)
		return
	}
	respond(w)(s.store.GetUser(u.ID))
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	c, err := r.Cookie(oidcCookie)
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: "", Path: "/auth/oidc", MaxAge: -1,
		HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode})
	if err != nil {
		ssoFail(w, r, "/", "sso_expired")
		return
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 4 {
		ssoFail(w, r, "/", "sso_expired")
		return
	}
	state, nonce, verifier := parts[0], parts[1], parts[2]
	linkUser, _ := strconv.ParseInt(parts[3], 10, 64)
	back := "/"
	if linkUser != 0 {
		back = "/settings"
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		slog.Info("sso: refused by the provider", "error", e, "description", q.Get("error_description"))
		ssoFail(w, r, back, "sso_denied")
		return
	}
	if state == "" || q.Get("state") != state {
		ssoFail(w, r, back, "sso_expired")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.oidc.ready(ctx); err != nil {
		slog.Error("sso: provider unreachable", "issuer", s.oidc.Issuer, "err", err)
		ssoFail(w, r, back, "sso_unavailable")
		return
	}
	id, claims, err := s.oidc.identify(ctx, q.Get("code"), nonce, verifier)
	if err != nil {
		slog.Error("sso: login failed", "err", err)
		ssoFail(w, r, back, "sso_failed")
		return
	}

	old, _ := r.Cookie(cookieName)
	if linkUser != 0 {
		// The user who started linking must still be logged in.
		var u *store.User
		if old != nil {
			u, err = s.store.SessionUser(old.Value)
		}
		if old == nil || err != nil || u.ID != linkUser {
			ssoFail(w, r, back, "sso_expired")
			return
		}
		if err := s.store.LinkOIDC(u.ID, id); err != nil {
			ssoFail(w, r, back, ssoCode(err))
			return
		}
		slog.Info("sso: account linked", "user", u.Username, "subject", id.Subject)
		http.Redirect(w, r, "/settings?sso=linked", http.StatusFound)
		return
	}

	admin := s.oidc.AdminGroup != "" && slices.Contains(claims.Groups, s.oidc.AdminGroup)
	u, err := s.store.UserByOIDC(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if !s.oidc.AutoRegister {
			ssoFail(w, r, back, "sso_not_registered")
			return
		}
		name := store.OIDCUsername(claims.PreferredUsername, claims.Email, id.Subject)
		u, err = s.store.CreateOIDCUser(id, name, claims.Name, admin)
		if err == nil {
			slog.Info("sso: user created", "username", u.Username, "admin", u.IsAdmin)
		}
	case err == nil && s.oidc.AdminGroup != "" && admin != u.IsAdmin:
		err = s.store.SetAdmin(u.ID, admin)
	}
	if err != nil {
		ssoFail(w, r, back, ssoCode(err))
		return
	}
	token, err := s.store.CreateSession(u.ID)
	if err != nil {
		ssoFail(w, r, back, ssoCode(err))
		return
	}
	if old != nil {
		s.store.DeleteSession(old.Value)
	}
	s.setCookie(w, r, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

// ssoFail sends the browser back to the app, which shows the error.
func ssoFail(w http.ResponseWriter, r *http.Request, back, code string) {
	http.Redirect(w, r, back+"?sso_error="+code, http.StatusFound)
}

func ssoCode(err error) string {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return ve.Code
	}
	slog.Error("sso: internal error", "err", err)
	return "internal"
}
