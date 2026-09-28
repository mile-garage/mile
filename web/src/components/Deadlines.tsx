import type { ReactNode } from 'react'
import type { Deadline } from '../api'
import { useI18n, type T } from '../i18n'
import { date, num } from '../format'
import { StatusPill } from './ui'

export function relativeDays(n: number, t: T): string {
  if (n === 0) return t('deadline.today')
  if (n === 1) return t('relative.in1')
  if (n === -1) return t('relative.ago1')
  return n > 0 ? t('relative.in', { n }) : t('relative.ago', { n: -n })
}

export function deadlineTitle(d: Deadline, t: T): string {
  if (d.kind === 'reminder') return d.title ?? ''
  if (d.kind === 'inspection' && d.first) return t('deadline.inspection.first')
  return t(`deadline.${d.kind}`)
}

/** One line per deadline: what, when, and how urgent. */
export function DeadlineRow({
  d,
  showVehicle,
  onClick,
  actions,
}: {
  d: Deadline
  showVehicle?: boolean
  onClick?: () => void
  actions?: ReactNode
}) {
  const { t, locale } = useI18n()
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

  return (
    <div className={`deadline deadline-${d.status}${onClick ? ' clickable' : ''}`} onClick={onClick}>
      <div className="deadline-main">
        <div className="deadline-title">
          {deadlineTitle(d, t)}
          {showVehicle && d.vehicle_name && <span className="deadline-vehicle"> · {d.vehicle_name}</span>}
        </div>
        <div className="deadline-detail">{details.join(' · ')}</div>
        {d.estimated && <div className="deadline-note">{t('deadline.estimated')}</div>}
      </div>
      <div className="deadline-side">
        <StatusPill status={d.status} label={t(`status.${d.status}`)} />
        {d.days_left !== undefined && d.status !== 'suspended' && <span className="deadline-rel">{relativeDays(d.days_left, t)}</span>}
        {actions && (
          <div className="deadline-actions" onClick={(e) => e.stopPropagation()}>
            {actions}
          </div>
        )}
      </div>
    </div>
  )
}
