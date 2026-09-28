// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { api, type FuelType, type InspectionRule, type Member, type Role, type Vehicle, type VehicleInput, type VehicleKind } from '../api'
import { PageHead, useSession } from '../App'
import { useI18n } from '../i18n'
import { monthName, numInput, parseNumber } from '../format'
import { Icon } from '../components/Icon'
import { Field, Loading, Spinner, useLoad, useUI } from '../components/ui'

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
  notes: '',
}

export function VehicleEdit() {
  const { id } = useParams()
  const { data } = useLoad<Vehicle>(id ? `/api/vehicles/${id}` : null)
  if (id && !data) return <Loading />
  return <VehicleEditForm vehicle={data ?? undefined} />
}

function VehicleEditForm({ vehicle }: { vehicle?: Vehicle }) {
  const { t, locale } = useI18n()
  const { toast, fail, confirm } = useUI()
  const navigate = useNavigate()
  const [v, setV] = useState<VehicleInput>(vehicle ?? empty)
  // numeric fields are edited as text
  const [nums, setNums] = useState({
    initial_odometer: numInput(vehicle?.initial_odometer),
    tank_capacity: numInput(vehicle?.tank_capacity),
    service_interval_km: numInput(vehicle?.service_interval_km),
    service_interval_months: numInput(vehicle?.service_interval_months),
    oil_interval_km: numInput(vehicle?.oil_interval_km),
    oil_interval_months: numInput(vehicle?.oil_interval_months),
  })
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof VehicleInput>(k: K, val: VehicleInput[K]) => setV((x) => ({ ...x, [k]: val }))
  const setNum = (k: keyof typeof nums, s: string) => setNums((x) => ({ ...x, [k]: s }))
  const dateOrNull = (s: string) => (s ? s : null)

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
              <select value={v.kind} onChange={(e) => set('kind', e.target.value as VehicleKind)}>
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
            <Field label={t('vehicle.name')} hint={t('vehicle.nameHint')}>
              <input value={v.name} onChange={(e) => set('name', e.target.value)} />
            </Field>
            <Field label={t('vehicle.plate')}>
              <input value={v.plate} onChange={(e) => set('plate', e.target.value.toUpperCase())} autoCapitalize="characters" />
            </Field>
            <Field label={t('vehicle.registration')}>
              <input type="date" value={v.registration_date ?? ''} onChange={(e) => set('registration_date', dateOrNull(e.target.value))} />
            </Field>
            <Field label={t('vehicle.purchase')}>
              <input type="date" value={v.purchase_date ?? ''} onChange={(e) => set('purchase_date', dateOrNull(e.target.value))} />
            </Field>
            <Field label={t('vehicle.initialKm')}>
              <input inputMode="numeric" value={nums.initial_odometer} onChange={(e) => setNum('initial_odometer', e.target.value)} />
            </Field>
            <Field label={`${t('vehicle.tank')} (${v.fuel_type === 'electric' ? 'kWh' : v.fuel_type === 'cng' ? 'kg' : 'l'})`}>
              <input inputMode="decimal" value={nums.tank_capacity} onChange={(e) => setNum('tank_capacity', e.target.value)} />
            </Field>
            <Field label={t('vehicle.vin')} wide>
              <input value={v.vin} onChange={(e) => set('vin', e.target.value.toUpperCase())} autoCapitalize="characters" />
            </Field>
          </div>
        </section>

        <section className="card form-card">
          <h2>{t('vehicle.section.deadlines')}</h2>
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
            <Field label={t('vehicle.oilKm')} hint={t('vehicle.oilHint')}>
              <input inputMode="numeric" value={nums.oil_interval_km} onChange={(e) => setNum('oil_interval_km', e.target.value)} placeholder="5000" />
            </Field>
            <Field label={t('vehicle.oilMonths')}>
              <input inputMode="numeric" value={nums.oil_interval_months} onChange={(e) => setNum('oil_interval_months', e.target.value)} placeholder="12" />
            </Field>
            <Field label={t('common.notes')} wide>
              <textarea rows={3} value={v.notes} onChange={(e) => set('notes', e.target.value)} />
            </Field>
          </div>
        </section>

        <div className="form-footer">
          <button className="btn btn-primary" disabled={busy}>
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
