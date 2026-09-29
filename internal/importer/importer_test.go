// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

// A current Fuelio backup (trimmed), with a second tank, an income, a
// template, a station known only by id and a BOM inside a category name.
var fuelioFile = `"## Vehicle"
"Name","Description","DistUnit","FuelUnit","ConsumptionUnit","ImportCSVDateFormat","VIN","Insurance","Plate","Make","Model","Year","TankCount","Tank1Type","Tank2Type","Active","Tank1Capacity","Tank2Capacity","FuelUnitTank2","FuelConsumptionTank2","guid","lastupdated"
"Panda","","0","0","0","yyyy-MM-dd","","","AB123CD","Fiat","Panda","2022","1","100","0","1","37.0","0.0","0","0","ac57","1781573930794"
"## Log"
"Data","Odo (km)","Fuel (litres)","Full","Price (optional)","l/100km (optional)","latitude (optional)","longitude (optional)","City (optional)","Notes (optional)","Missed","TankNumber","FuelType","VolumePrice","StationID (optional)","ExcludeDistance","UniqueId","TankCalc","Weather","guid","lastupdated"
"2025-10-30 14:42","10600.0","36.0","1","63.0","6.0","0.0","0.0","Eni, Via Roma","","0","1","119","1.75","0","0.0","2","0.0",,"e68e","1783723655147"
"2025-10-01 08:00","10000.0","40.0","1","70.0",,"0.0","0.0",,"first","1","1","119","1.75","208772","0.0","1","0.0",,"e68f","1783723655147"
"2025-10-15 08:00","10300.0","20.0","0","20.0",,"0.0","0.0",,"","0","2","401","1.0","0","0.0","3","0.0",,"e690","1783723655147"
"2025-10-16 08:00","abc","20.0","0","20.0",,"0.0","0.0",,"","0","1","119","1.0","0","0.0","4","0.0",,"e691","1783723655147"
"## CostCategories"
"CostTypeID","Name","priority","color","guid","lastupdated"
"1","Service","0","","4d2f","1784993917111"
"8","` + bom + `Tickets/Fines","0","","4d30","1784993917111"
"32","Cambio olio","0","","4d31","1784993917111"
"33","Varie","0","","4d32","1784993917111"
"## Costs"
"CostTitle","Date","Odo","CostTypeID","Notes","Cost","flag","idR","read","RemindOdo","RemindDate","isTemplate","RepeatOdo","RepeatMonths","isIncome","UniqueId","guid","lastupdated"
"Tagliando","2025-09-01 10:00","9800","1","filtri","250.5","0","0","0","0","2011-01-01","0","0","0","0","51","a","1"
"Olio","2025-06-01 10:00","0","32","","45.0","0","0","0","0","2011-01-01","0","0","0","0","52","b","1"
"Multa","2025-05-01","0","8","","42.0","0","0","0","0","2011-01-01","0","0","0","0","53","c","1"
"Boh","2025-05-02","0","33","","1.0","0","0","0","0","2011-01-01","0","0","0","0","54","d","1"
"Rimborso","2025-05-03","0","33","","10.0","0","0","0","0","2011-01-01","0","0","0","1","55","e","1"
"Bollo","2025-01-01","0","4","","180.0","0","0","0","0","2011-01-01","1","0","12","0","56","f","1"
"## FavStations"
"NameBrand","Latitude","Longitude","StationID","Description","CountryCode"
"Q8 Rimini","44.0","12.5","208772","","ITA"
`

