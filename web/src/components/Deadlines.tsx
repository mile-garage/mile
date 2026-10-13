// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { api, type Deadline, type DeadlineKind, type ExpenseInput, type Reminder, type Vehicle } from '../api'
import { useI18n, type T } from '../i18n'
import { addMonths, date, num } from '../format'
import { ExpenseForm, ReminderForm } from './Forms'
import { Icon } from './Icon'
import { notifyChanged, useUI } from './ui'

export function relativeDays(n: number, t: T): string {
  if (n === 0) return t('deadline.today')
  if (n === 1) return t('relative.in1')
  if (n === -1) return t('relative.ago1')
  return n > 0 ? t('relative.in', { n }) : t('relative.ago', { n: -n })
}

/** Compact "when" for the pill: "33 g", "13 g fa", "5 mesi", "1.200 km". */
export function shortWhen(d: Deadline, t: T, locale: 'it' | 'en'): string {
  const n = d.days_left
  if (n === undefined) {
    if (d.km_left === undefined) return ''
    return d.km_left >= 0 ? t('short.km', { km: num(d.km_left, locale) }) : t('short.kmOver', { km: num(-d.km_left, locale) })
  }
  if (n === 0) return t('short.today')
  if (n < 0) return t('short.daysAgo', { n: -n })
  if (n <= 90) return t('short.days', { n })
  if (n < 730) return t('short.months', { n: Math.round(n / 30.4) })
  return t('short.years', { n: Math.round(n / 365) })
}

export function deadlineTitle(d: Deadline, t: T): string {
  if (d.kind === 'reminder') return d.title ?? ''
  if (d.kind === 'inspection' && d.first) return t('deadline.inspection.first')
  if (d.kind === 'tyre_change') return t(d.season === 'summer' ? 'deadline.tyre_change.summer' : 'deadline.tyre_change.winter')
  return t(`deadline.${d.kind}`)
}

const icons: Record<DeadlineKind, string> = {
  inspection: 'inspection',
  road_tax: 'receipt',
  insurance: 'shield',
  service: 'wrench',
  oil_change: 'wrench',
  transmission_oil: 'wrench',
  brake_pads: 'wrench',
  brake_discs: 'wrench',
  tyre_change: 'tyre',
  tyre_rotation: 'tyre',
  reminder: 'bell',
}

/** Km interval of a deadline, to draw how much of it has been used. */
function intervalKm(d: Deadline, v?: Vehicle): number | null {
  if (!v) return null
  switch (d.kind) {
    case 'service':
      return v.service_interval_km
    case 'oil_change':
      return v.oil_interval_km
    case 'transmission_oil':
      return v.transmission_oil_interval_km
    case 'brake_pads':
      return v.brake_pads_interval_km
    case 'brake_discs':
      return v.brake_discs_interval_km
    case 'tyre_rotation':
      return v.tyre_rotation_km
  }
  return null
}

export function deadlineDetail(d: Deadline, t: T, locale: 'it' | 'en'): string {
  const details: string[] = []
  if (d.due) {
    let s = t(d.kind === 'insurance' ? 'deadline.expires' : 'deadline.due', { date: date(d.due, locale) })
    if (d.due_km !== undefined) s += ' ' + t('deadline.byKm', { km: num(d.due_km, locale) })
    details.push(s)
  } else if (d.due_km !== undefined) {
    details.push(t('deadline.kmOnly', { km: num(d.due_km, locale) }))
  }
  if (d.km_left !== undefined) {
    details.push(d.km_left >= 0 ? t('deadline.kmLeft', { km: num(d.km_left, locale) }) : t('deadline.kmOver', { km: num(-d.km_left, locale) }))
  }
  if (d.status === 'grace' && d.grace_until) details.push(t('deadline.grace', { date: date(d.grace_until, locale) }))
  if (d.status === 'suspended' && d.since) details.push(t('deadline.suspended', { date: date(d.since, locale) }))
  return details.join(' · ')
}

