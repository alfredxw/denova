import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronDown, Download, ExternalLink, Loader2, RefreshCw, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { APP_VERSION } from '@/app-version'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { applyUpdate, checkForUpdate, installUpdateStream, uploadUpdate } from './api'
import type { UpdateCheckResult, UpdateInstallProgress, UpdateInstallResult } from './types'
import { markAutoUpdateChecked, notifyUpdateCheckResult, shouldRunAutoUpdateCheck } from './update-check-cache'
import { scheduleFrontendReloadAfterUpdate } from './update-reload'

type UpdateOperation = 'idle' | 'checking' | 'installing' | 'uploading' | 'applying' | 'restarting'

// Keep update state in SettingsView so collapsing the section cannot discard
// an upload in progress or its staged result.
export function useUpdateSettings({ autoCheckEnabled }: { autoCheckEnabled: boolean }) {
  const { t } = useTranslation()
  const [updateStatus, setUpdateStatus] = useState<UpdateCheckResult | null>(null)
  const [updateInstallResult, setUpdateInstallResult] = useState<UpdateInstallResult | null>(null)
  const [updateInstallProgress, setUpdateInstallProgress] = useState<UpdateInstallProgress | null>(null)
  const [operation, setOperation] = useState<UpdateOperation>('idle')
  const [updateError, setUpdateError] = useState<string | null>(null)
  const runUpdateCheck = useCallback(async (source: 'auto' | 'manual' = 'manual') => {
    setOperation('checking')
    setUpdateError(null)
    try {
      const result = await checkForUpdate()
      setUpdateStatus(result)
      notifyUpdateCheckResult(result)
    } catch (e) {
      setUpdateError((e as Error).message)
    } finally {
      if (source === 'auto') markAutoUpdateChecked()
      setOperation('idle')
    }
  }, [])

  useEffect(() => {
    if (!autoCheckEnabled || updateStatus || operation !== 'idle' || updateInstallResult) return
    if (!shouldRunAutoUpdateCheck()) return
    void runUpdateCheck('auto')
  }, [autoCheckEnabled, operation, updateInstallResult, runUpdateCheck, updateStatus])

  const runUpdateInstall = useCallback(async () => {
    setOperation('installing')
    setUpdateError(null)
    setUpdateInstallProgress(null)
    try {
      const stream = await installUpdateStream()
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const data = JSON.parse(value.data) as Record<string, unknown>
        if (value.event === 'update_progress') {
          setUpdateInstallProgress(data as unknown as UpdateInstallProgress)
        } else if (value.event === 'update_result') {
          const result = data as unknown as UpdateInstallResult
          setUpdateInstallResult(result)
          setUpdateInstallProgress((prev) => prev ? { ...prev, phase: 'staged', percent: 100 } : { phase: 'staged', percent: 100 })
        } else if (value.event === 'error') {
          throw new Error(typeof data.message === 'string' && data.message ? data.message : t('settings.updates.error'))
        }
      }
    } catch (e) {
      setUpdateError((e as Error).message)
    } finally {
      setOperation('idle')
    }
  }, [t])

  const runUpdateApply = useCallback(async () => {
    setOperation('applying')
    setUpdateError(null)
    try {
      const result = await applyUpdate()
      setOperation('restarting')
      scheduleFrontendReloadAfterUpdate(result.version)
    } catch (e) {
      setUpdateError((e as Error).message)
      setOperation('idle')
    }
  }, [])

  const runLocalUpdate = async (file: File) => {
    setUpdateError(null)
    if (file.size > 512 * 1024 * 1024) {
      setUpdateError(t('settings.updates.tooLarge'))
      return
    }
    setOperation('uploading')
    setUpdateInstallProgress(null)
    try {
      setUpdateInstallResult(await uploadUpdate(file))
    } catch (error) {
      setUpdateError((error as Error).message)
    } finally {
      setOperation('idle')
    }
  }

  return {
    status: updateStatus,
    installResult: updateInstallResult,
    installProgress: updateInstallProgress,
    operation,
    error: updateError,
    onCheck: () => void runUpdateCheck(),
    onInstall: () => void runUpdateInstall(),
    onApply: () => void runUpdateApply(),
    onUpload: (file: File) => void runLocalUpdate(file),
  }
}

