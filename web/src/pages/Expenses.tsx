// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// What the vehicles cost: all of them, or one picked with the chips.

import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api, type MonthCost, type Stats, type Vehicle } from '../api'
import { useI18n } from '../i18n'
import { euro, month, num } from '../format'
import { Empty, Loading, useLoad, useUI } from '../components/ui'

const series = ['fuel', 'expenses', 'insurance'] as const

export function Expenses() {
  const { t, locale } = useI18n()
  const { fail } = useUI()
  const [params, setParams] = useSearchParams()
  const vehicles = useLoad<Vehicle[]>('/api/vehicles')
  const [stats, setStats] = useState<Map<number, Stats> | null>(null)
  const active = (vehicles.data ?? []).filter((v) => !v.archived)
  const ids = active.map((v) => v.id).join(',')

  useEffect(() => {
    if (!vehicles.data) return
    let alive = true
    Promise.all(active.map((v) => api.get<Stats>(`/api/vehicles/${v.id}/stats`).then((s) => [v.id, s] as const)))
      .then((all) => alive && setStats(new Map(all)))
      .catch(fail)
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ids, vehicles.data])

  if (!vehicles.data || !stats) return <Loading />
  const picked = Number(params.get('vehicle')) || null
  const shown = active.filter((v) => picked === null || v.id === picked)
  const list = shown.map((v) => stats.get(v.id)).filter((s): s is Stats => !!s)

  // the 12 months are the same for every vehicle: add them up month by month
  const months: MonthCost[] = (list[0]?.last_12_months ?? []).map((m, i) => ({
    month: m.month,
    fuel: list.reduce((s, x) => s + (x.last_12_months[i]?.fuel ?? 0), 0),
    expenses: list.reduce((s, x) => s + (x.last_12_months[i]?.expenses ?? 0), 0),
    insurance: list.reduce((s, x) => s + (x.last_12_months[i]?.insurance ?? 0), 0),
  }))
  const monthTotal = (m: MonthCost) => m.fuel + m.expenses + m.insurance
  const year = months.reduce((s, m) => s + monthTotal(m), 0)
  const current = months.at(-1)
  const allTime = list.reduce((s, x) => s + x.total, 0)
  const cats = new Map<string, number>()
  for (const x of list) for (const [c, v] of Object.entries(x.by_category)) cats.set(c, (cats.get(c) ?? 0) + v)
  const catList = [...cats.entries()].sort((a, b) => b[1] - a[1])
  const maxCat = Math.max(1, ...catList.map((c) => c[1]))
  const maxMonth = Math.max(1, ...months.map(monthTotal))
  const seriesLabel = { fuel: t('cat.fuel'), expenses: t('tab.expenses'), insurance: t('cat.insurance') }

  return (
    <div className="page">
      <div className="page-head">
        <div className="page-title">
          <h1>{t('expenses.title')}</h1>
        </div>
      </div>

      {active.length > 1 && (
        <div className="chips scroll-x" role="group" aria-label={t('nav.vehicles')} style={{ marginBottom: 18 }}>
          <button className="chip" aria-pressed={picked === null} onClick={() => setParams({}, { replace: true })}>
            {t('home.all')}
          </button>
          {active.map((v) => (
            <button key={v.id} className="chip" aria-pressed={picked === v.id} onClick={() => setParams({ vehicle: String(v.id) }, { replace: true })}>
              {v.name}
            </button>
          ))}
        </div>
      )}

      {allTime === 0 ? (
        <Empty>{t('expenses.empty')}</Empty>
      ) : (
        <>
          <div className="tiles">
            <div className="tile">
              <span className="tile-label">{t('expenses.thisMonth')}</span>
              <span className="tile-value">{euro(current ? monthTotal(current) : 0, locale)}</span>
            </div>
            <div className="tile">
              <span className="tile-label">{t('expenses.last12')}</span>
              <span className="tile-value">{euro(year, locale)}</span>
            </div>
            <div className="tile">
              <span className="tile-label">{t('expenses.allTime')}</span>
              <span className="tile-value">{euro(allTime, locale)}</span>
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
              {months.map((m) => {
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
                  {months.map((m) => (
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

          {catList.length > 0 && (
            <section className="card chart-card">
              <h3>{t('summary.byCategory')}</h3>
              <div className="hbars">
                {catList.map(([c, v]) => (
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

          {picked === null && shown.length > 1 && (
            <>
              <h2 className="section-title">{t('expenses.byVehicle')}</h2>
              <div className="card list">
                {shown.map((v) => {
                  const s = stats.get(v.id)
                  const last12 = (s?.last_12_months ?? []).reduce((a, m) => a + monthTotal(m), 0)
                  return (
                    <Link key={v.id} className="row clickable" to={`/vehicles/${v.id}?tab=log`}>
                      <div className="row-main">
                        <div className="row-title">{v.name}</div>
                        <div className="row-sub">
                          {s?.cents_per_km != null ? `${num(s.cents_per_km / 100, locale, 2)} €/km · ` : ''}
                          {t('expenses.last12').toLowerCase()}
                        </div>
                      </div>
                      <span className="row-amount">{euro(last12, locale)}</span>
                    </Link>
                  )
                })}
              </div>
            </>
          )}
        </>
      )}
    </div>
  )
}
