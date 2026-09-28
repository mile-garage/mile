// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import type { FuelType, Locale } from './api'

const pad = (n: number) => String(n).padStart(2, '0')

export function today(): string {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** Adds months to a YYYY-MM-DD date, clamping the day to the month's end. */
export function addMonths(iso: string, n: number): string {
  const [y, m, d] = iso.split('-').map(Number)
  const first = new Date(Date.UTC(y, m - 1 + n, 1))
  const last = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate()
  return `${first.getUTCFullYear()}-${pad(first.getUTCMonth() + 1)}-${pad(Math.min(d, last))}`
}

/** "2026-09-27" → "27/09/2026" (it) or "27 Sep 2026" (en). */
export function date(iso: string | null | undefined, locale: Locale): string {
  if (!iso) return ''
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  return locale === 'it'
    ? `${pad(d)}/${pad(m)}/${y}`
    : dt.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })
}

/** "2026-09" → "settembre 2026". */
export function month(ym: string, locale: Locale, short = false): string {
  const [y, m] = ym.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString(locale === 'it' ? 'it-IT' : 'en-GB', {
    month: short ? 'short' : 'long',
    year: short ? undefined : 'numeric',
    timeZone: 'UTC',
  })
}

export function monthName(m: number, locale: Locale): string {
  return new Date(Date.UTC(2000, m - 1, 1)).toLocaleDateString(locale === 'it' ? 'it-IT' : 'en-GB', {
    month: 'long',
    timeZone: 'UTC',
  })
}

const tag = (l: Locale) => (l === 'it' ? 'it-IT' : 'en-GB')

export function euro(cents: number | null | undefined, locale: Locale): string {
  if (cents === null || cents === undefined) return ''
  return (cents / 100).toLocaleString(tag(locale), { style: 'currency', currency: 'EUR' })
}

export function num(n: number | null | undefined, locale: Locale, digits = 0): string {
  if (n === null || n === undefined) return ''
  return n.toLocaleString(tag(locale), { minimumFractionDigits: digits, maximumFractionDigits: digits })
}

/** Parses "1.234,56", "1234.56" or "1234,5". Empty → null, invalid → NaN. */
export function parseNumber(s: string): number | null {
  let t = s.trim().replace(/[€\s]/g, '')
  if (t === '') return null
  if (t.includes(',')) t = t.replace(/\./g, '').replace(',', '.')
  else if (/^\d{1,3}(\.\d{3})+$/.test(t)) t = t.replace(/\./g, '') // "45.000" km
  if (!/^-?\d*\.?\d+$/.test(t)) return NaN
  return Number(t)
}

export function parseCents(s: string): number | null {
  const n = parseNumber(s)
  return n === null || Number.isNaN(n) ? n : Math.round(n * 100)
}

export function centsInput(c: number | null | undefined): string {
  return c === null || c === undefined ? '' : (c / 100).toFixed(2).replace('.', ',')
}

export function numInput(n: number | null | undefined): string {
  return n === null || n === undefined ? '' : String(n).replace('.', ',')
}

/** Unit of the fuel quantity for the vehicle. */
export function unit(f: FuelType): string {
  return f === 'electric' ? 'kWh' : f === 'cng' ? 'kg' : 'l'
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