export function UpdatePanel({
  status,
  installResult,
  installProgress,
  operation,
  error,
  onCheck,
  onInstall,
  onApply,
  onUpload,
}: ReturnType<typeof useUpdateSettings>) {
  const { t } = useTranslation()
  const fileInput = useRef<HTMLInputElement>(null)
  const releaseDate = status?.published_at ? new Date(status.published_at).toLocaleString() : ''
  const applyReady = Boolean(installResult?.apply_ready)
  const checking = operation === 'checking'
  const installing = operation === 'installing'
  const uploading = operation === 'uploading'
  const applying = operation === 'applying' || operation === 'restarting'
  const busy = operation !== 'idle'
  const installDisabled = busy || !status?.can_install || applyReady
  const applyDisabled = busy || !applyReady
  const progressPercent = clampPercent(installProgress?.percent ?? 0)
  const progressLabel = installProgress ? updatePhaseLabel(installProgress.phase, t) : ''
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-[var(--nova-text)]">{status ? updateStatusLabel(status, t) : t('settings.updates.notChecked')}</span>
            {status?.update_available && (
              <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-1.5 py-0.5 text-[11px] text-[var(--nova-text)]">
                {t('settings.updates.available')}
              </span>
            )}
          </div>
          <div className="grid gap-1 text-[var(--nova-text-faint)] sm:grid-cols-2">
            <span>{t('settings.updates.currentVersion', { version: status?.current_version || APP_VERSION })}</span>
            <span>{t('settings.updates.latestVersion', { version: status?.latest_version || t('common.notSet') })}</span>
            <span>{t('settings.updates.platform', { platform: status?.platform || t('common.notSet') })}</span>
            <span>{t('settings.updates.publishedAt', { time: releaseDate || t('common.notSet') })}</span>
          </div>
          {status?.asset && (
            <div className="truncate text-[var(--nova-text-faint)]">
              {t('settings.updates.asset', { name: status.asset.name, size: formatBytes(status.asset.size) })}
            </div>
          )}
          {installProgress && (
            <div className="mt-2 flex flex-col gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2">
              <div className="flex items-center justify-between gap-3 text-[var(--nova-text-muted)]">
                <span>{progressLabel}</span>
                <span>{t('settings.updates.progressPercent', { percent: Math.round(progressPercent) })}</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[var(--nova-surface-3)]" aria-label={t('settings.updates.progressAria')}>
                <div
                  className="h-full rounded-full bg-[var(--nova-text)] transition-[width] duration-200"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
              <div className="flex flex-col gap-1 text-[11px] text-[var(--nova-text-faint)] sm:flex-row sm:items-center sm:justify-between">
                <span>{t('settings.updates.downloaded', {
                  downloaded: formatBytes(installProgress.downloaded_bytes ?? 0),
                  total: installProgress.total_bytes ? formatBytes(installProgress.total_bytes) : t('common.notSet'),
                })}</span>
                {installProgress.archive_path && (
                  <span className="max-w-full truncate">{t('settings.updates.localPackage', { path: installProgress.archive_path })}</span>
                )}
              </div>
            </div>
          )}
          {installResult?.apply_ready && (
            <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
              {t('settings.updates.stagedVersion', { version: installResult.installed_version })} {t('settings.updates.stagedRestart')}
            </div>
          )}
          {operation === 'restarting' && (
            <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
              {t('settings.updates.applyingRestart')}
            </div>
          )}
          {error && <InlineErrorNotice className="mt-2" message={error} title={t('settings.updates.error')} />}
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {status?.release_url && (
            <a
              href={status.release_url}
              target="_blank"
              rel="noreferrer"
              className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-1 text-[var(--nova-text)]"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {t('settings.updates.openRelease')}
            </a>
          )}
          <button
            type="button"
            onClick={onCheck}
            disabled={busy || applyReady}
            className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${checking ? 'animate-spin' : ''}`} />
            {checking ? t('settings.updates.checking') : t('settings.updates.check')}
          </button>
          <button
            type="button"
            onClick={onInstall}
            disabled={installDisabled}
            className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
          >
            {installing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Download className="h-3.5 w-3.5" />}
            {installing ? t('settings.updates.installing') : t('settings.updates.install')}
          </button>
          {applyReady && (
            <button
              type="button"
              onClick={onApply}
              disabled={applyDisabled}
              className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
            >
              {applying ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              {applying ? t('settings.updates.applying') : t('settings.updates.apply')}
            </button>
          )}
        </div>
      </div>
      <Collapsible className="mt-3">
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm" className="group w-full justify-start">
            <ChevronDown data-icon="inline-start" className="transition-transform group-data-[state=open]:rotate-180" />
            {t('settings.updates.manual')}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="flex min-w-0 flex-col gap-2 px-2 pt-2">
            <p className="text-xs text-[var(--nova-text-faint)]">{t('settings.updates.manualHint')}</p>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="link" size="sm" asChild>
                <a href="https://github.com/alfredxw/denova/releases/latest" target="_blank" rel="noreferrer">
                  <ExternalLink data-icon="inline-start" />
                  {t('settings.updates.openRelease')}
                </a>
              </Button>
              <input
                ref={fileInput}
                type="file"
                accept=".zip,.tar.gz"
                className="hidden"
                aria-label={t('settings.updates.selectPackage')}
                disabled={busy || applyReady}
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  event.target.value = ''
                  if (file) onUpload(file)
                }}
              />
              <Button variant="outline" size="sm" onClick={() => fileInput.current?.click()} disabled={busy || applyReady}>
                {uploading ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Upload data-icon="inline-start" />}
                {uploading ? t('settings.updates.uploading') : t('settings.updates.selectPackage')}
              </Button>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

function updateStatusLabel(status: UpdateCheckResult, t: (key: string, args?: Record<string, unknown>) => string) {
  if (status.update_available) return t('settings.updates.updateAvailableTitle')
  return t('settings.updates.upToDateTitle')
}

function updatePhaseLabel(phase: string, t: (key: string, args?: Record<string, unknown>) => string) {
  switch (phase) {
    case 'checking':
      return t('settings.updates.phase.checking')
    case 'downloading':
      return t('settings.updates.phase.downloading')
    case 'verifying':
      return t('settings.updates.phase.verifying')
    case 'extracting':
      return t('settings.updates.phase.extracting')
    case 'replacing':
      return t('settings.updates.phase.replacing')
    case 'staging':
      return t('settings.updates.phase.staging')
    case 'staged':
      return t('settings.updates.phase.staged')
    case 'installed':
      return t('settings.updates.phase.installed')
    default:
      return t('settings.updates.phase.running')
  }
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

