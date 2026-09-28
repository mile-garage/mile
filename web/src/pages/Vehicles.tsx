// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { Link } from 'react-router'
import { fileUrl, type Vehicle } from '../api'
import { PageHead } from '../App'
import { useI18n } from '../i18n'
import { num } from '../format'
import { Icon } from '../components/Icon'
import { Empty, Loading, useLoad } from '../components/ui'

export function Vehicles() {
  const { t } = useI18n()
  const { data } = useLoad<Vehicle[]>('/api/vehicles')
  if (!data) return <Loading />
  const active = data.filter((v) => !v.archived)
  const archived = data.filter((v) => v.archived)

  return (
    <div className="page">
      <PageHead title={t('vehicles.title')}>
        <Link className="btn btn-primary" to="/vehicles/new">
          <Icon name="plus" size={16} />
          {t('vehicles.add')}
        </Link>
      </PageHead>
      {active.length === 0 && <Empty>{t('vehicles.empty')}</Empty>}
      <div className="vehicle-grid">
        {active.map((v) => (
          <VehicleCard key={v.id} v={v} />
        ))}
      </div>
      {archived.length > 0 && (
        <>
          <h2 className="section-title">{t('vehicles.archived')}</h2>
          <div className="vehicle-grid archived">
            {archived.map((v) => (
              <VehicleCard key={v.id} v={v} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}

export function VehicleCover({ v, className = 'cover' }: { v: Vehicle; className?: string }) {
  return v.cover_id ? (
    <img className={className} src={fileUrl(v.cover_id)} alt="" loading="lazy" />
  ) : (
    <div className={`${className} cover-empty`}>
      <Icon name="car" size={40} />
    </div>
  )
}

function VehicleCard({ v }: { v: Vehicle }) {
  const { t, locale } = useI18n()
  return (
    <Link to={`/vehicles/${v.id}`} className="vehicle-card">
      <VehicleCover v={v} />
      <div className="vehicle-card-body">
        <div className="vehicle-card-name">{v.name}</div>
        <div className="muted">
          {[v.plate, v.current_km !== null ? t('vehicle.currentKm', { km: num(v.current_km, locale) }) : ''].filter(Boolean).join(' · ')}
        </div>
        {v.role !== 'owner' && <span className="badge">{t('vehicles.shared')}</span>}
      </div>
    </Link>
  )
}
