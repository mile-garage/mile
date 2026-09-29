// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, useContext, useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { NavLink, Outlet } from 'react-router'
import { api, ApiError, takeParam, type Status, type User } from './api'
import { I18nProvider, useI18n } from './i18n'
import { Field, Spinner, UIProvider, useUI } from './components/ui'
import { Icon } from './components/Icon'

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

type Phase = { k: 'loading' } | { k: 'setup' } | { k: 'login' } | { k: 'in'; user: User }

function AuthGate() {
  const { setLocale } = useI18n()
  const [phase, setPhase] = useState<Phase>({ k: 'loading' })
  const [sso, setSso] = useState('')

  const enter = (u: User) => {
    setLocale(u.locale)
    setPhase({ k: 'in', user: u })
  }

  useEffect(() => {
    const status = api.get<Status>('/api/status').catch(() => null)
    status.then((s) => setSso(s?.sso ?? ''))
    api
      .get<User>('/api/me')
      .then(enter)
      .catch(() => status.then((s) => setPhase({ k: s?.setup_required ? 'setup' : 'login' })))
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
  const { t } = useI18n()
  const nav = (
    <>
      <NavLink to="/" end>
        <Icon name="calendar" />
        <span>{t('nav.deadlines')}</span>
      </NavLink>
      <NavLink to="/vehicles">
        <Icon name="car" />
        <span>{t('nav.vehicles')}</span>
      </NavLink>
      <NavLink to="/settings">
        <Icon name="settings" />
        <span>{t('nav.settings')}</span>
      </NavLink>
    </>
  )
  return (
    <div className="app">
      <header className="topbar">
        <NavLink to="/" className="brand" aria-label="MILE">
          <img src="/logo-m.png" alt="" width="42" height="40" />
          <span>MILE</span>
        </NavLink>
        <nav className="topnav">{nav}</nav>
      </header>
      <main className="content">
        <Outlet />
      </main>
      <nav className="bottomnav">{nav}</nav>
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
