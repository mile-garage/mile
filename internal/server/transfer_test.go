// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/mile-garage/mile/internal/store"
)

const fuelioBackup = `"## Vehicle"
"Name","Description","DistUnit","FuelUnit","ConsumptionUnit","ImportCSVDateFormat","VIN","Insurance","Plate","Make","Model","Year","TankCount","Tank1Type","Tank2Type","Active","Tank1Capacity","Tank2Capacity","FuelUnitTank2","FuelConsumptionTank2","guid","lastupdated"
"Panda","","0","0","0","yyyy-MM-dd","","","","Fiat","Panda","2022","1","100","0","1","37.0","0.0","0","0","ac57","1"
"## Log"
"Data","Odo (km)","Fuel (litres)","Full","Price (optional)","l/100km (optional)","latitude (optional)","longitude (optional)","City (optional)","Notes (optional)","Missed","TankNumber","FuelType","VolumePrice","StationID (optional)","ExcludeDistance","UniqueId","TankCalc","Weather","guid","lastupdated"
"2026-02-01 14:42","10600.0","36.5","1","63.87",,"0.0","0.0","Eni","","0","1","119","1.75","0","0.0","2","0.0",,"a","1"
"2026-01-01 08:00","10000.0","40.0","1","70.0",,"0.0","0.0","","","0","1","119","1.75","0","0.0","1","0.0",,"b","1"
"2026-01-15 08:00","10300.0","-3","1","70.0",,"0.0","0.0","","","0","1","119","1.75","0","0.0","3","0.0",,"c","1"
"## Costs"
"CostTitle","Date","Odo","CostTypeID","Notes","Cost","flag","idR","read","RemindOdo","RemindDate","isTemplate","RepeatOdo","RepeatMonths","isIncome","UniqueId","guid","lastupdated"
"Tagliando","2026-01-10 10:00","10100","1","=cmd","250.5","0","0","0","0","2011-01-01","0","0","0","0","51","d","1"
`

type importOut struct {
	store.ImportResult
	InvalidCount int `json:"invalid_count"`
	Warnings     []struct {
		Code string
		N    int
	}
}

func importFile(t *testing.T, c *client, path string, fields map[string]string, data string) (int, importOut, string) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", "export.csv")
	fw.Write([]byte(data))
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "fetch")
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out importOut
	json.Unmarshal(raw, &out)
	return res.StatusCode, out, string(raw)
}

// download returns the files of an exported archive.
func download(t *testing.T, c *client, path string) (map[string]string, http.Header) {
	t.Helper()
	res, err := c.http.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET %s: %d", path, res.StatusCode)
	}
	data, _ := io.ReadAll(res.Body)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		out[f.Name] = string(b)
	}
	return out, res.Header
}

