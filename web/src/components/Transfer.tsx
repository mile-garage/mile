// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Export to CSV and import from other apps (Fuelio, LubeLogger).

import { useState, type FormEvent } from 'react'
import { api, ApiError, type Vehicle } from '../api'
import { useI18n, type Key } from '../i18n'
import { Field, Modal, Spinner, useUI } from './ui'
import { Icon } from './Icon'

/** Download of a ZIP archive of CSV files: url is /api/export or /api/vehicles/{id}/export. */
export function ExportDialog({ url, title, onClose }: { url: string; title: string; onClose: () => void }) {
  const { t, locale } = useI18n()
  const [format, setFormat] = useState<'spreadsheet' | 'csv'>('spreadsheet')
  const [files, setFiles] = useState(false)
  const href = `${url}?format=${format}&lang=${locale}${files ? '&files=1' : ''}`
  return (
    <Modal title={title} onClose={onClose} narrow>
      <p className="muted">{t('export.text')}</p>
      <div className="form-grid">
        <Field label={t('export.format')} hint={t(format === 'spreadsheet' ? 'export.spreadsheetHint' : 'export.csvHint')} wide>
          <select value={format} onChange={(e) => setFormat(e.target.value as 'spreadsheet' | 'csv')}>
            <option value="spreadsheet">{t('export.spreadsheet')}</option>
            <option value="csv">{t('export.csv')}</option>
          </select>
        </Field>
        <label className="check field-wide">
          <input type="checkbox" checked={files} onChange={(e) => setFiles(e.target.checked)} />
          {t('export.files')}
        </label>
      </div>
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t('common.cancel')}
        </button>
        {/* A plain link: the browser downloads the archive while it is written. */}
        <a className="btn btn-primary" href={href} download onClick={() => setTimeout(onClose, 300)}>
          <Icon name="download" size={16} />
          {t('export.download')}
        </a>
      </div>
    </Modal>
  )
}

type Source = 'fuelio' | 'lubelogger'
const kinds = ['gas', 'service', 'repair', 'upgrade', 'tax', 'odometer'] as const
const unitOptions = ['km', 'mi_us', 'mi_imp'] as const

interface ImportResult {
  refuels: number
  expenses: number
  odometer: number
  duplicates: number
  invalid: { line: number; code: string; message: string }[]
  invalid_count: number
  warnings: { code: string; n?: number }[]
}

