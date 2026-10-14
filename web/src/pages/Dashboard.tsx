// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from 'react'
import { Link } from 'react-router'
import { type Deadline, type Vehicle } from '../api'
import { useI18n } from '../i18n'
import { actionLabel, deadlineDetail, DeadlineRow, deadlineTab, deadlineTitle, relativeDays, useDeadlineActions } from '../components/Deadlines'
import { ReminderForm } from '../components/Forms'
import { Icon } from '../components/Icon'
import { Empty, Loading, notifyChanged, useLoad } from '../components/ui'

/** Deadlines within this many days are listed first, the others under "Later". */
const SOON = 90

export function Dashboard() {
  const { t, locale } = useI18n()
  const deadlines = useLoad<Deadline[]>('/api/deadlines')
  const vehicles = useLoad<Vehicle[]>('/api/vehicles')
  const [filter, setFilter] = useState<number | null>(null)
  const [newReminder, setNewReminder] = useState(false)
  const actions = useDeadlineActions(vehicles.data ?? [])

  if (!deadlines.data || !vehicles.data) return <Loading />
  const active = vehicles.data.filter((v) => !v.archived)
  const byId = new Map(vehicles.data.map((v) => [v.id, v]))
  const shown = deadlines.data.filter((d) => filter === null || d.vehicle_id === filter)
  const [next, ...rest] = shown
  const soon = rest.filter((d) => d.status !== 'ok' || (d.days_left ?? Infinity) <= SOON)
  const later = rest.filter((d) => !soon.includes(d))
  const today = new Date().toLocaleDateString(locale === 'it' ? 'it-IT' : 'en-GB', { weekday: 'long', day: 'numeric', month: 'long' })

  const list = (ds: Deadline[]) => (
    <div className="card list">
      {ds.map((d) => (
        <DeadlineRow
          key={`${d.kind}-${d.vehicle_id}-${d.ref_id}`}
          d={d}
          vehicle={d.vehicle_id ? byId.get(d.vehicle_id) : undefined}
          showVehicle={filter === null}
          onClick={() => actions.run(d, false)}
        />
      ))}
    </div>
  )

  return (
    <div className="page">
      <div className="page-head">
        <img className="page-brand" src="/logo-m.png" alt="" width="34" height="32" />
        <div className="page-title">
          <div className="page-kicker">{today.charAt(0).toUpperCase() + today.slice(1)}</div>
          <h1>{t('nav.deadlines')}</h1>
        </div>
        <button className="btn-icon btn-round" onClick={() => setNewReminder(true)} aria-label={t('reminder.new')} title={t('reminder.new')}>
          <Icon name="bell" />
        </button>
      </div>

      {active.length === 0 && (
        <div className="hero">
          <Icon name="car" size={40} />
          <p>{t('vehicles.empty')}</p>
          <Link className="btn btn-cta btn-lg" to="/vehicles/new">
            <Icon name="plus" size={18} />
            {t('vehicles.first')}
          </Link>
        </div>
      )}

      {active.length > 1 && (
        <div className="chips scroll-x" role="group" aria-label={t('nav.vehicles')} style={{ marginBottom: 18 }}>
          <button className="chip" aria-pressed={filter === null} onClick={() => setFilter(null)}>
            {t('home.all')}
          </button>
          {active.map((v) => (
            <button key={v.id} className="chip" aria-pressed={filter === v.id} onClick={() => setFilter(v.id)}>
              {v.name}
            </button>
          ))}
        </div>
      )}

      {!next ? (
        active.length > 0 && <Empty>{t('deadline.none')}</Empty>
      ) : (
        <section className="next" aria-label={t('home.next')}>
          <div className="next-top">
            <span className="next-kicker">{next.status === 'overdue' || next.status === 'grace' ? t('status.overdue') : t('home.next')}</span>
            <span className={`pill pill-${next.status}`}>
              {next.status === 'suspended' ? t('status.suspended') : next.days_left !== undefined ? relativeDays(next.days_left, t) : t(`status.${next.status}`)}
            </span>
          </div>
          <div className="next-body">
            <div className="next-title">{deadlineTitle(next, t)}</div>
            <div className="next-sub">{[next.vehicle_name ?? t('home.personal'), deadlineDetail(next, t, locale)].filter(Boolean).join(' · ')}</div>
          </div>
          <div className="next-actions">
            {actions.canEdit(next) && (
              <button className="btn btn-cta btn-lg" onClick={() => actions.run(next, true)}>
                {actionLabel(next, t)}
              </button>
            )}
            {next.vehicle_id && (
              <Link className="btn btn-ghost btn-lg" to={`/vehicles/${next.vehicle_id}?tab=${deadlineTab(next)}`} aria-label={t('action.open')}>
                <Icon name="next" />
              </Link>
            )}
          </div>
        </section>
      )}

      {soon.length > 0 && (
        <>
          <h2 className="section-title">{t('home.soon')}</h2>
          {list(soon)}
        </>
      )}
      {later.length > 0 && (
        <>
          <h2 className="section-title">{t('home.later')}</h2>
          {list(later)}
        </>
      )}

      {actions.element}
      {newReminder && (
        <ReminderForm
          vehicles={vehicles.data}
          vehicleId={filter ?? undefined}
          onClose={() => setNewReminder(false)}
          onSaved={() => {
            setNewReminder(false)
            notifyChanged()
          }}
        />
      )}
    </div>
  )
}
