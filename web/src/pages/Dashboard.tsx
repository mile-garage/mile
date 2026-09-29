// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { api, type Deadline, type Reminder, type Vehicle } from '../api'
import { PageHead } from '../App'
import { useI18n } from '../i18n'
import { date } from '../format'
import { DeadlineRow } from '../components/Deadlines'
import { ReminderForm } from '../components/Forms'
import { Icon } from '../components/Icon'
import { Empty, Loading, useLoad, useUI } from '../components/ui'

export const tabFor: Record<string, string> = {
  inspection: 'deadlines',
  road_tax: 'deadlines',
  service: 'deadlines',
  oil_change: 'deadlines',
  transmission_oil: 'deadlines',
  insurance: 'insurance',
  tyre_change: 'tyres',
  tyre_rotation: 'tyres',
}

export function Dashboard() {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const navigate = useNavigate()
  const deadlines = useLoad<Deadline[]>('/api/deadlines')
  const vehicles = useLoad<Vehicle[]>('/api/vehicles')
  const [editing, setEditing] = useState<Reminder | 'new' | null>(null)

  const openReminder = async (id: number) => {
    try {
      const all = await api.get<Reminder[]>('/api/reminders')
      const r = all.find((x) => x.id === id)
      if (r) setEditing(r)
    } catch (e) {
      fail(e)
    }
  }

  const done = async (d: Deadline) => {
    try {
      const r = await api.post<Reminder>(`/api/reminders/${d.ref_id}/done`)
      toast(r.done_at ? t('common.saved') : t('reminder.doneRepeat', { date: date(r.due_date, locale) }))
      deadlines.reload()
    } catch (e) {
      fail(e)
    }
  }

  if (!deadlines.data || !vehicles.data) return <Loading />
  const hasVehicles = vehicles.data.some((v) => !v.archived)

  return (
    <div className="page">
      <PageHead title={t('nav.deadlines')}>
        <button className="btn" onClick={() => setEditing('new')}>
          <Icon name="bell" size={16} />
          {t('deadline.addReminder')}
        </button>
      </PageHead>

      {!hasVehicles && (
        <div className="hero">
          <Icon name="car" size={40} />
          <p>{t('vehicles.empty')}</p>
          <Link className="btn btn-primary" to="/vehicles/new">
            <Icon name="plus" size={16} />
            {t('vehicles.first')}
          </Link>
        </div>
      )}

      {deadlines.data.length === 0 ? (
        hasVehicles && <Empty>{t('deadline.none')}</Empty>
      ) : (
        <div className="card list">
          {deadlines.data.map((d) => (
            <DeadlineRow
              key={`${d.kind}-${d.vehicle_id}-${d.ref_id}`}
              d={d}
              showVehicle
              onClick={() =>
                d.kind === 'reminder' ? openReminder(d.ref_id!) : navigate(`/vehicles/${d.vehicle_id}?tab=${tabFor[d.kind]}`)
              }
              actions={
                d.kind === 'reminder' && (
                  <button className="btn btn-sm" onClick={() => done(d)}>
                    <Icon name="check" size={14} />
                    {t('deadline.done')}
                  </button>
                )
              }
            />
          ))}
        </div>
      )}

      {editing && (
        <ReminderForm
          vehicles={vehicles.data}
          reminder={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            deadlines.reload()
          }}
        />
      )}
    </div>
  )
}
