// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

package export

// Column names and values of the spreadsheet format, in the user's language.
// The standard format uses the keys themselves.

var headers = map[string]map[string]string{
	"it": {
		"vehicle_id": "ID veicolo", "vehicle": "Veicolo", "id": "ID", "name": "Nome", "kind": "Tipo", "make": "Marca",
		"model": "Modello", "plate": "Targa", "vin": "Telaio", "fuel_type": "Alimentazione",
		"registration_date": "Immatricolazione", "purchase_date": "Acquisto", "initial_odometer": "Km iniziali",
		"tank_capacity": "Serbatoio", "inspection_rule": "Revisione", "tax_month": "Mese bollo",
		"tax_exempt_until": "Esente bollo fino al", "service_interval_km": "Tagliando ogni km",
		"service_interval_months": "Tagliando ogni mesi", "oil_interval_km": "Olio ogni km",
		"oil_interval_months": "Olio ogni mesi", "tyre_rotation_km": "Inversione gomme ogni km", "archived": "Archiviato",
		"date": "Data", "odometer": "Km", "quantity": "Quantità", "total": "Totale", "unit_price": "Prezzo unitario",
		"full_tank": "Pieno", "missed_previous": "Rifornimento precedente mancante", "station": "Distributore",
		"notes": "Note", "category": "Categoria", "description": "Descrizione", "amount": "Importo", "vendor": "Fornitore",
		"valid_until": "Valido fino a", "insurer": "Compagnia", "policy_number": "Numero polizza",
		"start_date": "Decorrenza", "end_date": "Scadenza", "effective_end": "Scadenza effettiva", "premium": "Premio",
		"suspensions": "Sospensioni", "title": "Titolo", "due_date": "Scadenza", "repeat_months": "Ripeti ogni mesi",
		"done_at": "Completato il", "season": "Tipo", "brand": "Marca", "size": "Misura", "dot": "DOT",
		"storage": "Deposito", "retired": "Dismesse", "mounted": "Montate", "km_driven": "Km percorsi", "event": "Evento",
		"tyre_set": "Treno di gomme", "record": "Riferito a", "record_date": "Data riferimento", "file_name": "Nome file",
		"file": "File nell'archivio", "size_bytes": "Dimensione (byte)",
	},
	"en": {
		"vehicle_id": "Vehicle ID", "vehicle": "Vehicle", "id": "ID", "name": "Name", "kind": "Type", "make": "Make",
		"model": "Model", "plate": "Plate", "vin": "VIN", "fuel_type": "Fuel",
		"registration_date": "Registration", "purchase_date": "Purchase", "initial_odometer": "Initial km",
		"tank_capacity": "Tank", "inspection_rule": "Inspection", "tax_month": "Road tax month",
		"tax_exempt_until": "Road tax exempt until", "service_interval_km": "Service every km",
		"service_interval_months": "Service every months", "oil_interval_km": "Oil every km",
		"oil_interval_months": "Oil every months", "tyre_rotation_km": "Tyre rotation every km", "archived": "Archived",
		"date": "Date", "odometer": "Km", "quantity": "Quantity", "total": "Total", "unit_price": "Unit price",
		"full_tank": "Full tank", "missed_previous": "Previous refuel missing", "station": "Station",
		"notes": "Notes", "category": "Category", "description": "Description", "amount": "Amount", "vendor": "Vendor",
		"valid_until": "Valid until", "insurer": "Insurer", "policy_number": "Policy number",
		"start_date": "Start", "end_date": "Expiry", "effective_end": "Effective expiry", "premium": "Premium",
		"suspensions": "Suspensions", "title": "Title", "due_date": "Due", "repeat_months": "Repeat every months",
		"done_at": "Done on", "season": "Type", "brand": "Brand", "size": "Size", "dot": "DOT",
		"storage": "Storage", "retired": "Retired", "mounted": "Fitted", "km_driven": "Km driven", "event": "Event",
		"tyre_set": "Tyre set", "record": "Belongs to", "record_date": "Record date", "file_name": "File name",
		"file": "File in the archive", "size_bytes": "Size (bytes)",
	},
}

var values = map[string]map[string]string{
	"it": {
		"true": "Sì", "false": "No",
		"car": "Auto", "motorcycle": "Moto", "moped": "Ciclomotore", "van": "Furgone", "truck": "Autocarro",
		"petrol": "Benzina", "diesel": "Diesel", "lpg": "GPL", "cng": "Metano", "hybrid": "Ibrida", "electric": "Elettrica",
		"standard": "Standard", "annual": "Annuale", "none": "Nessuna",
		"road_tax": "Bollo", "inspection": "Revisione", "service": "Tagliando", "oil_change": "Cambio olio",
		"maintenance": "Manutenzione", "repair": "Riparazione", "tyres": "Pneumatici", "parking": "Parcheggio",
		"tolls": "Pedaggi", "fine": "Multe", "wash": "Lavaggio", "accessories": "Accessori", "other": "Altro",
		"summer": "Estive", "winter": "Invernali", "all_season": "4 stagioni",
		"mount": "Montate", "rotate": "Inversione anteriori e posteriori",
		"photo": "Foto", "refuel": "Rifornimento", "expense": "Spesa", "policy": "Polizza",
	},
	"en": {
		"true": "Yes", "false": "No",
		"car": "Car", "motorcycle": "Motorcycle", "moped": "Moped", "van": "Van", "truck": "Truck",
		"petrol": "Petrol", "diesel": "Diesel", "lpg": "LPG", "cng": "CNG", "hybrid": "Hybrid", "electric": "Electric",
		"standard": "Standard", "annual": "Annual", "none": "None",
		"road_tax": "Road tax", "inspection": "Inspection", "service": "Service", "oil_change": "Oil change",
		"maintenance": "Maintenance", "repair": "Repair", "tyres": "Tyres", "parking": "Parking",
		"tolls": "Tolls", "fine": "Fines", "wash": "Car wash", "accessories": "Accessories", "other": "Other",
		"summer": "Summer", "winter": "Winter", "all_season": "All-season",
		"mount": "Fitted", "rotate": "Front/rear rotation",
		"photo": "Photo", "refuel": "Refuel", "expense": "Expense", "policy": "Policy",
	},
}
