import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { api } from '../api'
import { useI18n } from '../i18n'

// ---- toast ----

type ToastKind = 'ok' | 'error' | 'info'
interface Toast {
  id: number
  kind: ToastKind
  text: ReactNode
}

interface ConfirmOptions {
  title: string
  message?: ReactNode
  confirmLabel?: string
  danger?: boolean
}

interface UI {
  toast: (text: ReactNode, kind?: ToastKind) => void
  confirm: (o: ConfirmOptions) => Promise<boolean>
  /** Shows the error as a toast, translated. */
  fail: (e: unknown) => void
}

const Ctx = createContext<UI | null>(null)

export function useUI(): UI {
  const c = useContext(Ctx)
  if (!c) throw new Error('UIProvider missing')
  return c
}

let nextId = 1

export function UIProvider({ children }: { children: ReactNode }) {
  const { t, errorText } = useI18n()
  const [toasts, setToasts] = useState<Toast[]>([])
  const [pending, setPending] = useState<(ConfirmOptions & { resolve: (v: boolean) => void }) | null>(null)

  const toast = useCallback((text: ReactNode, kind: ToastKind = 'ok') => {
    const id = nextId++
    setToasts((x) => [...x, { id, kind, text }])
    setTimeout(() => setToasts((x) => x.filter((y) => y.id !== id)), kind === 'error' ? 7000 : 3500)
  }, [])

  const confirm = useCallback(
    (o: ConfirmOptions) => new Promise<boolean>((resolve) => setPending({ ...o, resolve })),
    [],
  )

  const fail = useCallback((e: unknown) => toast(errorText(e), 'error'), [toast, errorText])

  const close = (v: boolean) => {
    pending?.resolve(v)
    setPending(null)
  }

  return (
    <Ctx.Provider value={{ toast, confirm, fail }}>
      {children}
      <div className="toasts" aria-live="polite">
        {toasts.map((x) => (
          <div key={x.id} className={`toast toast-${x.kind}`} onClick={() => setToasts((y) => y.filter((z) => z.id !== x.id))}>
            {x.text}
          </div>
        ))}
      </div>
      {pending && (
        <Modal title={pending.title} onClose={() => close(false)} narrow>
          {pending.message && <div className="confirm-msg">{pending.message}</div>}
          <div className="modal-actions">
            <button className="btn" onClick={() => close(false)}>
              {t('common.cancel')}
            </button>
            <button className={`btn ${pending.danger ? 'btn-danger' : 'btn-primary'}`} autoFocus onClick={() => close(true)}>
              {pending.confirmLabel ?? t('common.confirm')}
            </button>
          </div>
        </Modal>
      )}
    </Ctx.Provider>
  )
}

// ---- modal ----

const modalStack: object[] = []

/** Dialog; on phones it becomes a bottom sheet. */
export function Modal({
  title,
  children,
  onClose,
  narrow,
}: {
  title: ReactNode
  children: ReactNode
  onClose: () => void
  narrow?: boolean
}) {
  const { t } = useI18n()
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  useEffect(() => {
    // Esc closes only the topmost dialog (e.g. a confirmation above a form).
    const me = {}
    modalStack.push(me)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && modalStack[modalStack.length - 1] === me) onCloseRef.current()
    }
    window.addEventListener('keydown', onKey)
    document.body.classList.add('modal-open')
    return () => {
      window.removeEventListener('keydown', onKey)
      modalStack.splice(modalStack.indexOf(me), 1)
      if (modalStack.length === 0) document.body.classList.remove('modal-open')
    }
  }, [])
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`modal${narrow ? ' modal-narrow' : ''}`} role="dialog" aria-modal="true">
        <div className="modal-head">
          <h2>{title}</h2>
          <button className="btn-icon" onClick={onClose} aria-label={t('common.close')}>
            ×
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  )
}

export function Spinner({ small }: { small?: boolean }) {
  return <span className={`spinner${small ? ' spinner-sm' : ''}`} role="status" />
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>
}

export function Loading() {
  return (
    <div className="loading">
      <Spinner />
    </div>
  )
}

/** Loads data from the API; reload() fetches it again. */
export function useLoad<T>(url: string | null, version = 0) {
  const { fail } = useUI()
  const [data, setData] = useState<T | null>(null)
  const [n, setN] = useState(0)
  useEffect(() => {
    if (!url) return
    let alive = true
    api.get<T>(url)
      .then((d) => alive && setData(d))
      .catch((e) => alive && fail(e))
    return () => {
      alive = false
    }
  }, [url, n, version, fail])
  return { data, setData, reload: () => setN((x) => x + 1) }
}

// ---- form fields ----

export function Field({
  label,
  hint,
  children,
  wide,
}: {
  label: ReactNode
  hint?: ReactNode
  children: ReactNode
  wide?: boolean
}) {
  return (
    <label className={`field${wide ? ' field-wide' : ''}`}>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  )
}

export function StatusPill({ status, label }: { status: string; label: string }) {
  return <span className={`pill pill-${status}`}>{label}</span>
}
