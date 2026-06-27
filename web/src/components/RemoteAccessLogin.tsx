import { type FormEvent, useEffect, useState } from 'react'
import { LockKeyhole, LogIn } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { fetchSettings } from '@/features/settings/api'
import { setRemoteAccessCredentials } from '@/lib/api-client'

const REMOTE_ACCESS_REQUIRED_EVENT = 'nova:remote-access-required'

export function RemoteAccessLogin() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const show = () => {
      setOpen(true)
      setError('')
    }
    window.addEventListener(REMOTE_ACCESS_REQUIRED_EVENT, show)
    return () => window.removeEventListener(REMOTE_ACCESS_REQUIRED_EVENT, show)
  }, [])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      setRemoteAccessCredentials(username, password)
      await fetchSettings()
      setOpen(false)
      setPassword('')
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      setError((e as Error).message || t('remoteAccess.loginFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/45 px-4 backdrop-blur-xl">
      <form
        className="w-full max-w-sm rounded-[20px] border border-white/15 bg-[var(--nova-surface)]/95 p-5 shadow-2xl"
        onSubmit={submit}
      >
        <div className="mb-4 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-[14px] bg-[var(--nova-surface-2)] text-[var(--nova-accent)]">
            <LockKeyhole className="h-5 w-5" aria-hidden="true" />
          </div>
          <div>
            <h2 className="text-sm font-semibold text-[var(--nova-text)]">{t('remoteAccess.title')}</h2>
            <p className="mt-1 text-xs leading-5 text-[var(--nova-text-muted)]">{t('remoteAccess.description')}</p>
          </div>
        </div>
        <label className="mb-3 block text-xs text-[var(--nova-text-muted)]">
          <span className="mb-1.5 block">{t('remoteAccess.username')}</span>
          <input
            autoFocus
            className="nova-field min-h-9 w-full rounded-[var(--nova-radius)] border px-3 py-2 text-sm outline-none max-md:text-[16px]"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            autoComplete="username"
          />
        </label>
        <label className="block text-xs text-[var(--nova-text-muted)]">
          <span className="mb-1.5 block">{t('remoteAccess.password')}</span>
          <input
            className="nova-field min-h-9 w-full rounded-[var(--nova-radius)] border px-3 py-2 text-sm outline-none max-md:text-[16px]"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            type="password"
            autoComplete="current-password"
          />
        </label>
        {error && <div className="mt-3 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{error}</div>}
        <button
          className="nova-nav-item mt-4 inline-flex h-9 w-full items-center justify-center gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-accent)] px-3 text-sm font-medium text-white disabled:opacity-60"
          type="submit"
          disabled={submitting || !username.trim() || !password}
        >
          <LogIn className="h-4 w-4" aria-hidden="true" />
          {submitting ? t('remoteAccess.signingIn') : t('remoteAccess.signIn')}
        </button>
      </form>
    </div>
  )
}
