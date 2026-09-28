// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useState, type FormEvent, type InputHTMLAttributes, type ReactNode } from 'react'
import {
  api,
  expenseCategories,
  type Attachment,
  type Expense,
  type ExpenseCategory,
  type ExpenseInput,
  type OdometerReading,
  type Policy,
  type PolicyInput,
  type Refuel,
  type RefuelInput,
  type Reminder,
  type ReminderInput,
  type Vehicle,
} from '../api'
import { useI18n } from '../i18n'
import { addMonths, centsInput, num, numInput, parseCents, parseNumber, today, unit } from '../format'
import { FileList, upload, UploadButton, type UploadTarget } from './Files'
import { Icon } from './Icon'
import { Field, Modal, Spinner, useUI } from './ui'

// ---- shared pieces ----

function Actions({ busy, onClose, onDelete }: { busy: boolean; onClose: () => void; onDelete?: () => void }) {
  const { t } = useI18n()
  return (
    <div className="modal-actions">
      {onDelete && (
        <button type="button" className="btn btn-danger-ghost push-left" onClick={onDelete}>
          <Icon name="trash" size={16} />
          {t('common.delete')}
        </button>
      )}
      <button type="button" className="btn" onClick={onClose}>
        {t('common.cancel')}
      </button>
      <button className="btn btn-primary" disabled={busy}>
        {busy ? <Spinner small /> : t('common.save')}
      </button>
    </div>
  )
}

/**
 * Attachments inside a form: existing records upload immediately; for new
 * records the files are queued and uploaded after saving.
 */