func TestFuelio(t *testing.T) {
	res, err := Fuelio([]byte(fuelioFile))
	if err != nil {
		t.Fatal(err)
	}
	b := res.Batch
	if len(b.Refuels) != 2 {
		t.Fatalf("refuels: %+v", b.Refuels)
	}
	r := b.Refuels[0].Rec
	if r.Date != "2025-10-30" || r.Odometer != 10600 || r.Quantity != 36 || r.TotalCents != 6300 || !r.FullTank || r.MissedPrevious || r.Station != "Eni, Via Roma" {
		t.Errorf("refuel: %+v", r)
	}
	if r := b.Refuels[1].Rec; r.Station != "Q8 Rimini" || !r.MissedPrevious || r.Notes != "first" || b.Refuels[1].Line != 7 {
		t.Errorf("refuel with station id: %+v line %d", r, b.Refuels[1].Line)
	}
	if len(res.Invalid) != 1 || res.Invalid[0].Line != 9 {
		t.Errorf("invalid: %+v", res.Invalid)
	}
	cats := []string{}
	for _, e := range b.Expenses {
		cats = append(cats, e.Rec.Category)
	}
	if len(cats) != 4 || cats[0] != "service" || cats[1] != "oil_change" || cats[2] != "fine" || cats[3] != "other" {
		t.Fatalf("categories: %v", cats)
	}
	if e := b.Expenses[0].Rec; e.AmountCents != 25050 || *e.Odometer != 9800 || e.Description != "Tagliando" || e.Notes != "filtri" || e.Date != "2025-09-01" {
		t.Errorf("expense: %+v", e)
	}
	if b.Expenses[1].Rec.Odometer != nil {
		t.Error("odometer 0 should be unknown")
	}
	warn := map[string]int{}
	for _, w := range res.Warnings {
		warn[w.Code] = w.N
	}
	if warn["second_tank_skipped"] != 1 || warn["income_skipped"] != 1 || len(warn) != 2 {
		t.Errorf("warnings: %v", res.Warnings)
	}
}

func TestFuelioImperialZip(t *testing.T) {
	// Old format: no Vehicle header names beyond the basics, units in the log header.
	file := `"## Log"
"Data","Odo (mi)","Fuel (us gallons)","Full","Price (optional)","mpg (optional)"
"2016-05-01","100","10","1","23.5",
`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("vehicle-1-sync.csv")
	w.Write([]byte(file))
	zw.Close()
	res, err := Fuelio(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Batch.Refuels[0].Rec
	if r.Odometer != 161 || r.Quantity != 37.854 || r.TotalCents != 2350 {
		t.Errorf("converted refuel: %+v", r)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != "units_converted" {
		t.Errorf("warnings: %v", res.Warnings)
	}
	if _, err := Fuelio([]byte("Date,Odometer\n2024-01-01,5\n")); !isCode(err, "import_not_fuelio") {
		t.Errorf("not fuelio: %v", err)
	}
}

var bom = string(rune(0xFEFF))

func isCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

func TestLubeLoggerGas(t *testing.T) {
	file := bom + "Date,Odometer,FuelConsumed,Cost,FuelEconomy,IsFillToFull,MissedFuelUp,Notes,Tags,extrafield_Station\r\n" +
		"1/5/2024,45210,11.2,39.87,0,True,False,,,Shell\r\n" +
		"1/19/2024,45530,10.8,38.45,29.62962962962962962962962963,True,False,Highway trip,roadtrip,Costco\r\n" +
		"2/2/2024,0,4.2,15.10,0,False,False,,,\r\n" +
		"2/9/2024,46125,11.9,42.60,36.95,False,True,\"Rain, heavy traffic\",,Shell\r\n"
	res, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeGas, Units: "mi_us"})
	if err != nil {
		t.Fatal(err)
	}
	b := res.Batch.Refuels
	if len(b) != 3 {
		t.Fatalf("refuels: %+v", b)
	}
	if r := b[1].Rec; r.Date != "2024-01-19" || r.Odometer != 73273 || r.Quantity != 40.882 || r.TotalCents != 3845 || !r.FullTank ||
		r.Notes != "Highway trip\nTag: roadtrip\nStation: Costco" {
		t.Errorf("refuel: %+v", r)
	}
	if r := b[2].Rec; r.FullTank || !r.MissedPrevious || b[2].Line != 5 {
		t.Errorf("partial refuel: %+v line %d", r, b[2].Line)
	}
	if _, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeService}); !isCode(err, "import_wrong_kind") {
		t.Errorf("gas file as service: %v", err)
	}
}

