// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { type Vehicle } from '../api'
import { useI18n } from '../i18n'
import { num } from '../format'
import { Icon } from '../components/Icon'
import { OdometerForm } from '../components/Forms'
import { ExportDialog, ImportDialog } from '../components/Transfer'
import { Loading, useLoad } from '../components/ui'
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

export function VehicleDetail() {
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
  const tab: Tab = tabs.includes(asked as Tab) ? (asked as Tab) : (oldTabs[asked] ?? 'status')

  useEffect(() => {
    if (!menu) return
    const close = () => setMenu(false)
    window.addEventListener('click', close)
    return () => window.removeEventListener('click', close)
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
          className="hero-btn right"
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
          <div className="menu" role="menu">
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
            changed()
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