function FormFiles({
  target,
  existing,
  pending,
  setPending,
  onChange,
}: {
  target: UploadTarget | null
  existing: Attachment[]
  pending: File[]
  setPending: (f: File[]) => void
  onChange: (a: Attachment[]) => void
}) {
  const { t } = useI18n()
  return (
    <div className="field field-wide">
      <FileList files={existing} canEdit onDeleted={(id) => onChange(existing.filter((a) => a.id !== id))} />
      {pending.length > 0 && (
        <ul className="files">
          {pending.map((f, i) => (
            <li key={i}>
              <span>
                <Icon name="file" size={16} />
                <span className="file-name">{f.name}</span>
              </span>
              <button type="button" className="btn-icon" onClick={() => setPending(pending.filter((_, j) => j !== i))} aria-label={t('common.delete')}>
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      {target ? (
        <UploadButton target={target} multiple onUploaded={(a) => onChange([...existing, a])} />
      ) : (
        <label className="btn btn-sm file-pick">
          <Icon name="paperclip" size={16} />
          {t('files.attach')}
          <input
            type="file"
            hidden
            multiple
            accept="application/pdf,image/*"
            onChange={(e) => {
              setPending([...pending, ...Array.from(e.target.files ?? [])])
              e.target.value = ''
            }}
          />
        </label>
      )}
    </div>
  )
}

function useSubmit() {
  const { fail } = useUI()
  const [busy, setBusy] = useState(false)
  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      fail(e)
    } finally {
      setBusy(false)
    }
  }
  return { busy, run }
}

/** Number field that accepts both "1.234,5" and "1234.5". */
function NumInput({ value, onChange, decimal, ...rest }: { value: string; onChange: (s: string) => void; decimal?: boolean } & Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  return <input {...rest} inputMode={decimal ? 'decimal' : 'numeric'} value={value} onChange={(e) => onChange(e.target.value)} />
}

function checkNum(v: number | null, required: boolean): boolean {
  if (v === null) return !required
  return !Number.isNaN(v) && v >= 0
}

async function uploadPending(files: File[], target: UploadTarget) {
  for (const f of files) await upload(target, f)
}

function useDelete(url: string | null, text: string, onDone: () => void) {
  const { t } = useI18n()
  const { confirm, fail, toast } = useUI()
  if (!url) return undefined
  return async () => {
    if (!(await confirm({ title: t('common.delete'), message: text, confirmLabel: t('common.delete'), danger: true }))) return
    try {
      await api.del(url)
      toast(t('common.deleted'))
      onDone()
    } catch (e) {
      fail(e)
    }
  }
}

// ---- expense ----

export function ExpenseForm({
  vehicle,
  expense,
  preset,
  onSaved,
  onClose,
}: {
  vehicle: Vehicle
  expense?: Expense
  preset?: Partial<ExpenseInput>
  onSaved: () => void
  onClose: () => void
}) {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const init = { ...preset, ...expense }
  const [date, setDate] = useState(init.date ?? today())
  const [category, setCategory] = useState<ExpenseCategory>(init.category ?? 'service')
  const [amount, setAmount] = useState(centsInput(init.amount_cents))
  const [odometer, setOdometer] = useState(numInput(init.odometer))
  const [description, setDescription] = useState(init.description ?? '')
  const [vendor, setVendor] = useState(init.vendor ?? '')
  const [validUntil, setValidUntil] = useState(init.valid_until ?? '')
  const [notes, setNotes] = useState(init.notes ?? '')
  const [files, setFiles] = useState<Attachment[]>(expense?.attachments ?? [])
  const [pending, setPending] = useState<File[]>([])
  const { busy, run } = useSubmit()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const cents = parseCents(amount)
    const km = parseNumber(odometer)
    if (!checkNum(cents, true) || !checkNum(km, false)) return fail(new Error(t('err.number')))
    const body: ExpenseInput = {
      date,
      category,
      amount_cents: cents!,
      odometer: km === null ? null : Math.round(km),
      description,
      vendor,
      valid_until: category === 'road_tax' && validUntil ? validUntil : null,
      notes,
    }
    run(async () => {
      const saved = expense
        ? await api.put<Expense>(`/api/expenses/${expense.id}`, body)
        : await api.post<Expense>(`/api/vehicles/${vehicle.id}/expenses`, body)
      await uploadPending(pending, { vehicleId: vehicle.id, kind: 'document', expense_id: saved.id })
      toast(t('common.saved'))
      onSaved()
    })
  }

  const onDelete = useDelete(expense ? `/api/expenses/${expense.id}` : null, t('expense.deleteText'), onSaved)

  return (
    <Modal title={expense ? t('expense.edit') : t('expense.new')} onClose={onClose}>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('expense.category')}>
          <select value={category} onChange={(e) => setCategory(e.target.value as ExpenseCategory)}>
            {expenseCategories.map((c) => (
              <option key={c} value={c}>
                {t(`cat.${c}`)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('common.date')}>
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </Field>
        <Field label={t('common.amount') + ' (€)'}>
          <NumInput decimal value={amount} onChange={setAmount} required placeholder="0,00" />
        </Field>
        <Field label={`${t('common.km')} (${t('common.optional')})`} hint={vehicle.current_km !== null ? t('refuel.odometerHint', { km: num(vehicle.current_km, locale) }) : undefined}>
          <NumInput value={odometer} onChange={setOdometer} />
        </Field>
        {category === 'road_tax' && (
          <Field label={t('expense.validUntil')} wide>
            <input type="month" value={validUntil} onChange={(e) => setValidUntil(e.target.value)} />
          </Field>
        )}
        <Field label={t('expense.description')} wide>
          <input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label={t('expense.vendor')} wide>
          <input value={vendor} onChange={(e) => setVendor(e.target.value)} />
        </Field>
        <Field label={t('common.notes')} wide>
          <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <FormFiles
          target={expense ? { vehicleId: vehicle.id, kind: 'document', expense_id: expense.id } : null}
          existing={files}
          pending={pending}
          setPending={setPending}
          onChange={setFiles}
        />
        <Actions busy={busy} onClose={onClose} onDelete={onDelete} />
      </form>
    </Modal>
  )
}

// ---- refuel ----

export function RefuelForm({
  vehicle,
  refuel,
  onSaved,
  onClose,
}: {
  vehicle: Vehicle
  refuel?: Refuel
  onSaved: () => void
  onClose: () => void
}) {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const u = unit(vehicle.fuel_type)
  const [date, setDate] = useState(refuel?.date ?? today())
  const [odometer, setOdometer] = useState(numInput(refuel?.odometer))
  const [quantity, setQuantity] = useState(numInput(refuel?.quantity))
  const [price, setPrice] = useState(refuel?.unit_price ? refuel.unit_price.toFixed(3).replace('.', ',') : '')
  const [total, setTotal] = useState(centsInput(refuel?.total_cents))
  const [full, setFull] = useState(refuel?.full_tank ?? true)
  const [missed, setMissed] = useState(refuel?.missed_previous ?? false)
  const [station, setStation] = useState(refuel?.station ?? '')
  const [notes, setNotes] = useState(refuel?.notes ?? '')
  const [files, setFiles] = useState<Attachment[]>(refuel?.attachments ?? [])
  const [pending, setPending] = useState<File[]>([])
  const { busy, run } = useSubmit()

  // With two of quantity, unit price and total, compute the third.
  const complete = () => {
    const q = parseNumber(quantity)
    const p = parseNumber(price)
    const tot = parseNumber(total)
    const ok = (v: number | null) => v !== null && !Number.isNaN(v) && v > 0
    if (ok(q) && ok(p) && !ok(tot)) setTotal((q! * p!).toFixed(2).replace('.', ','))
    else if (ok(tot) && ok(p) && !ok(q)) setQuantity((tot! / p!).toFixed(2).replace('.', ','))
    else if (ok(tot) && ok(q) && !ok(p)) setPrice((tot! / q!).toFixed(3).replace('.', ','))
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const km = parseNumber(odometer)
    const q = parseNumber(quantity)
    const cents = parseCents(total)
    if (!checkNum(km, true) || !checkNum(q, true) || !checkNum(cents, true)) return fail(new Error(t('err.number')))
    const body: RefuelInput = {
      date,
      odometer: Math.round(km!),
      quantity: q!,
      total_cents: cents!,
      full_tank: full,
      missed_previous: missed,
      station,
      notes,
    }
    run(async () => {
      const saved = refuel
        ? await api.put<Refuel>(`/api/refuels/${refuel.id}`, body)
        : await api.post<Refuel>(`/api/vehicles/${vehicle.id}/refuels`, body)
      await uploadPending(pending, { vehicleId: vehicle.id, kind: 'document', refuel_id: saved.id })
      toast(t('common.saved'))
      onSaved()
    })
  }

  const onDelete = useDelete(refuel ? `/api/refuels/${refuel.id}` : null, t('refuel.deleteText'), onSaved)

  return (
    <Modal title={refuel ? t('refuel.edit') : t('refuel.new')} onClose={onClose}>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('common.date')}>
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </Field>
        <Field label={t('common.km')} hint={vehicle.current_km !== null && !refuel ? t('refuel.odometerHint', { km: num(vehicle.current_km, locale) }) : undefined}>
          <NumInput value={odometer} onChange={setOdometer} required autoFocus={!refuel} />
        </Field>
        <Field label={t('refuel.quantity', { unit: u })}>
          <NumInput decimal value={quantity} onChange={setQuantity} onBlur={complete} required />
        </Field>
        <Field label={t('refuel.unitPrice', { unit: u }) + ' (€)'}>
          <NumInput decimal value={price} onChange={setPrice} onBlur={complete} placeholder={t('common.optional')} />
        </Field>
        <Field label={t('refuel.total') + ' (€)'} wide>
          <NumInput decimal value={total} onChange={setTotal} onBlur={complete} required />
        </Field>
        <label className="check field-wide">
          <input type="checkbox" checked={full} onChange={(e) => setFull(e.target.checked)} />
          {t('refuel.full')}
        </label>
        <label className="check field-wide">
          <input type="checkbox" checked={missed} onChange={(e) => setMissed(e.target.checked)} />
          <span>
            {t('refuel.missed')}
            <span className="field-hint">{t('refuel.missedHint')}</span>
          </span>
        </label>
        <Field label={t('refuel.station')} wide>
          <input value={station} onChange={(e) => setStation(e.target.value)} />
        </Field>
        <Field label={t('common.notes')} wide>
          <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <FormFiles
          target={refuel ? { vehicleId: vehicle.id, kind: 'document', refuel_id: refuel.id } : null}
          existing={files}
          pending={pending}
          setPending={setPending}
          onChange={setFiles}
        />
        <Actions busy={busy} onClose={onClose} onDelete={onDelete} />
      </form>
    </Modal>
  )
}

// ---- policy ----

export function PolicyForm({
  vehicle,
  policy,
  preset,
  onSaved,
  onClose,
}: {
  vehicle: Vehicle
  policy?: Policy
  preset?: Partial<PolicyInput>
  onSaved: () => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const { toast, fail } = useUI()
  const init = { ...preset, ...policy }
  const [insurer, setInsurer] = useState(init.insurer ?? '')
  const [number, setNumber] = useState(init.policy_number ?? '')
  const [start, setStart] = useState(init.start_date ?? today())
  const [end, setEnd] = useState(init.end_date ?? addMonths(init.start_date ?? today(), 12))
  const [premium, setPremium] = useState(centsInput(init.premium_cents))
  const [notes, setNotes] = useState(init.notes ?? '')
  const [files, setFiles] = useState<Attachment[]>(policy?.attachments ?? [])
  const [pending, setPending] = useState<File[]>([])
  const { busy, run } = useSubmit()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const cents = parseCents(premium)
    if (!checkNum(cents, false)) return fail(new Error(t('err.number')))
    const body: PolicyInput = { insurer, policy_number: number, start_date: start, end_date: end, premium_cents: cents, notes }
    run(async () => {
      const saved = policy
        ? await api.put<Policy>(`/api/policies/${policy.id}`, body)
        : await api.post<Policy>(`/api/vehicles/${vehicle.id}/policies`, body)
      await uploadPending(pending, { vehicleId: vehicle.id, kind: 'document', policy_id: saved.id })
      toast(t('common.saved'))
      onSaved()
    })
  }

  const onDelete = useDelete(policy ? `/api/policies/${policy.id}` : null, t('policy.deleteText'), onSaved)

  return (
    <Modal title={policy ? t('policy.edit') : t('policy.new')} onClose={onClose}>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('policy.insurer')}>
          <input value={insurer} onChange={(e) => setInsurer(e.target.value)} />
        </Field>
        <Field label={t('policy.number')}>
          <input value={number} onChange={(e) => setNumber(e.target.value)} />
        </Field>
        <Field label={t('policy.start')}>
          <input
            type="date"
            value={start}
            onChange={(e) => {
              if (!policy && e.target.value && end === addMonths(start, 12)) setEnd(addMonths(e.target.value, 12))
              setStart(e.target.value)
            }}
            required
          />
        </Field>
        <Field label={t('policy.end')}>
          <input type="date" value={end} onChange={(e) => setEnd(e.target.value)} required />
        </Field>
        <Field label={t('policy.premium') + ' (€)'} wide>
          <NumInput decimal value={premium} onChange={setPremium} placeholder={t('common.optional')} />
        </Field>
        <Field label={t('common.notes')} wide>
          <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <FormFiles
          target={policy ? { vehicleId: vehicle.id, kind: 'document', policy_id: policy.id } : null}
          existing={files}
          pending={pending}
          setPending={setPending}
          onChange={setFiles}
        />
        <Actions busy={busy} onClose={onClose} onDelete={onDelete} />
      </form>
    </Modal>
  )
}

// ---- reminder ----

export function ReminderForm({
  vehicles,
  reminder,
  vehicleId,
  onSaved,
  onClose,
}: {
  vehicles: Vehicle[]
  reminder?: Reminder
  vehicleId?: number
  onSaved: () => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const { toast } = useUI()
  const [title, setTitle] = useState(reminder?.title ?? '')
  const [due, setDue] = useState(reminder?.due_date ?? today())
  const [vid, setVid] = useState<string>(String(reminder?.vehicle_id ?? vehicleId ?? ''))
  const [repeat, setRepeat] = useState(numInput(reminder?.repeat_months))
  const [notes, setNotes] = useState(reminder?.notes ?? '')
  const { busy, run } = useSubmit()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const r = parseNumber(repeat)
    const body: ReminderInput = {
      title,
      due_date: due,
      vehicle_id: vid ? Number(vid) : null,
      repeat_months: r && !Number.isNaN(r) && r > 0 ? Math.round(r) : null,
      notes,
    }
    run(async () => {
      if (reminder) await api.put(`/api/reminders/${reminder.id}`, body)
      else await api.post('/api/reminders', body)
      toast(t('common.saved'))
      onSaved()
    })
  }

  const onDelete = useDelete(reminder ? `/api/reminders/${reminder.id}` : null, t('reminder.deleteText'), onSaved)
  const editable = vehicles.filter((v) => v.role !== 'viewer' && !v.archived)

  return (
    <Modal title={reminder ? t('reminder.edit') : t('reminder.new')} onClose={onClose}>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('reminder.title')} hint={t('reminder.titleHint')} wide>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <Field label={t('reminder.due')}>
          <input type="date" value={due} onChange={(e) => setDue(e.target.value)} required />
        </Field>
        <Field label={t('reminder.repeat')}>
          <NumInput value={repeat} onChange={setRepeat} placeholder={t('common.optional')} />
        </Field>
        <Field label={t('reminder.vehicle')} wide>
          <select value={vid} onChange={(e) => setVid(e.target.value)}>
            <option value="">{t('reminder.personal')}</option>
            {editable.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('common.notes')} wide>
          <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <Actions busy={busy} onClose={onClose} onDelete={onDelete} />
      </form>
    </Modal>
  )
}

// ---- odometer ----

export function OdometerForm({ vehicle, onSaved, onClose }: { vehicle: Vehicle; onSaved: () => void; onClose: () => void }) {
  const { t, locale } = useI18n()
  const { toast, fail } = useUI()
  const [date, setDate] = useState(today())
  const [km, setKm] = useState('')
  const { busy, run } = useSubmit()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const v = parseNumber(km)
    if (!checkNum(v, true)) return fail(new Error(t('err.number')))
    run(async () => {
      await api.post<OdometerReading>(`/api/vehicles/${vehicle.id}/odometer`, { date, km: Math.round(v!), notes: '' })
      toast(t('common.saved'))
      onSaved()
    })
  }
  return (
    <Modal title={t('vehicle.updateKm')} onClose={onClose} narrow>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('common.km')} hint={vehicle.current_km !== null ? t('refuel.odometerHint', { km: num(vehicle.current_km, locale) }) : undefined}>
          <NumInput value={km} onChange={setKm} required autoFocus />
        </Field>
        <Field label={t('common.date')}>
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </Field>
        <Actions busy={busy} onClose={onClose} />
      </form>
    </Modal>
  )
}

// ---- a single date (suspend / reactivate) ----

export function DateForm({
  title,
  message,
  submitLabel,
  onSubmit,
  onClose,
}: {
  title: string
  message?: ReactNode
  submitLabel: string
  onSubmit: (date: string) => Promise<void>
  onClose: () => void
}) {
  const { t } = useI18n()
  const [date, setDate] = useState(today())
  const { busy, run } = useSubmit()
  return (
    <Modal title={title} onClose={onClose} narrow>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          run(() => onSubmit(date))
        }}
      >
        {message && <p className="confirm-msg">{message}</p>}
        <Field label={t('common.date')}>
          <input type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
        </Field>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button className="btn btn-primary" disabled={busy}>
            {busy ? <Spinner small /> : submitLabel}
          </button>
        </div>
      </form>
    </Modal>
  )
}
