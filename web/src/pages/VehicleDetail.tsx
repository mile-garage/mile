import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { type Vehicle } from '../api'
import { PageHead } from '../App'
import { useI18n } from '../i18n'
import { num } from '../format'
import { Icon } from '../components/Icon'
import { OdometerForm } from '../components/Forms'
import { Loading, useLoad } from '../components/ui'
import { VehicleCover } from './Vehicles'
import { DeadlinesTab, ExpensesTab, FuelTab, InsuranceTab, PhotosTab, SummaryTab } from './VehicleTabs'

const tabs = ['deadlines', 'expenses', 'fuel', 'insurance', 'photos', 'summary'] as const
type Tab = (typeof tabs)[number]

export function VehicleDetail() {
  const { id } = useParams()
  const { t, locale } = useI18n()
  const [params, setParams] = useSearchParams()
  const vehicle = useLoad<Vehicle>(`/api/vehicles/${id}`)
  const [kmForm, setKmForm] = useState(false)
  // bumped after every change, so that the tabs reload their data
  const [version, setVersion] = useState(0)
  const tab: Tab = tabs.includes(params.get('tab') as Tab) ? (params.get('tab') as Tab) : 'deadlines'

  const v = vehicle.data
  if (!v) return <Loading />
  const canEdit = v.role !== 'viewer'
  const changed = () => {
    vehicle.reload()
    setVersion((x) => x + 1)
  }
  const props = { vehicle: v, canEdit, version, changed }

  const back = (
    <Link className="btn-icon back" to="/vehicles" aria-label={t('common.back')}>
      <Icon name="back" />
    </Link>
  )

  return (
    <div className="page">
      <PageHead title={v.name} back={back}>
        {canEdit && (
          <Link className="btn" to={`/vehicles/${v.id}/edit`}>
            <Icon name="edit" size={16} />
            <span className="hide-sm">{t('common.edit')}</span>
          </Link>
        )}
      </PageHead>

      <div className="vehicle-hero card">
        <VehicleCover v={v} className="hero-cover" />
        <div className="vehicle-hero-info">
          {v.plate && <div className="plate">{v.plate}</div>}
          <div className="muted">
            {[v.make, v.model].filter(Boolean).join(' ')} · {t(`fuel.${v.fuel_type}`)}
          </div>
          <div className="vehicle-km">
            <Icon name="gauge" size={18} />
            {v.current_km !== null ? t('vehicle.currentKm', { km: num(v.current_km, locale) }) : '—'}
            {canEdit && (
              <button className="btn btn-sm" onClick={() => setKmForm(true)}>
                {t('vehicle.updateKm')}
              </button>
            )}
          </div>
          {v.archived && <span className="badge">{t('vehicles.archived')}</span>}
        </div>
      </div>

      <nav className="tabs" role="tablist">
        {tabs.map((x) => (
          <button key={x} role="tab" aria-selected={tab === x} className={tab === x ? 'on' : ''} onClick={() => setParams({ tab: x }, { replace: true })}>
            {t(`tab.${x}`)}
          </button>
        ))}
      </nav>

      {tab === 'deadlines' && <DeadlinesTab {...props} goTo={(x: string) => setParams({ tab: x }, { replace: true })} />}
      {tab === 'expenses' && <ExpensesTab {...props} />}
      {tab === 'fuel' && <FuelTab {...props} />}
      {tab === 'insurance' && <InsuranceTab {...props} />}
      {tab === 'photos' && <PhotosTab {...props} />}
      {tab === 'summary' && <SummaryTab {...props} />}

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
