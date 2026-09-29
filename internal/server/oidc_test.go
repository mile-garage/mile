// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/mile-garage/mile/internal/store"
)

// fakeProvider is a minimal OpenID Connect provider: /authorize logs in the
// current identity at once, /token checks the PKCE verifier and signs the ID token.
type fakeProvider struct {
	t      *testing.T
	srv    *httptest.Server
	signer jose.Signer
	key    *rsa.PrivateKey
	codes  map[string]url.Values // code -> authorize request

	sub, username, name string
	groups              []string
}

func newProvider(t *testing.T) *fakeProvider {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{t: t, key: key, signer: signer, codes: map[string]url.Values{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		u := p.srv.URL
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": u + "/", "authorization_endpoint": u + "/authorize", "token_endpoint": u + "/token",
			"jwks_uri": u + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := rand.Text()
		p.codes[code] = q
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		req, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != req.Get("code_challenge") {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims, _ := json.Marshal(map[string]any{
			"iss": p.srv.URL + "/", "aud": "mile", "sub": p.sub, "nonce": req.Get("nonce"),
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
			"preferred_username": p.username, "name": p.name, "groups": p.groups,
		})
		jws, _ := p.signer.Sign(claims)
		idToken, _ := jws.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": idToken})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeProvider) as(sub, username string, groups ...string) {
	p.sub, p.username, p.name, p.groups = sub, username, "Name "+username, groups
}

func newOIDCServer(t *testing.T, p *fakeProvider, configure func(*OIDCConfig)) *httptest.Server {
	app := newApp(t)
	var h http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(ts.Close)
	c := OIDCConfig{Issuer: p.srv.URL + "/", ClientID: "mile", ClientSecret: "secret",
		RedirectURL: ts.URL + "/auth/oidc/callback", Name: "Authentik", AutoRegister: true}
	if configure != nil {
		configure(&c)
	}
	app.SetOIDC(c)
	h = app.Handler()
	return ts
}

// visit follows the redirects of a browser navigation and returns where it ends.
func (c *client) visit(u string) *url.URL {
	c.t.Helper()
	res, err := c.http.Get(u)
	if err != nil {
		c.t.Fatal(err)
	}
	res.Body.Close()
	return res.Request.URL
}

func (c *client) ssoLogin() string {
	c.t.Helper()
	u := c.visit(c.base + "/auth/oidc/login")
	return u.RequestURI()
}

func TestOIDCLogin(t *testing.T) {
	p := newProvider(t)
	ts := newOIDCServer(t, p, func(c *OIDCConfig) { c.AdminGroup = "mile-admins" })

	var status map[string]any
	newClient(t, ts).must("GET", "/api/status", nil, &status)
	if status["sso"] != "Authentik" {
		t.Fatalf("status: %v", status)
	}

	// The first user of a new installation becomes the administrator.
	p.as("sub-1", "gabri")
	first := newClient(t, ts)
	if end := first.ssoLogin(); end != "/" {
		t.Fatalf("first login ended at %s", end)
	}
	var me store.User
	first.must("GET", "/api/me", nil, &me)
	if me.Username != "gabri" || me.DisplayName != "Name gabri" || !me.IsAdmin || !me.SSO || me.HasPassword {
		t.Fatalf("first user: %+v", me)
	}

	// Others are administrators only while they are in the admin group.
	p.as("sub-2", "Anna Rossi", "mile-admins")
	anna := newClient(t, ts)
	anna.ssoLogin()
	anna.must("GET", "/api/me", nil, &me)
	if me.Username != "Anna-Rossi" || !me.IsAdmin {
		t.Fatalf("admin by group: %+v", me)
	}
	p.as("sub-2", "Anna Rossi")
	anna.ssoLogin()
	anna.must("GET", "/api/me", nil, &me)
	if me.IsAdmin {
		t.Fatal("admin role not revoked")
	}
	// ...but the last administrator is never demoted.
	p.as("sub-1", "gabri")
	first.ssoLogin()
	first.must("GET", "/api/me", nil, &me)
	if !me.IsAdmin {
		t.Fatal("last administrator demoted")
	}

	// Without a password there is nothing to log in with, or to confirm.
	if code := anna.do("POST", "/api/login", map[string]string{"username": "Anna-Rossi", "password": ""}, nil); code != 401 {
		t.Fatalf("password login without password: %d", code)
	}
	if code := anna.do("DELETE", "/api/me/sso", nil, nil); code != 400 {
		t.Fatalf("unlink without password: %d", code)
	}
	anna.must("POST", "/api/me/password", map[string]string{"new": "password123"}, nil)
	anna.must("DELETE", "/api/me/sso", nil, &me)
	if me.SSO || !me.HasPassword {
		t.Fatalf("after unlink: %+v", me)
	}
}

func TestOIDCLink(t *testing.T) {
	p := newProvider(t)
	ts := newOIDCServer(t, p, nil)
	admin := newClient(t, ts)
	admin.must("POST", "/api/setup", map[string]string{"username": "mario", "password": "password123"}, nil)

	// A local user with the same name is not taken over: it must link the account itself.
	p.as("sub-mario", "mario")
	other := newClient(t, ts)
	if end := other.ssoLogin(); end != "/?sso_error=sso_username_taken" {
		t.Fatalf("login as an existing user ended at %s", end)
	}

	var link struct{ URL string }
	admin.must("POST", "/api/me/sso", nil, &link)
	if end := admin.visit(link.URL).RequestURI(); end != "/settings?sso=linked" {
		t.Fatalf("link ended at %s", end)
	}
	if end := other.ssoLogin(); end != "/" {
		t.Fatalf("login after linking ended at %s", end)
	}
	var me store.User
	other.must("GET", "/api/me", nil, &me)
	if me.Username != "mario" || !me.SSO || !me.HasPassword {
		t.Fatalf("linked user: %+v", me)
	}

	// The same identity cannot be linked to a second user.
	admin.must("POST", "/api/users", map[string]any{"username": "luigi", "password": "password123"}, nil)
	luigi := newClient(t, ts)
	luigi.must("POST", "/api/login", map[string]string{"username": "luigi", "password": "password123"}, nil)
	luigi.must("POST", "/api/me/sso", nil, &link)
	if end := luigi.visit(link.URL).RequestURI(); end != "/settings?sso_error=sso_linked_other" {
		t.Fatalf("second link ended at %s", end)
	}
}

func TestOIDCRejected(t *testing.T) {
	p := newProvider(t)
	ts := newOIDCServer(t, p, func(c *OIDCConfig) { c.AutoRegister = false })
	p.as("sub-x", "stranger")
	c := newClient(t, ts)
	if end := c.ssoLogin(); end != "/?sso_error=sso_not_registered" {
		t.Fatalf("unregistered user ended at %s", end)
	}
	// A callback that this browser did not start is refused.
	if end := c.visit(ts.URL + "/auth/oidc/callback?code=x&state=y").RequestURI(); end != "/?sso_error=sso_expired" {
		t.Fatalf("forged callback ended at %s", end)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("logged in after failures: %d", code)
	}
	// Without the provider configured the endpoints do not exist.
	plain := newServer(t)
	if res, _ := http.Get(plain.URL + "/auth/oidc/login"); res.StatusCode != 404 {
		t.Fatalf("login with OpenID Connect off: %d", res.StatusCode)
	}
}
