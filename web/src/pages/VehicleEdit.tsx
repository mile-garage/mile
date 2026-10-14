// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { api, twoWheels, type FuelType, type InspectionRule, type Member, type Role, type Vehicle, type VehicleInput, type VehicleKind } from '../api'
import { PageHead, useSession } from '../App'
import { useI18n } from '../i18n'
import { date, monthName, num, numInput, parseNumber, today, unit } from '../format'
import { Icon } from '../components/Icon'
import { Field, Loading, Spinner, useLoad, useUI } from '../components/ui'
import { MoreToggle } from '../components/Forms'

const kinds: VehicleKind[] = ['car', 'motorcycle', 'moped', 'van', 'truck', 'other']
const fuels: FuelType[] = ['petrol', 'diesel', 'lpg', 'cng', 'hybrid', 'electric', 'other']
const rules: InspectionRule[] = ['standard', 'annual', 'none']

const empty: VehicleInput = {
  name: '',
  kind: 'car',
  make: '',
  model: '',
  plate: '',
  vin: '',
  fuel_type: 'petrol',
  registration_date: null,
  purchase_date: null,
  initial_odometer: null,
  tank_capacity: null,
  inspection_rule: 'standard',
  tax_month: null,
  tax_exempt_until: null,
  service_interval_km: null,
  service_interval_months: null,
  oil_interval_km: null,
  oil_interval_months: null,
  transmission_oil_interval_km: null,
  transmission_oil_interval_months: null,
  brake_pads_interval_km: null,
  brake_discs_interval_km: null,
  tyre_rotation_km: null,
  notes: '',
}

export function VehicleEdit() {
  const { id } = useParams()
  const { data } = useLoad<Vehicle>(id ? `/api/vehicles/${id}` : null)
  if (id && !data) return <Loading />
  return <VehicleEditForm vehicle={data ?? undefined} />
}

/** Service interval offered for a new vehicle, by kind. */
function serviceDefaults(kind: VehicleKind): { km: number; months: number } {
  if (kind === 'moped') return { km: 5000, months: 12 }
  if (kind === 'motorcycle') return { km: 10000, months: 12 }
  return { km: 15000, months: 12 }
}

/** Values filled in when an optional deadline is switched on. */
const extraDefaults = {
  oil: { oil_interval_km: '5000', oil_interval_months: '12' },
  gearbox: { transmission_oil_interval_km: '60000', transmission_oil_interval_months: '48' },
  brakes: { brake_pads_interval_km: '40000', brake_discs_interval_km: '80000' },
  rotation: { tyre_rotation_km: '10000' },
}
type Extra = keyof typeof extraDefaults

