// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from 'react'
import { api, twoWheels, type TyreEvent, type TyreSet, type Tyres } from '../api'
import { useI18n, type T } from '../i18n'
import { date, num } from '../format'
import { TyreEventForm, TyreSetForm, tyreLabel } from '../components/Forms'
import { Icon } from '../components/Icon'
import { Empty, Loading, StatusPill, useLoad, useUI } from '../components/ui'
import type { TabProps } from './VehicleTabs'

/** Age of a tyre from its DOT date code (week and year, "2322"). */
function dotAge(dot: string, t: T): string | null {
  const m = /^(\d{2})(\d{2})$/.exec(dot)
  if (!m || Number(m[1]) < 1 || Number(m[1]) > 53) return null
  const made = Date.UTC(2000 + Number(m[2]), 0, 1 + (Number(m[1]) - 1) * 7)
  const years = Math.floor((Date.now() - made) / (365.25 * 24 * 3600 * 1000))
  if (years < 0) return null
  return years === 0 ? t('tyres.age0') : years === 1 ? t('tyres.age1') : t('tyres.age', { n: years })
}

export function TyresTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const { confirm, fail } = useUI()
  const { data } = useLoad<Tyres>(`/api/vehicles/${vehicle.id}/tyres`, version)
  const [editing, setEditing] = useState<TyreSet | 'new' | null>(null)
  const [event, setEvent] = useState<{ kind: 'mount' | 'rotate'; setId: number } | null>(null)
  if (!data) return <Loading />
  const rotates = !twoWheels(vehicle.kind)
  const mounted = data.sets.find((s) => s.mounted)
  const byId = new Map(data.sets.map((s) => [s.id, s]))

  const delEvent = async (e: TyreEvent) => {
    if (!(await confirm({ title: t('common.delete'), message: t('tyres.deleteEventText'), confirmLabel: t('common.delete'), danger: true }))) return
    try {
      await api.del(`/api/tyre-events/${e.id}`)
      changed()
    } catch (err) {
      fail(err)
    }
  }

  const eventText = (e: TyreEvent) => {
    const s = byId.get(e.set_id)
    return t(e.kind === 'mount' ? 'tyres.event.mount' : 'tyres.event.rotate', { set: s ? tyreLabel(s, t) : '' })
  }

  return (
    <>
      <div className="tab-head">
        <span className="muted">{data.sets.length > 0 && !mounted && t('tyres.noneMounted')}</span>
        {canEdit && (
          <button className="btn btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t('tyres.new')}
          </button>
        )}
      </div>
      {data.sets.length === 0 && <Empty>{t('tyres.empty')}</Empty>}
      {data.sets.map((s) => {
        const age = dotAge(s.dot, t)
        return (
          <div key={s.id} className={`card policy${s.retired ? ' retired' : ''}`}>
            <div className="policy-head">
              <div>
                <h3>{[s.brand, s.model].filter(Boolean).join(' ') || t(`tyres.season.${s.season}`)}</h3>
                {s.size && <div className="muted">{s.size}</div>}
              </div>
              {s.mounted ? (
                <StatusPill status="ok" label={t('tyres.mounted')} />
              ) : s.retired ? (
                <StatusPill status="retired" label={t('tyres.retiredShort')} />
              ) : (
                <StatusPill status="suspended" label={t('tyres.stored')} />
              )}
            </div>
            <dl className="facts">
              <dt>{t('tyres.season')}</dt>
              <dd>{t(`tyres.season.${s.season}`)}</dd>
              {s.dot && (
                <>
                  <dt>{t('tyres.dot')}</dt>
                  <dd>
                    {s.dot}
                    {age && <span className="muted"> · {age}</span>}
                  </dd>
                </>
              )}
              {s.mounted && s.mounted_since && (
                <>
                  <dt>{t('tyres.mountedSince')}</dt>
                  <dd>{date(s.mounted_since, locale)}</dd>
                </>
              )}
              <dt>{t('tyres.km')}</dt>
              <dd>{num(s.km, locale)} km</dd>
              {rotates && s.km > 0 && (
                <>
                  <dt>{t('tyres.kmSinceRotation')}</dt>
                  <dd>
                    {num(s.km_since_rotation, locale)} km
                    {s.last_rotation && <div className="muted small">{t('tyres.lastRotation', { date: date(s.last_rotation, locale) })}</div>}
                  </dd>
                </>
              )}
              {!s.mounted && s.storage && (
                <>
                  <dt>{t('tyres.storage')}</dt>
                  <dd>{s.storage}</dd>
                </>
              )}
            </dl>
            {s.notes && <p className="notes">{s.notes}</p>}
            {canEdit && (
              <div className="row-actions">
                {!s.mounted && !s.retired && (
                  <button className="btn btn-primary" onClick={() => setEvent({ kind: 'mount', setId: s.id })}>
                    <Icon name="swap" size={16} />
                    {t('tyres.mount')}
                  </button>
                )}
                {s.mounted && rotates && (
                  <button className="btn" onClick={() => setEvent({ kind: 'rotate', setId: s.id })}>
                    <Icon name="rotate" size={16} />
                    {t('tyres.rotate')}
                  </button>
                )}
                <button className="btn" onClick={() => setEditing(s)}>
                  <Icon name="edit" size={16} />
                  {t('common.edit')}
                </button>
              </div>
            )}
          </div>
        )
      })}

      {data.events.length > 0 && (
        <section className="card chart-card">
          <h3>{t('tyres.history')}</h3>
          <ul className="tyre-history">
            {data.events.map((e) => (
              <li key={e.id}>
                <span className="tyre-event">
                  {eventText(e)}
                  <small>
                    {date(e.date, locale)} · {num(e.odometer, locale)} km
                    {e.notes && ` · ${e.notes}`}
                  </small>
                </span>
                {canEdit && (
                  <button className="btn-icon" onClick={() => delEvent(e)} aria-label={t('common.delete')}>
                    <Icon name="trash" size={14} />
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}

      {editing && (
        <TyreSetForm
          vehicle={vehicle}
          set={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            changed()
          }}
        />
      )}
      {event && (
        <TyreEventForm
          vehicle={vehicle}
          kind={event.kind}
          sets={data.sets}
          setId={event.setId}
          onClose={() => setEvent(null)}
          onSaved={() => {
            setEvent(null)
            changed()
          }}
        />
      )}
    </>
  )
}
