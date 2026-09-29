// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Client for the Go server API.

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'X-Requested-With': 'fetch' }
  let payload: BodyInit | undefined
  if (body instanceof FormData) {
    payload = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  let res: Response
  try {
    res = await fetch(url, { method, headers, body: payload, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'offline', 'Server unreachable')
  }
  if (res.status === 401 && url !== '/api/login') {
    window.dispatchEvent(new Event('mile:unauthorized'))
  }
  const data = res.headers.get('Content-Type')?.includes('application/json') ? await res.json() : null
  if (!res.ok) throw new ApiError(res.status, data?.error ?? 'internal', data?.message ?? `Error ${res.status}`)
  return data as T
}

export const api = {
  get: <T>(url: string) => request<T>('GET', url),
  post: <T>(url: string, body?: unknown) => request<T>('POST', url, body ?? {}),
  put: <T>(url: string, body: unknown) => request<T>('PUT', url, body),
  del: <T>(url: string) => request<T>('DELETE', url),
}

export const fileUrl = (id: number) => `/api/attachments/${id}`

// ---- types ----

export type Locale = 'it' | 'en'

export interface User {
  id: number
  username: string
  display_name: string
  is_admin: boolean
  locale: '' | Locale
  created_at: string
  has_password: boolean
  /** Linked to an account of the login provider (OpenID Connect). */
  sso: boolean
}

export interface Status {
  setup_required: boolean
  version: string
  /** Name of the login provider, '' when the login with OpenID Connect is off. */
  sso: string
}

/** Reads a query parameter set by a server redirect and removes it from the address bar. */
export function takeParam(name: string): string | null {
  const url = new URL(window.location.href)
  const v = url.searchParams.get(name)
  if (v !== null) {
    url.searchParams.delete(name)
    window.history.replaceState(window.history.state, '', url)
  }
  return v
}

export type Role = 'owner' | 'editor' | 'viewer'
export type VehicleKind = 'car' | 'motorcycle' | 'moped' | 'van' | 'truck' | 'other'

/** Motorcycles and mopeds: front and rear tyres differ, they are never rotated. */
export const twoWheels = (k: VehicleKind) => k === 'motorcycle' || k === 'moped'
export type FuelType = 'petrol' | 'diesel' | 'lpg' | 'cng' | 'hybrid' | 'electric' | 'other'
export type InspectionRule = 'standard' | 'annual' | 'none'

export interface VehicleInput {
  name: string
  kind: VehicleKind
  make: string
  model: string
  plate: string
  vin: string
  fuel_type: FuelType
  registration_date: string | null
  purchase_date: string | null
  initial_odometer: number | null
  tank_capacity: number | null
  inspection_rule: InspectionRule
  tax_month: number | null
  tax_exempt_until: string | null
  service_interval_km: number | null
  service_interval_months: number | null
  oil_interval_km: number | null
  oil_interval_months: number | null
  transmission_oil_interval_km: number | null
  transmission_oil_interval_months: number | null
  tyre_rotation_km: number | null
  notes: string
}

export interface Vehicle extends VehicleInput {
  id: number
  cover_id: number | null
  archived: boolean
  role: Role
  current_km: number | null
  created_at: string
  updated_at: string
}

export type DeadlineKind = 'inspection' | 'road_tax' | 'insurance' | 'service' | 'oil_change' | 'transmission_oil' | 'tyre_change' | 'tyre_rotation' | 'reminder'
export type DeadlineStatus = 'overdue' | 'grace' | 'due_soon' | 'ok' | 'suspended'

export interface Deadline {
  kind: DeadlineKind
  vehicle_id?: number
  vehicle_name?: string
  ref_id?: number
  title?: string
  season?: 'summer' | 'winter'
  due?: string
  days_left?: number
  due_km?: number
  km_left?: number
  estimated?: boolean
  first?: boolean
  grace_until?: string
  since?: string
  status: DeadlineStatus
}

export interface Attachment {
  id: number
  vehicle_id: number
  kind: 'photo' | 'document'
  expense_id?: number
  refuel_id?: number
  policy_id?: number
  file_name: string
  content_type: string
  size: number
  created_at: string
}

export type ExpenseCategory =
  | 'road_tax'
  | 'inspection'
  | 'service'
  | 'oil_change'
  | 'transmission_oil'
  | 'maintenance'
  | 'repair'
  | 'tyres'
  | 'parking'
  | 'tolls'
  | 'fine'
  | 'wash'
  | 'accessories'
  | 'other'

export const expenseCategories: ExpenseCategory[] = [
  'service',
  'oil_change',
  'transmission_oil',
  'inspection',
  'road_tax',
  'maintenance',
  'repair',
  'tyres',
  'tolls',
  'parking',
  'fine',
  'wash',
  'accessories',
  'other',
]

export interface ExpenseInput {
  date: string
  category: ExpenseCategory
  description: string
  amount_cents: number
  odometer: number | null
  vendor: string
  valid_until: string | null
  notes: string
}

export interface Expense extends ExpenseInput {
  id: number
  vehicle_id: number
  attachments: Attachment[]
}

export interface RefuelInput {
  date: string
  odometer: number
  quantity: number
  total_cents: number
  full_tank: boolean
  missed_previous: boolean
  station: string
  notes: string
}

export interface Refuel extends RefuelInput {
  id: number
  vehicle_id: number
  unit_price: number | null
  attachments: Attachment[]
}

export interface PolicyInput {
  insurer: string
  policy_number: string
  start_date: string
  end_date: string
  premium_cents: number | null
  notes: string
}

export interface Suspension {
  id: number
  start_date: string
  end_date: string | null
}

export interface Policy extends PolicyInput {
  id: number
  vehicle_id: number
  suspensions: Suspension[]
  attachments: Attachment[]
  effective_end: string
  extension_days: number
  suspended: boolean
}

export interface ReminderInput {
  vehicle_id: number | null
  title: string
  due_date: string
  repeat_months: number | null
  notes: string
}

export interface Reminder extends ReminderInput {
  id: number
  user_id: number
  done_at: string | null
}

export interface OdometerReading {
  id: number
  vehicle_id: number
  date: string
  km: number
  notes: string
}

export interface Member {
  user_id: number
  username: string
  display_name: string
  role: Role
}

export interface FuelSegment {
  from_date: string
  to_date: string
  km: number
  quantity: number
  per_100km: number
  cents: number
}

export interface FuelStats {
  refuels: number
  total_quantity: number
  total_cents: number
  km: number
  avg_per_100km?: number
  last_per_100km?: number
  cents_per_km?: number
  avg_unit_price?: number
  segments: FuelSegment[]
}

export interface MonthCost {
  month: string
  fuel: number
  expenses: number
  insurance: number
}

export interface Stats {
  fuel: FuelStats
  by_category: Record<string, number>
  total: number
  last_12_months: MonthCost[]
  km: number | null
  cents_per_km: number | null
}

export interface NotificationSettings {
  email: string
  email_enabled: boolean
  ntfy_url: string
  ntfy_topic: string
  has_ntfy_token: boolean
  ntfy_enabled: boolean
  days: number[]
  smtp_configured: boolean
  base_url_configured: boolean
}

export interface NotificationInput {
  email: string
  email_enabled: boolean
  ntfy_url: string
  ntfy_topic: string
  /** undefined keeps the saved token, '' removes it */
  ntfy_token?: string
  ntfy_enabled: boolean
  days: number[]
}

export type TyreSeason = 'summer' | 'winter' | 'all_season'

export interface TyreSetInput {
  season: TyreSeason
  brand: string
  model: string
  size: string
  /** DOT date code, week and year: "2322" */
  dot: string
  storage: string
  notes: string
  retired: boolean
}

export interface TyreSet extends TyreSetInput {
  id: number
  vehicle_id: number
  mounted: boolean
  mounted_since: string | null
  last_rotation: string | null
  km: number
  km_since_rotation: number
}

export interface TyreEventInput {
  set_id: number
  kind: 'mount' | 'rotate'
  date: string
  odometer: number
  notes: string
}

export interface TyreEvent extends TyreEventInput {
  id: number
  vehicle_id: number
}

export interface Tyres {
  sets: TyreSet[]
  events: TyreEvent[]
}