function VehicleEditForm({ vehicle }: { vehicle?: Vehicle }) {
  const { t, locale } = useI18n()
  const { toast, fail, confirm } = useUI()
  const navigate = useNavigate()
  const [v, setV] = useState<VehicleInput>(vehicle ?? empty)
  // numeric fields are edited as text; a new vehicle starts with the usual service interval
  const service = serviceDefaults(v.kind)
  const [nums, setNums] = useState({
    initial_odometer: numInput(vehicle?.initial_odometer),
    tank_capacity: numInput(vehicle?.tank_capacity),
    service_interval_km: vehicle ? numInput(vehicle.service_interval_km) : String(service.km),
    service_interval_months: vehicle ? numInput(vehicle.service_interval_months) : String(service.months),
    oil_interval_km: numInput(vehicle?.oil_interval_km),
    oil_interval_months: numInput(vehicle?.oil_interval_months),
    transmission_oil_interval_km: numInput(vehicle?.transmission_oil_interval_km),
    transmission_oil_interval_months: numInput(vehicle?.transmission_oil_interval_months),
    brake_pads_interval_km: numInput(vehicle?.brake_pads_interval_km),
    brake_discs_interval_km: numInput(vehicle?.brake_discs_interval_km),
    tyre_rotation_km: numInput(vehicle?.tyre_rotation_km),
  })
  const [busy, setBusy] = useState(false)
  const [more, setMore] = useState(!!(vehicle && (vehicle.purchase_date || vehicle.tank_capacity || vehicle.vin || vehicle.notes)))
  const [custom, setCustom] = useState(false)
  const set = <K extends keyof VehicleInput>(k: K, val: VehicleInput[K]) => setV((x) => ({ ...x, [k]: val }))
  const setNum = (k: keyof typeof nums, s: string) => setNums((x) => ({ ...x, [k]: s }))
  const dateOrNull = (s: string) => (s ? s : null)
  const two = twoWheels(v.kind)

  const setKind = (k: VehicleKind) => {
    // a new vehicle follows the kind, unless the interval was changed by hand
    const was = serviceDefaults(v.kind)
    const next = serviceDefaults(k)
    if (!vehicle && nums.service_interval_km === String(was.km) && nums.service_interval_months === String(was.months))
      setNums((x) => ({ ...x, service_interval_km: String(next.km), service_interval_months: String(next.months) }))
    set('kind', k)
  }

  const setRegistration = (s: string) => {
    // the road tax usually expires in the month of the first registration
    setV((x) => ({ ...x, registration_date: dateOrNull(s), tax_month: x.tax_month ?? (s ? Number(s.slice(5, 7)) : null) }))
  }

  const extraOn: Record<Extra, boolean> = {
    oil: !!(nums.oil_interval_km || nums.oil_interval_months),
    gearbox: !!(nums.transmission_oil_interval_km || nums.transmission_oil_interval_months),
    brakes: !!(nums.brake_pads_interval_km || nums.brake_discs_interval_km),
    rotation: !!nums.tyre_rotation_km,
  }
  const toggleExtra = (e: Extra) => {
    const keys = Object.keys(extraDefaults[e]) as (keyof typeof nums)[]
    setNums((x) => {
      const y = { ...x }
      for (const k of keys) y[k] = extraOn[e] ? '' : (extraDefaults[e] as Record<string, string>)[k]
      return y
    })
  }

  const every = (km: string, months: string) => {
    const k = parseNumber(km)
    const m = parseNumber(months)
    const kmText = k ? num(k, locale) : ''
    if (k && m) return t('vehicle.sum.kmMonths', { km: kmText, months: m })
    if (k) return t('vehicle.sum.km', { km: kmText })
    if (m) return t('vehicle.sum.months', { months: m })
    return t('vehicle.sum.off')
  }
  const taxText =
    v.tax_exempt_until && v.tax_exempt_until >= today()
      ? t('vehicle.sum.taxExempt', { date: date(v.tax_exempt_until, locale) })
      : v.tax_month
        ? t('vehicle.sum.taxMonth', { month: monthName(v.tax_month, locale) })
        : t('vehicle.sum.taxNone')

  const extras: { key: Extra; title: string; on: string; off: string; show: boolean }[] = [
    { key: 'oil', title: t('vehicle.extra.oil'), on: every(nums.oil_interval_km, nums.oil_interval_months), off: t('vehicle.extra.oilOff'), show: true },
    {
      key: 'gearbox',
      title: t('vehicle.extra.gearbox'),
      on: every(nums.transmission_oil_interval_km, nums.transmission_oil_interval_months),
      off: t('vehicle.extra.gearboxOff'),
      show: !two,
    },
    {
      key: 'brakes',
      title: t('vehicle.extra.brakes'),
      on: t('vehicle.extra.brakesOn', { pads: num(parseNumber(nums.brake_pads_interval_km) ?? 0, locale), discs: num(parseNumber(nums.brake_discs_interval_km) ?? 0, locale) }),
      off: t('vehicle.extra.brakesOff'),
      show: true,
    },
    { key: 'rotation', title: t('vehicle.extra.rotation'), on: every(nums.tyre_rotation_km, ''), off: t('vehicle.extra.rotationOff'), show: !two },
  ]

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const parsed: Partial<VehicleInput> = {}
    for (const [k, s] of Object.entries(nums)) {
      const n = parseNumber(s)
      if (n !== null && (Number.isNaN(n) || n < 0)) return fail(new Error(t('err.number')))
      ;(parsed as Record<string, number | null>)[k] = n === null ? null : k === 'tank_capacity' ? n : Math.round(n)
    }
    setBusy(true)
    try {
      if (two) {
        parsed.tyre_rotation_km = null
        parsed.transmission_oil_interval_km = parsed.transmission_oil_interval_months = null
      }
      const body = { ...v, ...parsed }
      const saved = vehicle ? await api.put<Vehicle>(`/api/vehicles/${vehicle.id}`, body) : await api.post<Vehicle>('/api/vehicles', body)
      toast(t('common.saved'))
      navigate(`/vehicles/${saved.id}`, { replace: !vehicle })
    } catch (err) {
      fail(err)
      setBusy(false)
    }
  }

  const archive = async () => {
    if (!vehicle) return
    if (!vehicle.archived && !(await confirm({ title: t('vehicle.archive'), message: t('vehicle.archiveText'), confirmLabel: t('vehicle.archive') })))
      return
    try {
      await api.post(`/api/vehicles/${vehicle.id}/archive`, { archived: !vehicle.archived })
      navigate(`/vehicles/${vehicle.id}`)
    } catch (err) {
      fail(err)
    }
  }

  const remove = async () => {
    if (!vehicle) return
    if (!(await confirm({ title: t('vehicle.delete'), message: t('vehicle.deleteText'), confirmLabel: t('common.delete'), danger: true }))) return
    try {
      await api.del(`/api/vehicles/${vehicle.id}`)
      toast(t('common.deleted'))
      navigate('/vehicles')
    } catch (err) {
      fail(err)
    }
  }

  const back = (
    <Link className="btn-icon back" to={vehicle ? `/vehicles/${vehicle.id}` : '/vehicles'} aria-label={t('common.back')}>
      <Icon name="back" />
    </Link>
  )

  return (
    <div className="page">
      <PageHead title={vehicle ? t('vehicle.edit') : t('vehicle.new')} back={back} />
      <form onSubmit={submit}>
        <section className="card form-card">
          <h2>{t('vehicle.section.main')}</h2>
          <div className="form-grid">
            <Field label={t('vehicle.kind')}>
              <select value={v.kind} onChange={(e) => setKind(e.target.value as VehicleKind)}>
                {kinds.map((k) => (
                  <option key={k} value={k}>
                    {t(`kind.${k}`)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t('vehicle.fuel')}>
              <select value={v.fuel_type} onChange={(e) => set('fuel_type', e.target.value as FuelType)}>
                {fuels.map((f) => (
                  <option key={f} value={f}>
                    {t(`fuel.${f}`)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t('vehicle.make')}>
              <input value={v.make} onChange={(e) => set('make', e.target.value)} autoFocus={!vehicle} />
            </Field>
            <Field label={t('vehicle.model')}>
              <input value={v.model} onChange={(e) => set('model', e.target.value)} />
            </Field>
            <Field label={t('vehicle.plate')}>
              <input value={v.plate} onChange={(e) => set('plate', e.target.value.toUpperCase())} autoCapitalize="characters" />
            </Field>
            <Field label={t('vehicle.registration')}>
              <input type="date" value={v.registration_date ?? ''} onChange={(e) => setRegistration(e.target.value)} />
            </Field>
            <Field label={t('vehicle.name')} hint={t('vehicle.nameHint')}>
              <input value={v.name} onChange={(e) => set('name', e.target.value)} />
            </Field>
            <Field label={t('vehicle.initialKm')}>
              <input inputMode="numeric" value={nums.initial_odometer} onChange={(e) => setNum('initial_odometer', e.target.value)} />
            </Field>
          </div>
          <MoreToggle open={more} onToggle={() => setMore(!more)} label={t('vehicle.moreData')} />
          {more && (
            <div className="form-grid">
              <Field label={t('vehicle.purchase')}>
                <input type="date" value={v.purchase_date ?? ''} onChange={(e) => set('purchase_date', dateOrNull(e.target.value))} />
              </Field>
              <Field label={`${t('vehicle.tank')} (${unit(v.fuel_type)})`}>
                <input inputMode="decimal" value={nums.tank_capacity} onChange={(e) => setNum('tank_capacity', e.target.value)} />
              </Field>
              <Field label={t('vehicle.vin')} wide>
                <input value={v.vin} onChange={(e) => set('vin', e.target.value.toUpperCase())} autoCapitalize="characters" />
              </Field>
              <Field label={t('common.notes')} wide>
                <textarea rows={3} value={v.notes} onChange={(e) => set('notes', e.target.value)} />
              </Field>
            </div>
          )}
        </section>

        <section className="card form-card">
          <h2>{t('vehicle.section.maintenance')}</h2>
          <p className="muted">{t('vehicle.maintText')}</p>
          <div className="check-list">
            <div className="check-item">
              <Icon name="check" />
              <span>
                <strong>{t('vehicle.sum.inspection')}</strong>
                <span className="muted">{t(`vehicle.inspection.${v.inspection_rule}`)}</span>
              </span>
            </div>
            <div className="check-item">
              <Icon name="check" />
              <span>
                <strong>{t('vehicle.sum.tax')}</strong>
                <span className="muted">{taxText}</span>
              </span>
            </div>
            <div className="check-item">
              <Icon name="check" />
              <span>
                <strong>{t('vehicle.sum.service')}</strong>
                <span className="muted">{every(nums.service_interval_km, nums.service_interval_months)}</span>
              </span>
            </div>
          </div>

          <h3 className="section-title">{t('vehicle.extras')}</h3>
          <div className="switch-list">
            {extras
              .filter((x) => x.show)
              .map((x) => (
                <label key={x.key} className="switch-row">
                  <span>
                    {x.title}
                    <small>{extraOn[x.key] ? x.on : x.off}</small>
                  </span>
                  <input type="checkbox" className="switch" checked={extraOn[x.key]} onChange={() => toggleExtra(x.key)} />
                </label>
              ))}
          </div>

          <MoreToggle open={custom} onToggle={() => setCustom(!custom)} label={t('vehicle.customize')} />
          {custom && (
            <div className="form-grid">
              <Field label={t('vehicle.inspection')} wide>
                <select value={v.inspection_rule} onChange={(e) => set('inspection_rule', e.target.value as InspectionRule)}>
                  {rules.map((r) => (
                    <option key={r} value={r}>
                      {t(`vehicle.inspection.${r}`)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={t('vehicle.taxMonth')} hint={t('vehicle.taxMonthHint')}>
                <select value={v.tax_month ?? ''} onChange={(e) => set('tax_month', e.target.value ? Number(e.target.value) : null)}>
                  <option value="">—</option>
                  {Array.from({ length: 12 }, (_, i) => (
                    <option key={i + 1} value={i + 1}>
                      {monthName(i + 1, locale)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={t('vehicle.taxExempt')} hint={t('vehicle.taxExemptHint')}>
                <input type="date" value={v.tax_exempt_until ?? ''} onChange={(e) => set('tax_exempt_until', dateOrNull(e.target.value))} />
              </Field>
              <Field label={t('vehicle.serviceKm')}>
                <input inputMode="numeric" value={nums.service_interval_km} onChange={(e) => setNum('service_interval_km', e.target.value)} placeholder="15000" />
              </Field>
              <Field label={t('vehicle.serviceMonths')}>
                <input inputMode="numeric" value={nums.service_interval_months} onChange={(e) => setNum('service_interval_months', e.target.value)} placeholder="12" />
              </Field>
              {extraOn.oil && (
                <>
                  <Field label={t('vehicle.oilKm')} hint={t('vehicle.oilHint')}>
                    <input inputMode="numeric" value={nums.oil_interval_km} onChange={(e) => setNum('oil_interval_km', e.target.value)} placeholder="5000" />
                  </Field>
                  <Field label={t('vehicle.oilMonths')}>
                    <input inputMode="numeric" value={nums.oil_interval_months} onChange={(e) => setNum('oil_interval_months', e.target.value)} placeholder="12" />
                  </Field>
                </>
              )}
              {extraOn.gearbox && !two && (
                <>
                  <Field label={t('vehicle.transmissionOilKm')} hint={t('vehicle.transmissionOilHint')}>
                    <input
                      inputMode="numeric"
                      value={nums.transmission_oil_interval_km}
                      onChange={(e) => setNum('transmission_oil_interval_km', e.target.value)}
                      placeholder="60000"
                    />
                  </Field>
                  <Field label={t('vehicle.transmissionOilMonths')}>
                    <input
                      inputMode="numeric"
                      value={nums.transmission_oil_interval_months}
                      onChange={(e) => setNum('transmission_oil_interval_months', e.target.value)}
                      placeholder="48"
                    />
                  </Field>
                </>
              )}
              {extraOn.brakes && (
                <>
                  <Field label={t('vehicle.brakePadsKm')} hint={t('vehicle.brakePadsHint')}>
                    <input inputMode="numeric" value={nums.brake_pads_interval_km} onChange={(e) => setNum('brake_pads_interval_km', e.target.value)} placeholder="40000" />
                  </Field>
                  <Field label={t('vehicle.brakeDiscsKm')} hint={t('vehicle.brakeDiscsHint')}>
                    <input inputMode="numeric" value={nums.brake_discs_interval_km} onChange={(e) => setNum('brake_discs_interval_km', e.target.value)} placeholder="80000" />
                  </Field>
                </>
              )}
              {extraOn.rotation && !two && (
                <Field label={t('vehicle.tyreRotationKm')} hint={t('vehicle.tyreRotationHint')} wide>
                  <input inputMode="numeric" value={nums.tyre_rotation_km} onChange={(e) => setNum('tyre_rotation_km', e.target.value)} placeholder="10000" />
                </Field>
              )}
            </div>
          )}
        </section>

        <div className="form-footer">
          <button className="btn btn-cta btn-lg" disabled={busy}>
            {busy ? <Spinner small /> : t('common.save')}
          </button>
        </div>
      </form>

      {vehicle && vehicle.role === 'owner' && (
        <>
          <Sharing vehicleId={vehicle.id} />
          <section className="card form-card danger-zone">
            <h2>{t('vehicle.danger')}</h2>
            <div className="row-actions">
              <button className="btn" onClick={archive}>
                {vehicle.archived ? t('vehicle.unarchive') : t('vehicle.archive')}
              </button>
              <button className="btn btn-danger" onClick={remove}>
                <Icon name="trash" size={16} />
                {t('vehicle.delete')}
              </button>
            </div>
          </section>
        </>
      )}
    </div>
  )
}

function Sharing({ vehicleId }: { vehicleId: number }) {
  const { t } = useI18n()
  const { fail } = useUI()
  const { user } = useSession()
  const [members, setMembers] = useState<Member[] | null>(null)
  const [username, setUsername] = useState('')
  const [role, setRole] = useState<Role>('editor')

  useEffect(() => {
    api.get<Member[]>(`/api/vehicles/${vehicleId}/members`).then(setMembers).catch(fail)
  }, [vehicleId, fail])

  const save = async (name: string, r: Role) => {
    try {
      setMembers(await api.put<Member[]>(`/api/vehicles/${vehicleId}/members`, { username: name, role: r }))
      setUsername('')
    } catch (e) {
      fail(e)
    }
  }

  const remove = async (m: Member) => {
    try {
      setMembers(await api.del<Member[]>(`/api/vehicles/${vehicleId}/members/${m.user_id}`))
    } catch (e) {
      fail(e)
    }
  }

  if (!members) return null
  return (
    <section className="card form-card">
      <h2>
        <Icon name="share" size={18} /> {t('tab.sharing')}
      </h2>
      <p className="muted">{t('sharing.text')}</p>
      <ul className="members">
        {members.map((m) => (
          <li key={m.user_id}>
            <span>
              <strong>{m.display_name || m.username}</strong> <span className="muted">@{m.username}</span>
            </span>
            <select value={m.role} onChange={(e) => save(m.username, e.target.value as Role)} disabled={m.user_id === user.id}>
              {(['owner', 'editor', 'viewer'] as Role[]).map((r) => (
                <option key={r} value={r}>
                  {t(`role.${r}`)}
                </option>
              ))}
            </select>
            {m.user_id !== user.id && (
              <button className="btn-icon" onClick={() => remove(m)} aria-label={t('common.delete')}>
                <Icon name="trash" size={16} />
              </button>
            )}
          </li>
        ))}
      </ul>
      <form
        className="inline-form"
        onSubmit={(e) => {
          e.preventDefault()
          if (username.trim()) save(username.trim(), role)
        }}
      >
        <input placeholder={t('sharing.user')} value={username} onChange={(e) => setUsername(e.target.value)} autoCapitalize="none" />
        <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
          <option value="editor">{t('role.editor')}</option>
          <option value="viewer">{t('role.viewer')}</option>
          <option value="owner">{t('role.owner')}</option>
        </select>
        <button className="btn">{t('sharing.add')}</button>
      </form>
    </section>
  )
}
