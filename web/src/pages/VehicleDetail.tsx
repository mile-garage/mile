// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { type Vehicle } from '../api'
import { useI18n } from '../i18n'
import { num } from '../format'
import { Icon } from '../components/Icon'
import { OdometerForm } from '../components/Forms'
import { ExportDialog, ImportDialog } from '../components/Transfer'
import { Loading, notifyChanged, useLoad } from '../components/ui'
import { Plate, VehicleCover } from './Vehicles'
import { DocsTab, LogTab, StatusTab } from './VehicleTabs'
import { TyresTab } from './Tyres'

const tabs = ['status', 'log', 'tyres', 'docs'] as const
type Tab = (typeof tabs)[number]

/** The tabs before the redesign, still found in bookmarks and links. */
const oldTabs: Record<string, Tab> = {
  deadlines: 'status',
  summary: 'status',
  expenses: 'log',
  fuel: 'log',
  insurance: 'docs',
  photos: 'docs',
}

/** A new vehicle starts from a clean page: no menu or form left open from the previous one. */
export function VehicleDetailPage() {
  const { id } = useParams()
  return <VehicleDetail key={id} />
}

function VehicleDetail() {
  const { id } = useParams()
  const { t, locale } = useI18n()
  const [params, setParams] = useSearchParams()
  const vehicle = useLoad<Vehicle>(`/api/vehicles/${id}`)
  const [kmForm, setKmForm] = useState(false)
  const [menu, setMenu] = useState(false)
  const [dialog, setDialog] = useState<'export' | 'import' | null>(null)
  // bumped after every change, so that the tabs reload their data
  const [version, setVersion] = useState(0)
  const asked = params.get('tab') ?? ''
  const tab: Tab = tabs.includes(asked as Tab) ? (asked as Tab) : Object.hasOwn(oldTabs, asked) ? oldTabs[asked] : 'status'

  const menuRef = useRef<HTMLDivElement>(null)
  const menuButton = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!menu) return
    const items = () => [...(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    items()[0]?.focus()
    const close = () => setMenu(false)
    // keyboard: Esc closes and gives the focus back, the arrows move between items
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setMenu(false)
        menuButton.current?.focus()
      } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        const all = items()
        const i = all.indexOf(document.activeElement as HTMLElement)
        all[(i + (e.key === 'ArrowDown' ? 1 : all.length - 1)) % all.length]?.focus()
      }
    }
    window.addEventListener('click', close)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('keydown', onKey)
    }
  }, [menu])

  const v = vehicle.data
  if (!v) return <Loading />
  const canEdit = v.role !== 'viewer'
  const changed = () => {
    vehicle.reload()
    setVersion((x) => x + 1)
  }
  const props = { vehicle: v, canEdit, version, changed }

  return (
    <div className="page">
      <div className="vehicle-hero">
        <VehicleCover v={v} className="hero-cover" />
        <Link className="hero-btn left" to="/vehicles" aria-label={t('common.back')}>
          <Icon name="back" size={22} />
        </Link>
        <button
          ref={menuButton}
          className="hero-btn right"
          aria-haspopup="menu"
          aria-label={t('vehicle.more')}
          aria-expanded={menu}
          onClick={(e) => {
            e.stopPropagation()
            setMenu(!menu)
          }}
        >
          <Icon name="more" size={22} />
        </button>
        {menu && (
          <div className="menu" role="menu" ref={menuRef}>
            {canEdit && (
              <Link role="menuitem" to={`/vehicles/${v.id}/edit`}>
                <Icon name="edit" size={18} />
                {t('vehicle.edit')}
              </Link>
            )}
            {canEdit && (
              <button role="menuitem" onClick={() => setDialog('import')}>
                <Icon name="download" size={18} />
                {t('transfer.import')}
              </button>
            )}
            <button role="menuitem" onClick={() => setDialog('export')}>
              <Icon name="upload" size={18} />
              {t('transfer.export')}
            </button>
          </div>
        )}
      </div>

      <div className="vehicle-sheet">
        <div className="vehicle-head">
          <div>
            <h1>
              {v.name}
              {v.archived && <span className="badge">{t('vehicles.archived')}</span>}
            </h1>
            <div className="muted">{[[v.make, v.model].filter(Boolean).join(' '), t(`fuel.${v.fuel_type}`)].filter(Boolean).join(' · ')}</div>
          </div>
          {v.plate && <Plate plate={v.plate} />}
        </div>

        <div className="card km-card">
          <Icon name="gauge" size={26} />
          <div style={{ flex: 1 }}>
            <div className="km-value">{v.current_km !== null ? `${num(v.current_km, locale)} km` : '—'}</div>
            <div className="muted small">{t('vehicle.kmNow')}</div>
          </div>
          {canEdit && (
            <button className="btn" onClick={() => setKmForm(true)}>
              {t('vehicle.updateKm')}
            </button>
          )}
        </div>

        <nav className="segmented tabs" role="tablist">
          {tabs.map((x) => (
            <button key={x} role="tab" aria-selected={tab === x} className={tab === x ? 'on' : ''} onClick={() => setParams({ tab: x }, { replace: true })}>
              {t(x === 'status' ? 'tab.status' : x === 'log' ? 'tab.log' : x === 'tyres' ? 'tab.tyres' : 'tab.docs')}
            </button>
          ))}
        </nav>

        {tab === 'status' && <StatusTab {...props} />}
        {tab === 'log' && <LogTab {...props} />}
        {tab === 'tyres' && <TyresTab {...props} />}
        {tab === 'docs' && <DocsTab {...props} />}
      </div>

      {dialog === 'export' && <ExportDialog url={`/api/vehicles/${v.id}/export`} title={t('export.title')} onClose={() => setDialog(null)} />}
      {dialog === 'import' && (
        <ImportDialog
          vehicle={v}
          onClose={() => setDialog(null)}
          onDone={() => {
            setDialog(null)
            notifyChanged() // the import can change the km shown elsewhere
          }}
        />
      )}
      {kmForm && (
        <OdometerForm
          vehicle={v}
          onClose={() => setKmForm(false)}
          onSaved={() => {
            setKmForm(false)
            changed()
          }}
        />
      )}
    </div>
  )
}
