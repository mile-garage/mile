// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from 'react'
import { Link } from 'react-router'
import {
  api,
  fileUrl,
  type Attachment,
  type Deadline,
  type Expense,
  type OdometerReading,
  type Policy,
  type PolicyInput,
  type Refuel,
  type Stats,
  type Vehicle,
} from '../api'
import { useI18n } from '../i18n'
import { addMonths, date, euro, month, num, today, unit } from '../format'
import { DeadlineRow, useDeadlineActions } from '../components/Deadlines'
import { FileList, UploadButton } from '../components/Files'
import { DateForm, ExpenseForm, PolicyForm, RefuelForm } from '../components/Forms'
import { Icon } from '../components/Icon'
import { Empty, Loading, StatusPill, useLoad, useUI } from '../components/ui'

export interface TabProps {
  vehicle: Vehicle
  canEdit: boolean
  version: number
  changed: () => void
}

// ---- status: deadlines and the numbers that matter ----

export function StatusTab({ vehicle, canEdit, version }: TabProps) {
  const { t, locale } = useI18n()
  const deadlines = useLoad<Deadline[]>(`/api/vehicles/${vehicle.id}/deadlines`, version)
  const stats = useLoad<Stats>(`/api/vehicles/${vehicle.id}/stats`, version)
  const actions = useDeadlineActions([vehicle])
  if (!deadlines.data || !stats.data) return <Loading />
  const u = unit(vehicle.fuel_type)
  const f = stats.data.fuel

  return (
    <>
      {deadlines.data.length === 0 ? (
        <Empty>{t('deadline.noneVehicle')}</Empty>
      ) : (
        <div className="card list">
          {deadlines.data.map((d) => (
            <DeadlineRow
              key={`${d.kind}-${d.ref_id}`}
              d={d}
              vehicle={vehicle}
              full
              onClick={() => actions.run(d, canEdit && d.kind !== 'reminder')}
              actions={actions.rowActions(d)}
            />
          ))}
        </div>
      )}

      <h2 className="section-title">{t('vehicle.stats')}</h2>
      <div className="tiles">
        <div className="tile">
          <span className="tile-label">{t('fuel.avg')}</span>
          <span className="tile-value">
            {f.avg_per_100km !== undefined ? num(f.avg_per_100km, locale, 1) : '—'} <span className="tile-unit">{u}/100 km</span>
          </span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('summary.costKm')}</span>
          <span className="tile-value">
            {stats.data.cents_per_km !== null ? num(stats.data.cents_per_km / 100, locale, 2) : '—'} <span className="tile-unit">€</span>
          </span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('summary.total')}</span>
          <span className="tile-value">{euro(stats.data.total, locale)}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('fuel.avgPrice')}</span>
          <span className="tile-value">
            {f.avg_unit_price !== undefined ? num(f.avg_unit_price, locale, 3) : '—'} <span className="tile-unit">€/{u}</span>
          </span>
        </div>
      </div>
      {f.refuels > 1 && f.avg_per_100km === undefined && <p className="muted">{t('fuel.needFull')}</p>}
      <Link className="btn btn-block" to={`/expenses?vehicle=${vehicle.id}`}>
        <Icon name="chart" size={18} />
        {t('summary.last12')}
      </Link>
      {actions.element}
    </>
  )
}

// ---- log: refuels, expenses and km readings in one list ----

type LogItem =
  | { kind: 'refuel'; date: string; id: number; cents: number; r: Refuel }
  | { kind: 'expense'; date: string; id: number; cents: number; e: Expense }
  | { kind: 'km'; date: string; id: number; cents: number; o: OdometerReading }

type LogFilter = 'all' | 'refuel' | 'expense' | 'km'

