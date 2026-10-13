// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// The "+" button: one place to log a refuel, a service, an expense, the km or a reminder.

import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { api, type Vehicle } from '../api'
import { useI18n } from '../i18n'
import { plateText } from '../format'
import { ExpenseForm, OdometerForm, RefuelForm, ReminderForm } from './Forms'
import { Icon } from './Icon'
import { Loading, Modal, notifyChanged, useUI } from './ui'

type Action = 'refuel' | 'service' | 'expense' | 'km' | 'reminder'

interface AddCtx {
  /** Opens the sheet, with the vehicle already chosen when given. */
  openAdd: (vehicleId?: number) => void
}

const Ctx = createContext<AddCtx | null>(null)

export function useAdd(): AddCtx {
  const c = useContext(Ctx)
  if (!c) throw new Error('AddProvider missing')
  return c
}

export function AddProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState<{ vehicleId?: number } | null>(null)
  return (
    <Ctx.Provider value={{ openAdd: (vehicleId) => setOpen({ vehicleId }) }}>
      {children}
      {open && <AddSheet initial={open.vehicleId} onClose={() => setOpen(null)} />}
    </Ctx.Provider>
  )
}

function AddSheet({ initial, onClose }: { initial?: number; onClose: () => void }) {
  const { t } = useI18n()
  const { fail } = useUI()
  const [vehicles, setVehicles] = useState<Vehicle[] | null>(null)
  const [vid, setVid] = useState<number | undefined>(initial)
  const [action, setAction] = useState<Action | null>(null)

  useEffect(() => {
    api
      .get<Vehicle[]>('/api/vehicles')
      .then((all) => {
        const usable = all.filter((v) => !v.archived && v.role !== 'viewer')
        setVehicles(usable)
        setVid((x) => (usable.some((v) => v.id === x) ? x : usable[0]?.id))
      })
      .catch((e) => {
        fail(e)
        onClose()
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const saved = () => {
    notifyChanged()
    onClose()
  }
  const vehicle = vehicles?.find((v) => v.id === vid)
  const back = () => setAction(null)

  if (vehicle && action === 'refuel') return <RefuelForm vehicle={vehicle} onClose={back} onSaved={saved} />
  if (vehicle && (action === 'service' || action === 'expense')) return <ExpenseForm vehicle={vehicle} mode={action === 'service' ? 'service' : 'other'} onClose={back} onSaved={saved} />
  if (vehicle && action === 'km') return <OdometerForm vehicle={vehicle} onClose={back} onSaved={saved} />
  if (action === 'reminder' && vehicles) return <ReminderForm vehicles={vehicles} vehicleId={vid} onClose={back} onSaved={saved} />

  const tile = (a: Action, icon: string, label: string, main?: boolean) => (
    <button type="button" className={`add-tile${main ? ' main' : ''}`} onClick={() => setAction(a)}>
      <Icon name={icon} size={24} />
      {label}
    </button>
  )

  return (
    <Modal title={t('add.title')} onClose={onClose}>
      {!vehicles ? (
        <Loading />
      ) : vehicles.length === 0 ? (
        <div className="stack">
          <p className="muted">{t('add.noVehicles')}</p>
          <Link className="btn btn-primary btn-lg" to="/vehicles/new" onClick={onClose}>
            <Icon name="plus" size={18} />
            {t('vehicles.first')}
          </Link>
          <button type="button" className="add-tile" onClick={() => setAction('reminder')}>
            <Icon name="bell" />
            {t('add.reminder')}
          </button>
        </div>
      ) : (
        <>
          {vehicles.length > 1 && (
            <div className="add-vehicles" role="radiogroup" aria-label={t('reminder.vehicle')}>
              {vehicles.map((v) => (
                <button type="button" key={v.id} role="radio" aria-checked={v.id === vid} className="add-vehicle" onClick={() => setVid(v.id)}>
                  <strong>{v.name}</strong>
                  {v.plate && <span>{plateText(v.plate)}</span>}
                </button>
              ))}
            </div>
          )}
          <div className="add-grid">
            {tile('refuel', 'fuel', t('add.refuel'), true)}
            {tile('service', 'wrench', t('add.service'), true)}
            {tile('expense', 'receipt', t('add.expense'))}
            {tile('km', 'gauge', t('add.km'))}
            {tile('reminder', 'bell', t('add.reminder'))}
          </div>
        </>
      )}
    </Modal>
  )
}