func TestLubeLoggerItalian(t *testing.T) {
	file := "Date,Description,Cost,Notes,Odometer,Tags\r\n" +
		"14/03/2024,Tagliando,\"1.234,50 €\",,46010,\r\n" +
		"02/04/2024,Filtro,\"45,00 €\",\"Cambiato\nanche olio\",0,officina\r\n"
	res, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeRepair})
	if err != nil {
		t.Fatal(err)
	}
	e := res.Batch.Expenses
	if len(e) != 2 || e[0].Rec.Date != "2024-03-14" || e[0].Rec.AmountCents != 123450 || *e[0].Rec.Odometer != 46010 || e[0].Rec.Category != "repair" {
		t.Fatalf("expenses: %+v", e)
	}
	if r := e[1].Rec; r.Date != "2024-04-02" || r.AmountCents != 4500 || r.Odometer != nil || r.Notes != "Cambiato\nanche olio\nTag: officina" {
		t.Errorf("expense: %+v", r)
	}
	if _, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeTax}); !isCode(err, "import_wrong_kind") {
		t.Errorf("repair file as tax: %v", err)
	}
}

func TestLubeLoggerDates(t *testing.T) {
	file := "Date,Description,Cost,Notes,Tags\n03/07/2024,Bollo,¤145.00,,\n01/02/2024,Bollo,$1.00,,\n"
	if _, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeTax}); !isCode(err, "import_date_ambiguous") {
		t.Fatalf("ambiguous dates: %v", err)
	}
	res, err := LubeLogger([]byte(file), LubeOptions{Kind: LubeTax, DateOrder: "mdy"})
	if err != nil {
		t.Fatal(err)
	}
	if e := res.Batch.Expenses[0].Rec; e.Date != "2024-03-07" || e.AmountCents != 14500 || e.Category != "road_tax" {
		t.Errorf("tax: %+v", e)
	}
	iso := "Date,InitialOdometer,Odometer,Notes,Tags\n2024-03-07,100,46480,Commute,\n"
	res, err = LubeLogger([]byte(iso), LubeOptions{Kind: LubeOdometer})
	if err != nil {
		t.Fatal(err)
	}
	if o := res.Batch.Odometer[0].Rec; o.Date != "2024-03-07" || o.Km != 46480 || o.Notes != "Commute" {
		t.Errorf("odometer: %+v", o)
	}
}

func TestNumbers(t *testing.T) {
	for _, c := range []struct {
		values []string
		dec    byte
	}{
		{[]string{"$1,234.50"}, '.'},
		{[]string{"1.234,50 €"}, ','},
		{[]string{"1 234,50 kr"}, ','},
		{[]string{"1,234", "41,3"}, ','},
		{[]string{"1,234", ""}, '.'},
		{[]string{"7,942307692307692307692307692"}, ','},
	} {
		if d := decimalSeparator(c.values); d != c.dec {
			t.Errorf("%v: %c, want %c", c.values, d, c.dec)
		}
	}
	if v, _ := amount("1.234,50 €", ','); v != 1234.5 {
		t.Errorf("amount: %v", v)
	}
	if v, _ := amount("¤1,234.50", '.'); v != 1234.5 {
		t.Errorf("amount: %v", v)
	}
}

func TestCategory(t *testing.T) {
	for name, want := range map[string]string{
		"Cambio olio": "oil_change", "Olio cambio automatico": "transmission_oil", "ATF": "transmission_oil", "Revisione": "inspection", "MOT": "inspection", "Motore": "",
		"Gomme invernali": "tyres", "Pedaggi": "tolls", "Assicurazione": "",
	} {
		if got, _ := category(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}