export function ImportDialog({ vehicle, onClose, onDone }: { vehicle: Vehicle; onClose: () => void; onDone: () => void }) {
  const { t, errorText } = useI18n()
  const { toast, fail } = useUI()
  const [source, setSource] = useState<Source>('fuelio')
  const [kind, setKind] = useState<(typeof kinds)[number]>('gas')
  const [units, setUnits] = useState<(typeof unitOptions)[number]>('km')
  const [dateOrder, setDateOrder] = useState('')
  const [askDate, setAskDate] = useState(false)
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<ImportResult | null>(null)
  const [busy, setBusy] = useState(false)

  // Any change asks for a new check of the file.
  const change =
    <T,>(set: (v: T) => void) =>
    (v: T) => {
      set(v)
      setPreview(null)
    }

  const send = async (dryRun: boolean) => {
    const fd = new FormData()
    fd.append('source', source)
    if (source === 'lubelogger') {
      fd.append('kind', kind)
      fd.append('units', units)
      fd.append('date_order', dateOrder)
    }
    if (dryRun) fd.append('dry_run', '1')
    fd.append('file', file!)
    return api.post<ImportResult>(`/api/vehicles/${vehicle.id}/import`, fd)
  }

  const check = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      setPreview(await send(true))
    } catch (err) {
      if (err instanceof ApiError && err.code === 'import_date_ambiguous') setAskDate(true)
      fail(err)
    }
    setBusy(false)
  }

  const save = async () => {
    setBusy(true)
    try {
      const r = await send(false)
      toast(t('import.done', { n: r.refuels + r.expenses + r.odometer }))
      onDone()
    } catch (err) {
      fail(err)
      setBusy(false)
    }
  }

  const total = preview ? preview.refuels + preview.expenses + preview.odometer : 0
  return (
    <Modal title={t('import.title')} onClose={onClose} narrow>
      <form className="form-grid" onSubmit={check}>
        <Field label={t('import.source')} wide>
          <div className="segmented">
            {(['fuelio', 'lubelogger'] as const).map((s) => (
              <button type="button" key={s} className={source === s ? 'on' : ''} onClick={() => change(setSource)(s)}>
                {s === 'fuelio' ? 'Fuelio' : 'LubeLogger'}
              </button>
            ))}
          </div>
        </Field>
        <p className="muted small field-wide">{t(source === 'fuelio' ? 'import.fuelioHint' : 'import.lubeHint')}</p>
        {source === 'lubelogger' && (
          <>
            <Field label={t('import.kind')} wide>
              <select value={kind} onChange={(e) => change(setKind)(e.target.value as (typeof kinds)[number])}>
                {kinds.map((k) => (
                  <option key={k} value={k}>
                    {t(`import.kind.${k}` as Key)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t('import.units')}>
              <select value={units} onChange={(e) => change(setUnits)(e.target.value as (typeof unitOptions)[number])}>
                {unitOptions.map((u) => (
                  <option key={u} value={u}>
                    {t(`import.units.${u}` as Key)}
                  </option>
                ))}
              </select>
            </Field>
            {(askDate || dateOrder) && (
              <Field label={t('import.dateOrder')}>
                <select value={dateOrder} onChange={(e) => change(setDateOrder)(e.target.value)}>
                  <option value="">{t('import.dateOrder.auto')}</option>
                  <option value="dmy">{t('import.dateOrder.dmy')}</option>
                  <option value="mdy">{t('import.dateOrder.mdy')}</option>
                </select>
              </Field>
            )}
          </>
        )}
        <Field label={t('import.file')} wide>
          <input
            type="file"
            accept={source === 'fuelio' ? '.csv,.zip,text/csv,application/zip' : '.csv,text/csv'}
            onChange={(e) => change(setFile)(e.target.files?.[0] ?? null)}
            required
          />
        </Field>

        {preview && (
          <div className="import-preview field-wide">
            {total > 0 ? (
              <>
                <strong>{t('import.preview')}</strong>
                <ul>
                  {preview.refuels > 0 && <li>{t('import.refuels', { n: preview.refuels })}</li>}
                  {preview.expenses > 0 && <li>{t('import.expenses', { n: preview.expenses })}</li>}
                  {preview.odometer > 0 && <li>{t('import.odometer', { n: preview.odometer })}</li>}
                </ul>
              </>
            ) : (
              <strong>{t('import.nothing')}</strong>
            )}
            {preview.duplicates > 0 && <p className="muted small">{t('import.duplicates', { n: preview.duplicates })}</p>}
            {preview.warnings.map((w) => (
              <p key={w.code} className="muted small">
                {t(`import.warn.${w.code}` as Key, { n: w.n ?? 0 })}
              </p>
            ))}
            {preview.invalid_count > 0 && (
              <div className="import-invalid small">
                {t('import.invalid', { n: preview.invalid_count })}
                <ul>
                  {preview.invalid.map((l) => (
                    <li key={`${l.line}-${l.code}`}>
                      {t('import.line', { n: l.line })}: {errorText(new ApiError(0, l.code, l.message))}
                    </li>
                  ))}
                  {preview.invalid_count > preview.invalid.length && <li>…</li>}
                </ul>
              </div>
            )}
          </div>
        )}

        <div className="modal-actions field-wide">
          <button type="button" className="btn" onClick={onClose}>
            {t('common.cancel')}
          </button>
          {preview ? (
            <button type="button" className="btn btn-primary" onClick={save} disabled={busy || total === 0}>
              {busy ? <Spinner small /> : t('import.submit')}
            </button>
          ) : (
            <button className="btn btn-primary" disabled={busy || !file}>
              {busy ? <Spinner small /> : t('import.check')}
            </button>
          )}
        </div>
      </form>
    </Modal>
  )
}
