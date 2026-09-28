// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from 'react'
import {
  api,
  fileUrl,
  type Attachment,
  type Deadline,
  type Expense,
  type ExpenseInput,
  type OdometerReading,
  type Policy,
  type PolicyInput,
  type Refuel,
  type Reminder,
  type Stats,
  type Vehicle,
} from '../api'
import { useI18n } from '../i18n'
import { addMonths, date, euro, month, num, today, unit } from '../format'
import { DeadlineRow } from '../components/Deadlines'
import { FileList, UploadButton } from '../components/Files'
import { DateForm, ExpenseForm, PolicyForm, RefuelForm, ReminderForm } from '../components/Forms'
import { Icon } from '../components/Icon'
import { Empty, Loading, StatusPill, useLoad, useUI } from '../components/ui'

interface TabProps {
  vehicle: Vehicle
  canEdit: boolean
  version: number
  changed: () => void
}

// ---- deadlines ----

type ExpensePreset = Partial<ExpenseInput>

export function DeadlinesTab({ vehicle, canEdit, version, changed, goTo }: TabProps & { goTo: (tab: string) => void }) {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const { data } = useLoad<Deadline[]>(`/api/vehicles/${vehicle.id}/deadlines`, version)
  const [expense, setExpense] = useState<ExpensePreset | null>(null)
  const [reminder, setReminder] = useState<Reminder | 'new' | null>(null)
  if (!data) return <Loading />

  const km = vehicle.current_km
  const roadTax = data.find((d) => d.kind === 'road_tax')
  const presets: Record<string, ExpensePreset> = {
    inspection: { category: 'inspection', odometer: km },
    service: { category: 'service', odometer: km },
    oil_change: { category: 'oil_change', odometer: km },
    // a payment made for the deadline covers the 12 months after the expired one
    road_tax: { category: 'road_tax', valid_until: roadTax?.due ? addMonths(roadTax.due, 11).slice(0, 7) : null },
  }

  const open = async (d: Deadline) => {
    if (d.kind === 'insurance') return goTo('insurance')
    if (d.kind === 'reminder') {
      try {
        const rs = await api.get<Reminder[]>(`/api/reminders?vehicle=${vehicle.id}`)
        const r = rs.find((x) => x.id === d.ref_id)
        if (r && canEdit) setReminder(r)
      } catch (e) {
        fail(e)
      }
      return
    }
    if (canEdit) setExpense(presets[d.kind])
  }

  const done = async (d: Deadline) => {
    try {
      const r = await api.post<Reminder>(`/api/reminders/${d.ref_id}/done`)
      toast(r.done_at ? t('common.saved') : t('reminder.doneRepeat', { date: date(r.due_date, locale) }))
      changed()
    } catch (e) {
      fail(e)
    }
  }

  return (
    <>
      {canEdit && (
        <div className="quick-actions">
          <button className="btn" onClick={() => setExpense(presets.inspection)}>
            <Icon name="check" size={16} />
            {t('quick.inspection')}
          </button>
          <button className="btn" onClick={() => setExpense(presets.road_tax)}>
            <Icon name="check" size={16} />
            {t('quick.roadTax')}
          </button>
          <button className="btn" onClick={() => setExpense(presets.service)}>
            <Icon name="check" size={16} />
            {t('quick.service')}
          </button>
          <button className="btn" onClick={() => setExpense(presets.oil_change)}>
            <Icon name="check" size={16} />
            {t('quick.oilChange')}
          </button>
          <button className="btn" onClick={() => setReminder('new')}>
            <Icon name="bell" size={16} />
            {t('deadline.addReminder')}
          </button>
        </div>
      )}
      {data.length === 0 ? (
        <Empty>{t('deadline.noneVehicle')}</Empty>
      ) : (
        <div className="card list">
          {data.map((d) => (
            <DeadlineRow
              key={`${d.kind}-${d.ref_id}`}
              d={d}
              onClick={() => open(d)}
              actions={
                d.kind === 'reminder' &&
                canEdit && (
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
      {expense && (
        <ExpenseForm
          vehicle={vehicle}
          preset={expense}
          onClose={() => setExpense(null)}
          onSaved={() => {
            setExpense(null)
            changed()
          }}
        />
      )}
      {reminder && (
        <ReminderForm
          vehicles={[vehicle]}
          vehicleId={vehicle.id}
          reminder={reminder === 'new' ? undefined : reminder}
          onClose={() => setReminder(null)}
          onSaved={() => {
            setReminder(null)
            changed()
          }}
        />
      )}
    </>
  )
}

// ---- expenses ----

export function ExpensesTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const { data } = useLoad<Expense[]>(`/api/vehicles/${vehicle.id}/expenses`, version)
  const [editing, setEditing] = useState<Expense | 'new' | null>(null)
  if (!data) return <Loading />
  const total = data.reduce((s, e) => s + e.amount_cents, 0)

  return (
    <>
      <div className="tab-head">
        <span className="muted">
          {data.length > 0 && (
            <>
              {t('common.total')}: <strong>{euro(total, locale)}</strong>
            </>
          )}
        </span>
        {canEdit && (
          <button className="btn btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t('expense.new')}
          </button>
        )}
      </div>
      {data.length === 0 ? (
        <Empty>{t('expense.empty')}</Empty>
      ) : (
        <div className="card list">
          {data.map((e) => (
            <div key={e.id} className={`row${canEdit ? ' clickable' : ''}`} onClick={() => canEdit && setEditing(e)}>
              <div className="row-main">
                <div className="row-title">
                  {t(`cat.${e.category}`)}
                  {e.description && <span className="muted"> · {e.description}</span>}
                </div>
                <div className="row-sub">
                  {date(e.date, locale)}
                  {e.odometer !== null && ` · ${num(e.odometer, locale)} km`}
                  {e.vendor && ` · ${e.vendor}`}
                  {e.valid_until && ` · → ${month(e.valid_until, locale)}`}
                </div>
                {!canEdit && <FileList files={e.attachments} canEdit={false} onDeleted={() => {}} />}
              </div>
              <div className="row-side">
                <strong>{euro(e.amount_cents, locale)}</strong>
                {e.attachments.length > 0 && (
                  <span className="muted small">
                    <Icon name="paperclip" size={14} /> {e.attachments.length}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
      {editing && (
        <ExpenseForm
          vehicle={vehicle}
          expense={editing === 'new' ? undefined : editing}
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

// ---- fuel ----

export function FuelTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const refuels = useLoad<Refuel[]>(`/api/vehicles/${vehicle.id}/refuels`, version)
  const stats = useLoad<Stats>(`/api/vehicles/${vehicle.id}/stats`, version)
  const [editing, setEditing] = useState<Refuel | 'new' | null>(null)
  if (!refuels.data || !stats.data) return <Loading />
  const u = unit(vehicle.fuel_type)
  const f = stats.data.fuel

  const consumption = (per100?: number) => {
    if (per100 === undefined) return '—'
    const s = `${num(per100, locale, 1)} ${u}/100 km`
    return u === 'l' ? `${s} · ${num(100 / per100, locale, 1)} km/l` : s
  }

  return (
    <>
      <div className="tiles">
        <div className="tile">
          <span className="tile-label">{t('fuel.avg')}</span>
          <span className="tile-value">{consumption(f.avg_per_100km)}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('fuel.last')}</span>
          <span className="tile-value">{consumption(f.last_per_100km)}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('fuel.costKm')}</span>
          <span className="tile-value">{f.cents_per_km !== undefined ? `${num(f.cents_per_km / 100, locale, 3)} €` : '—'}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('fuel.avgPrice')}</span>
          <span className="tile-value">{f.avg_unit_price !== undefined ? `${num(f.avg_unit_price, locale, 3)} €/${u}` : '—'}</span>
        </div>
      </div>
      {f.refuels > 1 && f.avg_per_100km === undefined && <p className="muted">{t('fuel.needFull')}</p>}

      <div className="tab-head">
        <span />
        {canEdit && (
          <button className="btn btn-primary" onClick={() => setEditing('new')}>
            <Icon name="fuel" size={16} />
            {t('refuel.new')}
          </button>
        )}
      </div>
      {refuels.data.length === 0 ? (
        <Empty>{t('refuel.empty')}</Empty>
      ) : (
        <div className="card list">
          {refuels.data.map((r) => (
            <div key={r.id} className={`row${canEdit ? ' clickable' : ''}`} onClick={() => canEdit && setEditing(r)}>
              <div className="row-main">
                <div className="row-title">
                  {num(r.quantity, locale, 2)} {u}
                  {r.unit_price !== null && <span className="muted"> · {num(r.unit_price, locale, 3)} €/{u}</span>}
                  {!r.full_tank && <span className="badge">{t('refuel.partial')}</span>}
                </div>
                <div className="row-sub">
                  {date(r.date, locale)} · {num(r.odometer, locale)} km{r.station && ` · ${r.station}`}
                </div>
              </div>
              <div className="row-side">
                <strong>{euro(r.total_cents, locale)}</strong>
                {r.attachments.length > 0 && (
                  <span className="muted small">
                    <Icon name="paperclip" size={14} /> {r.attachments.length}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
      {editing && (
        <RefuelForm
          vehicle={vehicle}
          refuel={editing === 'new' ? undefined : editing}
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
          <button className="btn btn-primary" onClick={() => setEditing({})}>
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
            className="btn btn-primary"
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

// ---- summary ----

const series = ['fuel', 'expenses', 'insurance'] as const

export function SummaryTab({ vehicle, canEdit, version, changed }: TabProps) {
  const { t, locale } = useI18n()
  const { confirm, fail } = useUI()
  const stats = useLoad<Stats>(`/api/vehicles/${vehicle.id}/stats`, version)
  const readings = useLoad<OdometerReading[]>(`/api/vehicles/${vehicle.id}/odometer`, version)
  if (!stats.data || !readings.data) return <Loading />
  const s = stats.data
  const cats = Object.entries(s.by_category).sort((a, b) => b[1] - a[1])
  const maxCat = Math.max(1, ...cats.map((c) => c[1]))
  const monthTotal = (m: (typeof s.last_12_months)[number]) => m.fuel + m.expenses + m.insurance
  const maxMonth = Math.max(1, ...s.last_12_months.map(monthTotal))
  const seriesLabel = { fuel: t('cat.fuel'), expenses: t('tab.expenses'), insurance: t('cat.insurance') }

  const delReading = async (r: OdometerReading) => {
    if (!(await confirm({ title: t('common.delete'), message: `${num(r.km, locale)} km · ${date(r.date, locale)}`, confirmLabel: t('common.delete'), danger: true })))
      return
    try {
      await api.del(`/api/odometer/${r.id}`)
      changed()
    } catch (e) {
      fail(e)
    }
  }

  return (
    <>
      <div className="tiles">
        <div className="tile">
          <span className="tile-label">{t('summary.total')}</span>
          <span className="tile-value">{euro(s.total, locale)}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('summary.costKm')}</span>
          <span className="tile-value">{s.cents_per_km !== null ? `${num(s.cents_per_km / 100, locale, 2)} €` : '—'}</span>
        </div>
        <div className="tile">
          <span className="tile-label">{t('summary.kmDriven')}</span>
          <span className="tile-value">{s.km !== null ? `${num(s.km, locale)} km` : '—'}</span>
        </div>
      </div>

      <section className="card chart-card">
        <h3>{t('summary.last12')}</h3>
        <div className="legend">
          {series.map((k) => (
            <span key={k}>
              <i className={`swatch s-${k}`} />
              {seriesLabel[k]}
            </span>
          ))}
        </div>
        <div className="month-chart" role="img" aria-label={t('summary.last12')}>
          {s.last_12_months.map((m) => {
            const tot = monthTotal(m)
            const tip = `${month(m.month, locale)}: ${euro(tot, locale)}\n` + series.map((k) => `${seriesLabel[k]}: ${euro(m[k], locale)}`).join('\n')
            return (
              <div key={m.month} className="month-col" title={tip}>
                <div className="month-bar" style={{ height: `${(tot / maxMonth) * 100}%` }}>
                  {series.map((k) => m[k] > 0 && <div key={k} className={`seg s-${k}`} style={{ flexGrow: m[k] }} />)}
                </div>
                <span className="month-label">{month(m.month, locale, true)}</span>
              </div>
            )
          })}
        </div>
        <div className="sr-only">
          <table>
          <tbody>
            {s.last_12_months.map((m) => (
              <tr key={m.month}>
                <th>{month(m.month, locale)}</th>
                {series.map((k) => (
                  <td key={k}>
                    {seriesLabel[k]} {euro(m[k], locale)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        </div>
      </section>

      {cats.length > 0 && (
        <section className="card chart-card">
          <h3>{t('summary.byCategory')}</h3>
          <div className="hbars">
            {cats.map(([c, v]) => (
              <div key={c} className="hbar-row">
                <span className="hbar-label">{t(`cat.${c}` as 'cat.other')}</span>
                <span className="hbar-track">
                  <span className="hbar" style={{ width: `${(v / maxCat) * 100}%` }} />
                </span>
                <span className="hbar-value">{euro(v, locale)}</span>
              </div>
            ))}
          </div>
        </section>
      )}

      <section className="card chart-card">
        <h3>{t('summary.odometer')}</h3>
        {readings.data.length === 0 ? (
          <p className="muted">{t('summary.odometerEmpty')}</p>
        ) : (
          <ul className="readings">
            {readings.data.map((r) => (
              <li key={r.id}>
                <span>{date(r.date, locale)}</span>
                <strong>{num(r.km, locale)} km</strong>
                {canEdit && (
                  <button className="btn-icon" onClick={() => delReading(r)} aria-label={t('common.delete')}>
                    <Icon name="trash" size={14} />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  )
}
