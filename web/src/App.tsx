// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, useContext, useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { NavLink, Outlet, useMatch } from 'react-router'
import { api, ApiError, takeParam, type Status, type User, type Vehicle } from './api'
import { I18nProvider, useI18n } from './i18n'
import { Field, Spinner, UIProvider, useLoad, useUI } from './components/ui'
import { Icon } from './components/Icon'
import { AddProvider, useAdd } from './components/Add'
import { Plate } from './pages/Vehicles'

interface Session {
  user: User
  setUser: (u: User) => void
  logout: () => Promise<void>
  /** Name of the login provider, '' when the login with OpenID Connect is off. */
  sso: string
}

const SessionCtx = createContext<Session | null>(null)

export function useSession(): Session {
  const c = useContext(SessionCtx)
  if (!c) throw new Error('SessionCtx missing')
  return c
}

export function Root() {
  return (
    <I18nProvider initial="">
      <UIProvider>
        <AuthGate />
      </UIProvider>
    </I18nProvider>
  )
}

type Phase = { k: 'loading' } | { k: 'offline' } | { k: 'setup' } | { k: 'login' } | { k: 'in'; user: User }

function AuthGate() {
  const { setLocale } = useI18n()
  const [phase, setPhase] = useState<Phase>({ k: 'loading' })
  const [sso, setSso] = useState('')

  const enter = (u: User) => {
    setLocale(u.locale)
    setPhase({ k: 'in', user: u })
  }

  const load = () => {
    setPhase({ k: 'loading' })
    const status = api.get<Status>('/api/status').catch(() => null)
    status.then((s) => setSso(s?.sso ?? ''))
    api
      .get<User>('/api/me')
      .then(enter)
      .catch((e) => {
        // Opened from the home screen without a connection: the interface comes from the service worker.
        if (e instanceof ApiError && e.code === 'offline') setPhase({ k: 'offline' })
        else status.then((s) => setPhase({ k: s?.setup_required ? 'setup' : 'login' }))
      })
  }

  useEffect(() => {
    load()
    const onUnauth = () => setPhase({ k: 'login' })
    window.addEventListener('mile:unauthorized', onUnauth)
    return () => window.removeEventListener('mile:unauthorized', onUnauth)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  switch (phase.k) {
    case 'loading':
      return (
        <div className="center-page">
          <Spinner />
        </div>
      )
    case 'offline':
      return <Offline onRetry={load} />
    case 'setup':
      return <AuthForm setup sso={sso} onDone={enter} />
    case 'login':
      return <AuthForm sso={sso} onDone={enter} />
  }
  const session: Session = {
    sso,
    user: phase.user,
    setUser: enter,
    logout: async () => {
      await api.post('/api/logout')
      setPhase({ k: 'login' })
    },
  }
  return (
    <SessionCtx.Provider value={session}>
      <Layout />
    </SessionCtx.Provider>
  )
}

function Layout() {
  return (
    <AddProvider>
      <div className="app">
        <Sidebar />
        <main className="content">
          <Outlet />
        </main>
        <BottomNav />
      </div>
    </AddProvider>
  )
}

/** Desktop and tablet: sections, the vehicles and the "+" button. */
function Sidebar() {
  const { t } = useI18n()
  const { openAdd } = useAdd()
  const { data } = useLoad<Vehicle[]>('/api/vehicles')
  const vehicles = (data ?? []).filter((v) => !v.archived)
  return (
    <aside className="sidebar">
      <NavLink to="/" className="brand" aria-label="MILE">
        <img src="/logo-m.png" alt="" width="34" height="32" />
        <span>MILE</span>
      </NavLink>
      <nav className="side-nav" aria-label="MILE">
        <NavLink to="/" end>
          <Icon name="calendar" />
          {t('nav.deadlines')}
        </NavLink>
        <NavLink to="/vehicles" end>
          <Icon name="car" />
          {t('nav.vehicles')}
        </NavLink>
        <NavLink to="/expenses">
          <Icon name="chart" />
          {t('nav.expenses')}
        </NavLink>
        <NavLink to="/settings">
          <Icon name="settings" />
          {t('nav.settings')}
        </NavLink>
      </nav>
      {vehicles.length > 0 && (
        <div className="side-vehicles">
          <div className="side-label">{t('nav.vehicles')}</div>
          {vehicles.map((v) => (
            <NavLink key={v.id} to={`/vehicles/${v.id}`}>
              <span>{v.name}</span>
              {v.plate && <Plate plate={v.plate} small />}
            </NavLink>
          ))}
        </div>
      )}
      <button type="button" className="btn btn-cta btn-lg" onClick={() => openAdd()}>
        <Icon name="plus" />
        {t('nav.add')}
      </button>
    </aside>
  )
}

/** Phones: four sections and the "+" button in the middle. */
function BottomNav() {
  const { t } = useI18n()
  const { openAdd } = useAdd()
  // On a vehicle's page the "+" logs for that vehicle.
  const vehicleId = Number(useMatch('/vehicles/:id/*')?.params.id) || undefined
  return (
    <nav className="bottomnav" aria-label="MILE">
      <NavLink to="/" end>
        <Icon name="calendar" size={24} />
        <span>{t('nav.deadlines')}</span>
      </NavLink>
      <NavLink to="/vehicles">
        <Icon name="car" size={24} />
        <span>{t('nav.vehicles')}</span>
      </NavLink>
      <button type="button" className="fab" onClick={() => openAdd(vehicleId)} aria-label={t('nav.add')}>
        <span className="fab-circle">
          <Icon name="plus" size={26} />
        </span>
      </button>
      <NavLink to="/expenses">
        <Icon name="chart" size={24} />
        <span>{t('nav.expenses')}</span>
      </NavLink>
      <NavLink to="/settings">
        <Icon name="settings" size={24} />
        <span>{t('nav.settings')}</span>
      </NavLink>
    </nav>
  )
}

function Offline({ onRetry }: { onRetry: () => void }) {
  const { t } = useI18n()
  useEffect(() => {
    window.addEventListener('online', onRetry)
    return () => window.removeEventListener('online', onRetry)
  }, [onRetry])
  return (
    <div className="center-page">
      <div className="auth-card">
        <div className="auth-brand">
          <img src="/logo-m.png" alt="" width="59" height="56" />
          <div>
            <h1>MILE</h1>
            <p className="muted">{t('err.offline')}</p>
          </div>
        </div>
        <button className="btn btn-primary btn-block" onClick={onRetry}>
          {t('common.retry')}
        </button>
      </div>
    </div>
  )
}

function AuthForm({ setup, sso, onDone }: { setup?: boolean; sso: string; onDone: (u: User) => void }) {
  const { t, locale, errorText } = useI18n()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // The login with the provider comes back here with ?sso_error=<code> when it fails.
  useEffect(() => {
    const code = takeParam('sso_error')
    if (code) setError(errorText(new ApiError(0, code, t('err.sso_failed'))))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const u = setup
        ? await api.post<User>('/api/setup', { username, password, display_name: displayName, locale })
        : await api.post<User>('/api/login', { username, password })
      onDone(u)
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  return (
    <div className="center-page">
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-brand">
          <img src="/logo-m.png" alt="" width="59" height="56" />
          <div>
            <h1>{setup ? t('auth.setup.title') : 'MILE'}</h1>
            <p className="muted">{setup ? t('auth.setup.text') : t('app.tagline')}</p>
          </div>
        </div>
        <Field label={t('auth.username')}>
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus autoComplete="username" autoCapitalize="none" required />
        </Field>
        {setup && (
          <Field label={t('auth.displayName')}>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} autoComplete="name" />
          </Field>
        )}
        <Field label={t('auth.password')} hint={setup ? t('auth.passwordHint') : undefined}>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={setup ? 'new-password' : 'current-password'}
            minLength={setup ? 8 : undefined}
            required
          />
        </Field>
        {error && <div className="form-error">{error}</div>}
        <button className="btn btn-primary btn-block" disabled={busy}>
          {busy ? <Spinner small /> : setup ? t('auth.setup.submit') : t('auth.login')}
        </button>
        {sso && (
          <>
            <div className="auth-or">{t('auth.or')}</div>
            <a className="btn btn-block" href="/auth/oidc/login">
              {t('auth.sso', { name: sso })}
            </a>
          </>
        )}
      </form>
    </div>
  )
}

/** Page header with title, optional back link and actions. */
export function PageHead({ title, sub, back, children }: { title: ReactNode; sub?: ReactNode; back?: ReactNode; children?: ReactNode }) {
  return (
    <div className="page-head">
      {back}
      <div className="page-title">
        <h1>{title}</h1>
        {sub && <div className="muted">{sub}</div>}
      </div>
      {children && <div className="page-actions">{children}</div>}
    </div>
  )
}

export function useLogout() {
  const { logout } = useSession()
  const { confirm } = useUI()
  const { t } = useI18n()
  return async () => {
    if (await confirm({ title: t('nav.logout'), message: t('auth.logoutConfirm'), confirmLabel: t('nav.logout') })) await logout()
  }
}
