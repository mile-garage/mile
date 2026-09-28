package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mile-garage/mile/internal/db"
	"github.com/mile-garage/mile/internal/store"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newServer(t *testing.T) *httptest.Server {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := store.New(d, filepath.Join(dir, "files"), time.UTC)
	static := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	ts := httptest.NewServer(New(st, static, 5).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func newClient(t *testing.T, ts *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
}

// do sends a JSON request and decodes the response into out (if not nil).
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("X-Requested-With", "fetch")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (c *client) must(method, path string, body any, out any) {
	c.t.Helper()
	var raw json.RawMessage
	if code := c.do(method, path, body, &raw); code != 200 {
		c.t.Fatalf("%s %s: status %d %s", method, path, code, raw)
	}
	if out != nil {
		json.Unmarshal(raw, out)
	}
}

func TestFlow(t *testing.T) {
	ts := newServer(t)
	admin := newClient(t, ts)

	var status map[string]bool
	admin.must("GET", "/api/status", nil, &status)
	if !status["setup_required"] {
		t.Fatal("setup should be required")
	}
	if code := admin.do("GET", "/api/vehicles", nil, nil); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	admin.must("POST", "/api/setup", map[string]string{"username": "gabri", "password": "password123", "locale": "it"}, nil)
	if code := admin.do("POST", "/api/setup", map[string]string{"username": "evil", "password": "password123"}, nil); code != 403 {
		t.Fatalf("second setup: %d", code)
	}

	var v store.Vehicle
	admin.must("POST", "/api/vehicles", map[string]any{
		"make": "Fiat", "model": "Panda", "plate": "ab 123 cd", "kind": "car", "fuel_type": "petrol",
		"registration_date": "2022-05-10", "tax_month": 5, "initial_odometer": 0,
		"service_interval_km": 15000, "service_interval_months": 12, "oil_interval_km": 5000,
	}, &v)
	if v.Name != "Fiat Panda" || v.Plate != "AB123CD" || v.Role != store.RoleOwner {
		t.Fatalf("vehicle: %+v", v)
	}

	vp := "/api/vehicles/" + itoa(v.ID)
	admin.must("POST", vp+"/refuels", map[string]any{"date": "2026-01-01", "odometer": 10000, "quantity": 40, "total_cents": 7000, "full_tank": true}, nil)
	admin.must("POST", vp+"/refuels", map[string]any{"date": "2026-02-01", "odometer": 10600, "quantity": 36, "total_cents": 6300, "full_tank": true}, nil)
	var exp store.Expense
	admin.must("POST", vp+"/expenses", map[string]any{"date": "2026-03-01", "category": "road_tax", "amount_cents": 18000, "valid_until": "2027-04"}, &exp)
	var pol store.Policy
	admin.must("POST", vp+"/policies", map[string]any{"insurer": "Test", "start_date": "2026-01-01", "end_date": "2027-01-01", "premium_cents": 50000}, &pol)
	admin.must("POST", "/api/policies/"+itoa(pol.ID)+"/suspend", map[string]string{"date": "2026-02-01"}, &pol)
	if !pol.Suspended {
		t.Fatal("policy should be suspended")
	}
	admin.must("POST", "/api/policies/"+itoa(pol.ID)+"/resume", map[string]string{"date": "2026-03-03"}, &pol)
	if pol.Suspended || pol.ExtensionDays != 30 || pol.EffectiveEnd != "2027-01-31" {
		t.Fatalf("policy after resume: %+v", pol)
	}

	var ds []map[string]any
	admin.must("GET", "/api/deadlines", nil, &ds)
	kinds := map[string]map[string]any{}
	for _, d := range ds {
		kinds[d["kind"].(string)] = d
	}
	if kinds["inspection"]["due"] != "2026-05-31" {
		t.Errorf("inspection: %v", kinds["inspection"])
	}
	if kinds["road_tax"]["due"] != "2027-05-31" {
		t.Errorf("road tax: %v", kinds["road_tax"])
	}
	if kinds["insurance"]["due"] != "2027-01-31" {
		t.Errorf("insurance: %v", kinds["insurance"])
	}
	if _, ok := kinds["service"]; !ok {
		t.Errorf("service missing: %v", ds)
	}
	// oil change: last oil at 10000 km (refuels), 10600 now; a service resets it
	if kinds["oil_change"]["due_km"] != float64(5000) {
		t.Errorf("oil change from registration: %v", kinds["oil_change"])
	}
	admin.must("POST", vp+"/expenses", map[string]any{"date": "2026-02-02", "category": "oil_change", "amount_cents": 4000, "odometer": 10600}, nil)
	admin.must("GET", "/api/deadlines", nil, &ds)
	for _, d := range ds {
		if d["kind"] == "oil_change" && d["due_km"] != float64(15600) {
			t.Errorf("oil change after change: %v", d)
		}
	}

	var stats store.Stats
	admin.must("GET", vp+"/stats", nil, &stats)
	if stats.Fuel.AvgPer100Km == nil || *stats.Fuel.AvgPer100Km != 6 || stats.Total != 7000+6300+18000+50000+4000 {
		t.Errorf("stats: %+v", stats)
	}

	// attachment: PDF accepted, HTML rejected
	if code := upload(t, admin, vp+"/attachments", "document", itoa(exp.ID), "ricevuta.pdf", []byte("%PDF-1.4\n%test\n")); code != 200 {
		t.Fatalf("pdf upload: %d", code)
	}
	if code := upload(t, admin, vp+"/attachments", "document", "", "x.html", []byte("<html><script>alert(1)</script>")); code != 400 {
		t.Fatalf("html upload: %d", code)
	}
	var exps []store.Expense
	admin.must("GET", vp+"/expenses", nil, &exps)
	if len(exps) != 2 || exps[0].ID != exp.ID || len(exps[0].Attachments) != 1 || exps[0].Attachments[0].ContentType != "application/pdf" {
		t.Fatalf("expense attachments: %+v", exps)
	}
	res, err := admin.http.Get(ts.URL + "/api/attachments/" + itoa(exps[0].Attachments[0].ID))
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %v %v", err, res)
	}
	res.Body.Close()

	// another user cannot see the vehicle until it is shared
	admin.must("POST", "/api/users", map[string]string{"username": "lisa", "password": "password456"}, nil)
	lisa := newClient(t, ts)
	lisa.must("POST", "/api/login", map[string]string{"username": "lisa", "password": "password456"}, nil)
	if code := lisa.do("GET", vp, nil, nil); code != 404 {
		t.Fatalf("not shared: %d", code)
	}
	if code := lisa.do("GET", "/api/attachments/"+itoa(exps[0].Attachments[0].ID), nil, nil); code != 404 {
		t.Fatalf("attachment not shared: %d", code)
	}
	admin.must("PUT", vp+"/members", map[string]string{"username": "lisa", "role": "viewer"}, nil)
	lisa.must("GET", vp, nil, nil)
	if code := lisa.do("POST", vp+"/expenses", map[string]any{"date": "2026-03-01", "category": "wash", "amount_cents": 1000}, nil); code != 403 {
		t.Fatalf("viewer write: %d", code)
	}
	if code := lisa.do("DELETE", "/api/expenses/"+itoa(exp.ID), nil, nil); code != 403 {
		t.Fatalf("viewer delete: %d", code)
	}
	if code := lisa.do("GET", "/api/users", nil, nil); code != 403 {
		t.Fatalf("non-admin users: %d", code)
	}

	// calendar feed
	var cal map[string]string
	admin.must("GET", "/api/me/calendar", nil, &cal)
	res, err = http.Get(ts.URL + cal["path"])
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("calendar: %v %v", err, res)
	}
	var b bytes.Buffer
	b.ReadFrom(res.Body)
	res.Body.Close()
	if !strings.Contains(b.String(), "SUMMARY:Revisione – Fiat Panda") {
		t.Errorf("calendar content:\n%s", b.String())
	}
	if code, _ := http.Get(ts.URL + "/calendar/wrong-token-wrong-token.ics"); code.StatusCode != 404 {
		t.Errorf("wrong token: %d", code.StatusCode)
	}

	// requests without X-Requested-With are refused
	req, _ := http.NewRequest("DELETE", ts.URL+vp, nil)
	res, _ = admin.http.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("CSRF: %d", res.StatusCode)
	}
	admin.must("DELETE", vp, nil, nil)
}

func upload(t *testing.T, c *client, path, kind, expenseID, name string, data []byte) int {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("kind", kind)
	if expenseID != "" {
		mw.WriteField("expense_id", expenseID)
	}
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "fetch")
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func itoa(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}
