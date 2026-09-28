import { useEffect, useState, type FormEvent } from 'react'
import { api, type Locale, type User } from '../api'
import { PageHead, useLogout, useSession } from '../App'
import { useI18n } from '../i18n'
import { date } from '../format'
import { Icon } from '../components/Icon'
import { Field, Modal, Spinner, useLoad, useUI } from '../components/ui'

type Theme = 'light' | 'dark'

export function Settings() {
  const { t } = useI18n()
  const { user } = useSession()
  const logout = useLogout()
  return (
    <div className="page">
      <PageHead title={t('nav.settings')}>
        <button className="btn" onClick={logout}>
          <Icon name="logout" size={16} />
          {t('nav.logout')}
        </button>
      </PageHead>
      <Profile />
      <Calendar />
      <Password />
      {user.is_admin && <Users />}
    </div>
  )
}

function Profile() {
  const { t, setLocale } = useI18n()
  const { user, setUser } = useSession()
  const { fail, toast } = useUI()
  const [name, setName] = useState(user.display_name)
  const [theme, setTheme] = useState<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light')

  const saveLocale = async (l: '' | Locale) => {
    try {
      const u = await api.put<User>('/api/me', { display_name: name, locale: l })
      setUser(u)
      setLocale(l)
    } catch (e) {
      fail(e)
    }
  }

  const saveName = async (e: FormEvent) => {
    e.preventDefault()
    try {
      setUser(await api.put<User>('/api/me', { display_name: name, locale: user.locale }))
      toast(t('common.saved'))
    } catch (err) {
      fail(err)
    }
  }

  const chooseTheme = (x: Theme) => {
    document.documentElement.dataset.theme = x
    try {
      localStorage.setItem('mile-theme', x)
    } catch {
      /* storage unavailable: the theme lasts for this session only */
    }
    setTheme(x)
  }

  return (
    <section className="card form-card">
      <h2>{t('settings.profile')}</h2>
      <form className="form-grid" onSubmit={saveName}>
        <Field label={t('auth.username')}>
          <input value={user.username} disabled />
        </Field>
        <Field label={t('auth.displayName')}>
          <div className="inline-form">
            <input value={name} onChange={(e) => setName(e.target.value)} />
            <button className="btn">{t('common.save')}</button>
          </div>
        </Field>
        <Field label={t('settings.language')}>
          <select value={user.locale} onChange={(e) => saveLocale(e.target.value as '' | Locale)}>
            <option value="">{t('settings.languageAuto')}</option>
            <option value="it">Italiano</option>
            <option value="en">English</option>
          </select>
        </Field>
        <Field label={t('settings.theme')}>
          <div className="segmented">
            {(['light', 'dark'] as Theme[]).map((x) => (
              <button type="button" key={x} className={theme === x ? 'on' : ''} onClick={() => chooseTheme(x)} aria-pressed={theme === x}>
                {t(`settings.${x}`)}
              </button>
            ))}
          </div>
        </Field>
      </form>
    </section>
  )
}

function Calendar() {
  const { t } = useI18n()
  const { toast, fail, confirm } = useUI()
  const [path, setPath] = useState<string | null>(null)
  useEffect(() => {
    api
      .get<{ path: string }>('/api/me/calendar')
      .then((r) => setPath(r.path))
      .catch(fail)
  }, [fail])
  if (!path) return null
  const url = location.origin + path
  // webcal:// opens the subscription dialog on iPhone and macOS
  const webcal = url.replace(/^https?:/, 'webcal:')

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      toast(t('settings.calendarCopied'))
    } catch {
      prompt(t('settings.calendarCopy'), url)
    }
  }
  const regenerate = async () => {
    if (!(await confirm({ title: t('settings.calendarReset'), message: t('settings.calendarResetText'), danger: true }))) return
    try {
      setPath((await api.post<{ path: string }>('/api/me/calendar')).path)
    } catch (e) {
      fail(e)
    }
  }

  return (
    <section className="card form-card">
      <h2>
        <Icon name="calendar" size={18} /> {t('settings.calendar')}
      </h2>
      <p className="muted">{t('settings.calendarText')}</p>
      <input className="mono" readOnly value={url} onFocus={(e) => e.target.select()} />
      <p className="muted small">{t('settings.calendarWarn')}</p>
      <div className="row-actions">
        <button className="btn btn-primary" onClick={copy}>
          {t('settings.calendarCopy')}
        </button>
        <a className="btn" href={webcal}>
          webcal://
        </a>
        <button className="btn btn-danger-ghost" onClick={regenerate}>
          {t('settings.calendarReset')}
        </button>
      </div>
    </section>
  )
}