export function LogTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const { confirm, fail } = useUI()
  const expenses = useLoad<Expense[]>(`/api/vehicles/${vehicle.id}/expenses`, version)
  const refuels = useLoad<Refuel[]>(`/api/vehicles/${vehicle.id}/refuels`, version)
  const readings = useLoad<OdometerReading[]>(`/api/vehicles/${vehicle.id}/odometer`, version)
  const [filter, setFilter] = useState<LogFilter>('all')
  const [editing, setEditing] = useState<LogItem | null>(null)
  if (!expenses.data || !refuels.data || !readings.data) return <Loading />
  const u = unit(vehicle.fuel_type)

  const items: LogItem[] = [
    ...refuels.data.map((r) => ({ kind: 'refuel' as const, date: r.date, id: r.id, cents: r.total_cents, r })),
    ...expenses.data.map((e) => ({ kind: 'expense' as const, date: e.date, id: e.id, cents: e.amount_cents, e })),
    ...readings.data.map((o) => ({ kind: 'km' as const, date: o.date, id: o.id, cents: 0, o })),
  ]
    .filter((x) => filter === 'all' || x.kind === filter)
    .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1))

  const months: { key: string; items: LogItem[] }[] = []
  for (const x of items) {
    const key = x.date.slice(0, 7)
    if (months.at(-1)?.key !== key) months.push({ key, items: [] })
    months.at(-1)!.items.push(x)
  }

  const open = async (x: LogItem) => {
    if (!canEdit) return
    if (x.kind !== 'km') return setEditing(x)
    if (!(await confirm({ title: t('common.delete'), message: `${num(x.o.km, locale)} km · ${date(x.date, locale)}`, confirmLabel: t('common.delete'), danger: true })))
      return
    try {
      await api.del(`/api/odometer/${x.id}`)
      changed()
    } catch (e) {
      fail(e)
    }
  }

  const filters: { key: LogFilter; label: string }[] = [
    { key: 'all', label: t('log.all') },
    { key: 'refuel', label: t('cat.fuel') },
    { key: 'expense', label: t('tab.expenses') },
    { key: 'km', label: t('common.km') },
  ]

  return (
    <>
      <div className="tab-head">
        <div className="chips scroll-x" role="group" aria-label={t('tab.log')}>
          {filters.map((f) => (
            <button key={f.key} className="chip" aria-pressed={filter === f.key} onClick={() => setFilter(f.key)}>
              {f.label}
            </button>
          ))}
        </div>
      </div>
      {months.length === 0 && <Empty>{t('log.empty')}</Empty>}
      {months.map((m) => {
        const total = m.items.reduce((s, x) => s + x.cents, 0)
        return (
          <section key={m.key}>
            <h2 className="section-title" style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>{month(m.key, locale)}</span>
              {total > 0 && <span className="num">{euro(total, locale)}</span>}
            </h2>
            <div className="card list">
              {m.items.map((x) => (
                <LogRow key={`${x.kind}-${x.id}`} x={x} unit={u} canEdit={canEdit} onClick={() => open(x)} />
              ))}
            </div>
          </section>
        )
      })}
      {editing?.kind === 'refuel' && (
        <RefuelForm
          vehicle={vehicle}
          refuel={editing.r}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            changed()
          }}
        />
      )}
      {editing?.kind === 'expense' && (
        <ExpenseForm
          vehicle={vehicle}
          expense={editing.e}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            changed()
          }}
        />
      )}
    </>
  )
}

function LogRow({ x, unit: u, canEdit, onClick }: { x: LogItem; unit: string; canEdit: boolean; onClick: () => void }) {
  const { t, locale } = useI18n()
  let icon = 'gauge'
  let title = ''
  const sub: string[] = [date(x.date, locale)]
  let files: Attachment[] = []
  if (x.kind === 'refuel') {
    icon = 'fuel'
    title = t('add.refuel')
    sub.push(`${num(x.r.quantity, locale, 2)} ${u}`, `${num(x.r.odometer, locale)} km`)
    if (!x.r.full_tank) sub.push(t('refuel.partial'))
    files = x.r.attachments
  } else if (x.kind === 'expense') {
    icon = ['road_tax', 'tolls', 'parking', 'fine', 'wash', 'accessories', 'other'].includes(x.e.category) ? 'receipt' : 'wrench'
    title = t(`cat.${x.e.category}`) + (x.e.description ? ` · ${x.e.description}` : '')
    if (x.e.odometer !== null) sub.push(`${num(x.e.odometer, locale)} km`)
    if (x.e.vendor) sub.push(x.e.vendor)
    if (x.e.valid_until) sub.push(`→ ${month(x.e.valid_until, locale)}`)
    files = x.e.attachments
  } else {
    title = `${num(x.o.km, locale)} km`
  }
  return (
    <div className={`row${canEdit ? ' clickable' : ''}`} onClick={onClick}>
      <span className="row-icon">
        <Icon name={icon} />
      </span>
      <div className="row-main">
        <div className="row-title">{title}</div>
        <div className="row-sub">{sub.join(' · ')}</div>
        {!canEdit && <FileList files={files} canEdit={false} onDeleted={() => {}} />}
      </div>
      <div className="row-side">
        {x.cents > 0 && <span className="row-amount">{euro(x.cents, locale)}</span>}
        {files.length > 0 && (
          <span className="muted small">
            <Icon name="paperclip" size={14} /> {files.length}
          </span>
        )}
      </div>
    </div>
  )
}