/** One line per deadline: what, when, and how urgent. */
export function DeadlineRow({
  d,
  vehicle,
  showVehicle,
  full,
  onClick,
  actions,
}: {
  d: Deadline
  vehicle?: Vehicle
  showVehicle?: boolean
  /** Also shows the notes (estimated date, tyre rules). */
  full?: boolean
  onClick?: () => void
  actions?: ReactNode
}) {
  const { t, locale } = useI18n()
  const every = intervalKm(d, vehicle)
  const used = every && d.km_left !== undefined ? Math.min(1, Math.max(0, 1 - d.km_left / every)) : null
  const tone = d.status === 'overdue' || d.status === 'grace' ? 'overdue' : d.status === 'ok' ? 'ok' : ''
  return (
    <div
      className={`deadline${onClick ? ' clickable' : ''}`}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), onClick()) : undefined}
    >
      <span className="row-icon">
        <Icon name={icons[d.kind]} />
      </span>
      <div className="deadline-main">
        <div className="deadline-title">
          {deadlineTitle(d, t)}
          {showVehicle && <span className="deadline-vehicle"> {d.vehicle_name ?? t('home.personal')}</span>}
        </div>
        <div className="deadline-detail">{deadlineDetail(d, t, locale)}</div>
        {full && d.estimated && <div className="deadline-note">{t('deadline.estimated')}</div>}
        {full && d.kind === 'tyre_change' && (d.days_left ?? 0) <= 45 && <div className="deadline-note">{t(d.season === 'summer' ? 'deadline.tyreRule.summer' : 'deadline.tyreRule.winter')}</div>}
        {used !== null && (
          <span className={`bar${tone ? ` bar-${tone}` : ''}`} aria-hidden="true">
            <span style={{ width: `${Math.round(used * 100)}%` }} />
          </span>
        )}
      </div>
      <div className="deadline-side">
        <span className={`pill pill-${d.status}`} title={t(`status.${d.status}`)}>
          {d.status === 'suspended' ? t('status.suspended') : shortWhen(d, t, locale)}
        </span>
        {actions && (
          <div className="deadline-actions" onClick={(e) => e.stopPropagation()}>
            {actions}
          </div>
        )}
      </div>
    </div>
  )
}

/** Tab of the vehicle page where a deadline is handled. */
export function deadlineTab(d: Deadline): string {
  if (d.kind === 'insurance') return 'docs'
  if (d.kind === 'tyre_change' || d.kind === 'tyre_rotation') return 'tyres'
  return 'status'
}

/** Expense that marks a deadline as done, e.g. the service or the road tax. */
export function expensePreset(d: Deadline, v: Vehicle): Partial<ExpenseInput> | null {
  const km = v.current_km
  switch (d.kind) {
    case 'inspection':
    case 'service':
    case 'oil_change':
    case 'transmission_oil':
    case 'brake_pads':
    case 'brake_discs':
      return { category: d.kind, odometer: km }
    case 'road_tax':
      // a payment made for the deadline covers the 12 months after the expired one
      return { category: 'road_tax', valid_until: d.due ? addMonths(d.due, 11).slice(0, 7) : null }
  }
  return null
}

export function actionLabel(d: Deadline, t: T): string {
  switch (d.kind) {
    case 'reminder':
      return t('deadline.done')
    case 'road_tax':
      return t('action.paid')
    case 'insurance':
      return t('policy.renew')
    case 'tyre_change':
      return t('action.tyres')
    case 'tyre_rotation':
      return t('action.rotate')
  }
  return t('action.done')
}

/**
 * What happens when a deadline is tapped: the main action marks it as done
 * (an expense, a reminder done) or opens the tab where it is handled.
 */
export function useDeadlineActions(vehicles: Vehicle[]) {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const navigate = useNavigate()
  const [expense, setExpense] = useState<{ vehicle: Vehicle; preset: Partial<ExpenseInput> } | null>(null)
  const [reminder, setReminder] = useState<Reminder | null>(null)

  const canEdit = (d: Deadline) => {
    const v = vehicles.find((x) => x.id === d.vehicle_id)
    return d.kind === 'reminder' ? !v || v.role !== 'viewer' : !!v && v.role !== 'viewer'
  }

  const run = async (d: Deadline, main: boolean) => {
    const v = vehicles.find((x) => x.id === d.vehicle_id)
    if (d.kind === 'reminder') {
      try {
        if (main) {
          const r = await api.post<Reminder>(`/api/reminders/${d.ref_id}/done`)
          toast(r.done_at ? t('common.saved') : t('reminder.doneRepeat', { date: date(r.due_date, locale) }))
          notifyChanged()
        } else {
          const all = await api.get<Reminder[]>('/api/reminders')
          const r = all.find((x) => x.id === d.ref_id)
          if (r) setReminder(r)
        }
      } catch (e) {
        fail(e)
      }
      return
    }
    if (!v) return
    const preset = main && v.role !== 'viewer' ? expensePreset(d, v) : null
    if (preset) setExpense({ vehicle: v, preset })
    else navigate(`/vehicles/${v.id}?tab=${deadlineTab(d)}`)
  }

  const element = (
    <>
      {expense && (
        <ExpenseForm
          vehicle={expense.vehicle}
          preset={expense.preset}
          onClose={() => setExpense(null)}
          onSaved={() => {
            setExpense(null)
            notifyChanged()
          }}
        />
      )}
      {reminder && (
        <ReminderForm
          vehicles={vehicles}
          reminder={reminder}
          onClose={() => setReminder(null)}
          onSaved={() => {
            setReminder(null)
            notifyChanged()
          }}
        />
      )}
    </>
  )
  return { run, canEdit, element }
}