function Password() {
  const { t } = useI18n()
  const { toast, fail } = useUI()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.post('/api/me/password', { current, new: next })
      toast(t('settings.passwordChanged'))
      setCurrent('')
      setNext('')
    } catch (err) {
      fail(err)
    }
    setBusy(false)
  }
  return (
    <section className="card form-card">
      <h2>{t('settings.password')}</h2>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('settings.currentPassword')}>
          <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
        </Field>
        <Field label={t('settings.newPassword')} hint={t('auth.passwordHint')}>
          <input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={8} required />
        </Field>
        <div className="field-wide">
          <button className="btn" disabled={busy}>
            {busy ? <Spinner small /> : t('common.save')}
          </button>
        </div>
      </form>
    </section>
  )
}

function Users() {
  const { t, locale } = useI18n()
  const { user } = useSession()
  const { confirm, fail, toast } = useUI()
  const { data, reload } = useLoad<User[]>('/api/users')
  const [creating, setCreating] = useState(false)
  const [resetting, setResetting] = useState<User | null>(null)

  const del = async (u: User) => {
    if (!(await confirm({ title: t('common.delete'), message: t('settings.deleteUserText', { name: u.username }), confirmLabel: t('common.delete'), danger: true })))
      return
    try {
      await api.del(`/api/users/${u.id}`)
      toast(t('common.deleted'))
      reload()
    } catch (e) {
      fail(e)
    }
  }

  return (
    <section className="card form-card">
      <div className="card-head">
        <h2>{t('settings.users')}</h2>
        <button className="btn btn-sm" onClick={() => setCreating(true)}>
          <Icon name="plus" size={14} />
          {t('settings.newUser')}
        </button>
      </div>
      <ul className="members">
        {data?.map((u) => (
          <li key={u.id}>
            <span>
              <strong>{u.display_name || u.username}</strong> <span className="muted">@{u.username}</span>
              {u.is_admin && <span className="badge">{t('settings.admin')}</span>}
              <div className="muted small">{date(u.created_at, locale)}</div>
            </span>
            {u.id !== user.id && (
              <>
                <button className="btn btn-sm" onClick={() => setResetting(u)}>
                  {t('settings.resetPassword')}
                </button>
                <button className="btn-icon" onClick={() => del(u)} aria-label={t('common.delete')}>
                  <Icon name="trash" size={16} />
                </button>
              </>
            )}
          </li>
        ))}
      </ul>
      {creating && (
        <UserForm
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false)
            toast(t('settings.userCreated'))
            reload()
          }}
        />
      )}
      {resetting && <ResetForm user={resetting} onClose={() => setResetting(null)} />}
    </section>
  )
}

function UserForm({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const { t } = useI18n()
  const { fail } = useUI()
  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [admin, setAdmin] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await api.post('/api/users', { username, display_name: name, password, is_admin: admin })
      onSaved()
    } catch (err) {
      fail(err)
    }
  }
  return (
    <Modal title={t('settings.newUser')} onClose={onClose} narrow>
      <form className="form-grid" onSubmit={submit}>
        <Field label={t('auth.username')} wide>
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoCapitalize="none" required autoFocus />
        </Field>
        <Field label={t('auth.displayName')} wide>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={t('auth.password')} hint={t('auth.passwordHint')} wide>
          <input type="text" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required autoComplete="off" />
        </Field>
        <label className="check field-wide">
          <input type="checkbox" checked={admin} onChange={(e) => setAdmin(e.target.checked)} />
          {t('settings.admin')}
        </label>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button className="btn btn-primary">{t('common.save')}</button>
        </div>
      </form>
    </Modal>
  )
}

function ResetForm({ user, onClose }: { user: User; onClose: () => void }) {
  const { t } = useI18n()
  const { fail, toast } = useUI()
  const [password, setPassword] = useState('')
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await api.post(`/api/users/${user.id}/password`, { password })
      toast(t('common.saved'))
      onClose()
    } catch (err) {
      fail(err)
    }
  }
  return (
    <Modal title={`${t('settings.resetPassword')} · ${user.username}`} onClose={onClose} narrow>
      <form onSubmit={submit}>
        <Field label={t('settings.newPassword')} hint={t('auth.passwordHint')}>
          <input type="text" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required autoFocus autoComplete="off" />
        </Field>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button className="btn btn-primary">{t('common.save')}</button>
        </div>
      </form>
    </Modal>
  )
}