// ---- documents: insurance and photos ----

export function DocsTab(props: TabProps) {
  const { t } = useI18n()
  return (
    <>
      <h2 className="section-title">{t('tab.insurance')}</h2>
      <InsuranceTab {...props} />
      <h2 className="section-title">{t('tab.photos')}</h2>
      <PhotosTab {...props} />
    </>
  )
}

// ---- insurance ----

export function InsuranceTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const { confirm, fail } = useUI()
  const { data, setData } = useLoad<Policy[]>(`/api/vehicles/${vehicle.id}/policies`, version)
  const [editing, setEditing] = useState<{ policy?: Policy; preset?: Partial<PolicyInput> } | null>(null)
  const [dateAction, setDateAction] = useState<{ policy: Policy; resume: boolean } | null>(null)
  if (!data) return <Loading />

  const replace = (p: Policy) => setData(data.map((x) => (x.id === p.id ? p : x)))
  const renew = (p: Policy) =>
    setEditing({
      preset: {
        insurer: p.insurer,
        policy_number: p.policy_number,
        start_date: p.effective_end,
        end_date: addMonths(p.effective_end, 12),
        premium_cents: p.premium_cents,
      },
    })

  const delSuspension = async (p: Policy, id: number) => {
    if (!(await confirm({ title: t('policy.suspensions'), message: t('policy.deleteSuspension'), confirmLabel: t('common.delete'), danger: true })))
      return
    try {
      replace(await api.del<Policy>(`/api/policies/${p.id}/suspensions/${id}`))
      changed()
    } catch (e) {
      fail(e)
    }
  }

  return (
    <>
      <div className="tab-head">
        <span />
        {canEdit && (
          <button className="btn" onClick={() => setEditing({})}>
            <Icon name="plus" size={16} />
            {t('policy.new')}
          </button>
        )}
      </div>
      {data.length === 0 && <Empty>{t('policy.empty')}</Empty>}
      {data.map((p, i) => {
        const expired = p.effective_end < today() && !p.suspended
        const status = p.suspended ? 'suspended' : expired ? 'overdue' : 'ok'
        return (
          <div key={p.id} className="card policy">
            <div className="policy-head">
              <div>
                <h3>{p.insurer || t('deadline.insurance')}</h3>
                {p.policy_number && <div className="muted">{p.policy_number}</div>}
              </div>
              <StatusPill status={status} label={p.suspended ? t('status.suspended') : expired ? t('policy.expired') : t('policy.current')} />
            </div>
            <dl className="facts">
              <dt>{t('policy.start')}</dt>
              <dd>{date(p.start_date, locale)}</dd>
              <dt>{t('policy.end')}</dt>
              <dd>{date(p.end_date, locale)}</dd>
              {p.extension_days > 0 && (
                <>
                  <dt>{t('policy.effectiveEnd')}</dt>
                  <dd>
                    <strong>{date(p.effective_end, locale)}</strong>
                    <div className="muted small">{t('policy.extension', { days: p.extension_days })}</div>
                  </dd>
                </>
              )}
              {p.premium_cents !== null && (
                <>
                  <dt>{t('policy.premium')}</dt>
                  <dd>{euro(p.premium_cents, locale)}</dd>
                </>
              )}
            </dl>
            {p.suspensions.length > 0 && (
              <div className="suspensions">
                <div className="field-label">{t('policy.suspensions')}</div>
                <ul>
                  {p.suspensions.map((s) => (
                    <li key={s.id}>
                      {s.end_date
                        ? t('policy.suspensionRange', { from: date(s.start_date, locale), to: date(s.end_date, locale) })
                        : t('policy.suspensionOpen', { from: date(s.start_date, locale) })}
                      {canEdit && (
                        <button className="btn-icon" onClick={() => delSuspension(p, s.id)} aria-label={t('common.delete')}>
                          <Icon name="trash" size={14} />
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {p.notes && <p className="notes">{p.notes}</p>}
            <FileList files={p.attachments} canEdit={canEdit} onDeleted={(id) => replace({ ...p, attachments: p.attachments.filter((a) => a.id !== id) })} />
            {canEdit && (
              <div className="row-actions">
                {(i === 0 || p.suspended) &&
                  (p.suspended ? (
                    <button className="btn btn-primary" onClick={() => setDateAction({ policy: p, resume: true })}>
                      <Icon name="play" size={16} />
                      {t('policy.resume')}
                    </button>
                  ) : (
                    !expired && (
                      <button className="btn" onClick={() => setDateAction({ policy: p, resume: false })}>
                        <Icon name="pause" size={16} />
                        {t('policy.suspend')}
                      </button>
                    )
                  ))}
                {i === 0 && (
                  <button className="btn" onClick={() => renew(p)}>
                    {t('policy.renew')}
                  </button>
                )}
                <button className="btn" onClick={() => setEditing({ policy: p })}>
                  <Icon name="edit" size={16} />
                  {t('common.edit')}
                </button>
                <UploadButton
                  target={{ vehicleId: vehicle.id, kind: 'document', policy_id: p.id }}
                  onUploaded={(a: Attachment) => replace({ ...p, attachments: [...p.attachments, a] })}
                  className="btn"
                />
              </div>
            )}
          </div>
        )
      })}
      {editing && (
        <PolicyForm
          vehicle={vehicle}
          policy={editing.policy}
          preset={editing.preset}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            changed()
          }}
        />
      )}
      {dateAction && (
        <DateForm
          title={dateAction.resume ? t('policy.resumeTitle') : t('policy.suspendTitle')}
          message={dateAction.resume ? undefined : t('policy.suspendText')}
          submitLabel={dateAction.resume ? t('policy.resume') : t('policy.suspend')}
          onClose={() => setDateAction(null)}
          onSubmit={async (d) => {
            const p = await api.post<Policy>(`/api/policies/${dateAction.policy.id}/${dateAction.resume ? 'resume' : 'suspend'}`, { date: d })
            replace(p)
            setDateAction(null)
            changed()
          }}
        />
      )}
    </>
  )
}

// ---- photos ----

export function PhotosTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t } = useI18n()
  const { confirm, fail } = useUI()
  const { data, setData } = useLoad<Attachment[]>(`/api/vehicles/${vehicle.id}/photos`, version)
  if (!data) return <Loading />

  const setCover = async (id: number) => {
    try {
      await api.put(`/api/vehicles/${vehicle.id}/cover`, { attachment_id: id })
      changed()
    } catch (e) {
      fail(e)
    }
  }
  const del = async (a: Attachment) => {
    if (!(await confirm({ title: t('common.delete'), message: t('photos.deleteText'), confirmLabel: t('common.delete'), danger: true }))) return
    try {
      await api.del(`/api/attachments/${a.id}`)
      setData(data.filter((x) => x.id !== a.id))
      if (vehicle.cover_id === a.id) changed()
    } catch (e) {
      fail(e)
    }
  }

  return (
    <>
      {canEdit && (
        <div className="tab-head">
          <span />
          <UploadButton
            target={{ vehicleId: vehicle.id, kind: 'photo' }}
            multiple
            label={t('photos.add')}
            className="btn"
            onUploaded={(a) => {
              setData((d) => [a, ...(d ?? [])])
              if (vehicle.cover_id === null) setCover(a.id)
            }}
          />
        </div>
      )}
      {data.length === 0 ? (
        <Empty>{t('photos.empty')}</Empty>
      ) : (
        <div className="photo-grid">
          {data.map((a) => (
            <figure key={a.id} className="photo">
              <a href={fileUrl(a.id)} target="_blank" rel="noopener">
                <img src={fileUrl(a.id)} alt="" loading="lazy" />
              </a>
              {vehicle.cover_id === a.id && <span className="badge photo-badge">{t('photos.isCover')}</span>}
              {canEdit && (
                <figcaption>
                  {vehicle.cover_id !== a.id && (
                    <button className="btn btn-sm" onClick={() => setCover(a.id)}>
                      <Icon name="star" size={14} />
                      {t('photos.cover')}
                    </button>
                  )}
                  <button className="btn-icon" onClick={() => del(a)} aria-label={t('common.delete')}>
                    <Icon name="trash" size={16} />
                  </button>
                </figcaption>
              )}
            </figure>
          ))}
        </div>
      )}
    </>
  )
}