func TestImportExport(t *testing.T) {
	ts := newServer(t)
	c := newClient(t, ts)
	c.must("POST", "/api/setup", map[string]string{"username": "gabri", "password": "password123", "locale": "it"}, nil)
	var v store.Vehicle
	c.must("POST", "/api/vehicles", map[string]any{"name": "Panda", "kind": "car", "fuel_type": "petrol"}, &v)
	vp := "/api/vehicles/" + itoa(v.ID)

	// The dry run tells what would happen and saves nothing.
	fuelio := map[string]string{"source": "fuelio", "dry_run": "1"}
	code, out, raw := importFile(t, c, vp+"/import", fuelio, fuelioBackup)
	if code != 200 || out.Refuels != 2 || out.Expenses != 1 || out.InvalidCount != 1 || out.Invalid[0].Line != 8 || out.Invalid[0].Code != "quantity_invalid" {
		t.Fatalf("dry run: %d %s", code, raw)
	}
	var refuels []store.Refuel
	c.must("GET", vp+"/refuels", nil, &refuels)
	if len(refuels) != 0 {
		t.Fatal("the dry run saved refuels")
	}
	delete(fuelio, "dry_run")
	importFile(t, c, vp+"/import", fuelio, fuelioBackup)
	c.must("GET", vp+"/refuels", nil, &refuels)
	if len(refuels) != 2 || refuels[0].Odometer != 10600 || refuels[0].TotalCents != 6387 || refuels[0].Station != "Eni" {
		t.Fatalf("refuels: %+v", refuels)
	}
	// Importing the same file again adds nothing.
	_, out, raw = importFile(t, c, vp+"/import", fuelio, fuelioBackup)
	if out.Refuels != 0 || out.Expenses != 0 || out.Duplicates != 3 {
		t.Fatalf("second import: %s", raw)
	}

	lube := "Date,Description,Cost,Notes,Odometer,Tags\n03/07/2025,Pastiglie,\"$1,234.50\",,9000,\n"
	code, _, raw = importFile(t, c, vp+"/import", map[string]string{"source": "lubelogger", "kind": "repair"}, lube)
	if code != 400 || !strings.Contains(raw, "import_date_ambiguous") {
		t.Fatalf("ambiguous dates: %d %s", code, raw)
	}
	_, out, _ = importFile(t, c, vp+"/import", map[string]string{"source": "lubelogger", "kind": "repair", "date_order": "mdy"}, lube)
	if out.Expenses != 1 {
		t.Fatalf("lubelogger: %+v", out)
	}
	code, _, raw = importFile(t, c, vp+"/import", map[string]string{"source": "fuelio"}, lube)
	if code != 400 || !strings.Contains(raw, "import_not_fuelio") {
		t.Fatalf("wrong file: %d %s", code, raw)
	}

	var exps []store.Expense
	c.must("GET", vp+"/expenses", nil, &exps)
	if upload(t, c, vp+"/attachments", "document", itoa(exps[len(exps)-1].ID), "fattura.pdf", []byte("%PDF-1.4\n%test\n")) != 200 {
		t.Fatal("upload")
	}

	// Spreadsheet: Italian names and values, ';', decimal comma, BOM, no formulas.
	files, h := download(t, c, vp+"/export?files=1")
	if !strings.HasPrefix(h.Get("Content-Disposition"), `attachment; filename=mile-panda-`) {
		t.Errorf("disposition: %s", h.Get("Content-Disposition"))
	}
	rif := files["rifornimenti.csv"]
	if !strings.HasPrefix(rif, "\xEF\xBB\xBFID veicolo;Veicolo;Data;Km;Quantità;Totale;") ||
		!strings.Contains(rif, ";2026-02-01;10600;36,5;63,87;1,75;Sì;No;Eni;\r\n") {
		t.Errorf("rifornimenti.csv:\n%s", rif)
	}
	spese := files["spese.csv"]
	if !strings.Contains(spese, ";2026-01-10;Tagliando;Tagliando;250,50;10100;;;'=cmd\r\n") ||
		!strings.Contains(spese, ";2025-03-07;Riparazione;Pastiglie;1234,50;9000;") {
		t.Errorf("spese.csv:\n%s", spese)
	}
	if !strings.Contains(files["allegati.csv"], ";Spesa;2025-03-07;fattura.pdf;") {
		t.Errorf("allegati.csv:\n%s", files["allegati.csv"])
	}
	found := false
	for name, data := range files {
		if strings.HasPrefix(name, "files/panda-") && strings.HasSuffix(name, "-fattura.pdf") && strings.HasPrefix(data, "%PDF") {
			found = true
		}
	}
	if !found || len(files) != 10 {
		t.Errorf("archive: %d files, attachment found %v", len(files), found)
	}

	// Standard CSV of everything: keys, ',' and '.'.
	files, _ = download(t, c, "/api/export?format=csv")
	if r := files["refuels.csv"]; !strings.HasPrefix(r, "vehicle_id,vehicle,date,odometer,quantity,total,unit_price,full_tank,") ||
		!strings.Contains(r, ",2026-02-01,10600,36.5,63.87,1.75,true,false,Eni,\r\n") {
		t.Errorf("refuels.csv:\n%s", r)
	}
	if !strings.Contains(files["expenses.csv"], ",service,Tagliando,250.50,10100,,,=cmd\r\n") {
		t.Errorf("expenses.csv:\n%s", files["expenses.csv"])
	}
	if _, ok := files["attachments.csv"]; ok || len(files) != 8 {
		t.Errorf("archive without files: %v", len(files))
	}

	// Viewers export, only editors import.
	c.must("POST", "/api/users", map[string]any{"username": "anna", "password": "password123"}, nil)
	c.must("PUT", vp+"/members", map[string]string{"username": "anna", "role": "viewer"}, nil)
	anna := newClient(t, ts)
	anna.must("POST", "/api/login", map[string]string{"username": "anna", "password": "password123"}, nil)
	download(t, anna, vp+"/export")
	if code, _, _ := importFile(t, anna, vp+"/import", fuelio, fuelioBackup); code != 403 {
		t.Errorf("viewer import: %d", code)
	}
}
