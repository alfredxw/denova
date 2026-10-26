import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APP_VERSION } from '@/app-version'
import { Button } from '@/components/ui/button'
import { markMessageRead } from '@/features/messages/api'
import { requestSettingsSection } from '@/features/onboarding/events'
import { getUpdateStatus } from '@/features/settings/api'
import { UPDATE_CHECK_RESULT_EVENT, UPDATE_NOTES_EVENT } from '@/features/settings/update-events'
import { buildFrontendReloadURL } from '@/features/settings/update-reload'
import type { UpdateCheckResult, UpdateNotesRange } from '@/features/settings/types'
import { toast } from '@/lib/toast'
import { displayVersion, UpdateNotesDialog } from './UpdateNotes'

const ANNOUNCED_UPDATE_KEY = 'nova.update.announcedTransaction'

/** Owns browser update notices and the shared reader. Download/apply operations
 * remain in Settings and on the server, so opening notes never starts an update. */
export function SoftwareUpdateMonitor({ onCheckResult }: { onCheckResult: (result: UpdateCheckResult, ready?: boolean) => void }) {
  const { t } = useTranslation()
  const [target, setTarget] = useState<(UpdateNotesRange & { kind: 'available' | 'completed' }) | null>(null)
  const announced = useRef('')
  useEffect(() => {
    const check = (event: Event) => onCheckResult((event as CustomEvent<UpdateCheckResult>).detail)
    const notes = (event: Event) => setTarget({ ...(event as CustomEvent<UpdateNotesRange>).detail, kind: 'available' })
    window.addEventListener(UPDATE_CHECK_RESULT_EVENT, check)
    window.addEventListener(UPDATE_NOTES_EVENT, notes)
    return () => {
      window.removeEventListener(UPDATE_CHECK_RESULT_EVENT, check)
      window.removeEventListener(UPDATE_NOTES_EVENT, notes)
    }
  }, [onCheckResult])

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      try {
        const status = await getUpdateStatus(AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]))
        if (!controller.signal.aborted) {
          // Every workbench must load the new bundle, including tabs that did
          // not initiate the restart or open Settings.
          if (status.phase === 'succeeded' && status.id && status.version
            && status.current_version.replace(/^v/, '') === status.version.replace(/^v/, '')
            && status.current_version.replace(/^v/, '') !== APP_VERSION.replace(/^v/, '')) {
            window.location.replace(buildFrontendReloadURL(status.version, Date.now(), window.location.href))
            return
          }
          if (status.phase === 'staged' && status.version) {
            onCheckResult({ current_version: status.current_version, latest_version: status.version, update_available: true, can_install: false, platform: '', release_url: '', published_at: '' }, true)
          } else if (status.check) onCheckResult(status.check)
          const completed = status.completed_update
          if (completed && completed.id !== announced.current) {
            announced.current = completed.id
            let seen = false
            try { seen = window.localStorage.getItem(ANNOUNCED_UPDATE_KEY) === completed.id }
            catch (error) { console.warn('[software-update] Could not read announcement state', error) }
            if (!seen) {
              toast.success(t('settings.updates.notes.completedTitle', { version: displayVersion(completed.version) }), {
                description: t('settings.updates.notes.fromVersion', { version: displayVersion(completed.previous_version) }),
                action: { label: t('settings.updates.notes.viewChanges'), onClick: () => {
                  setTarget({ fromVersion: completed.previous_version, toVersion: completed.version, kind: 'completed' })
                  void markMessageRead(`update:${completed.id}`).catch(error => console.warn('[software-update] Could not mark upgrade notice read', error))
                } },
              })
              try { window.localStorage.setItem(ANNOUNCED_UPDATE_KEY, completed.id) }
              catch (error) { console.warn('[software-update] Could not save announcement state', error) }
            }
          }
        }
      } catch (error) { if (!controller.signal.aborted) console.warn('[software-update] Update status unavailable', error) }
      if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 15_000)
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [onCheckResult, t])

  return target && <UpdateNotesDialog range={target} kind={target.kind} open onOpenChange={open => { if (!open) setTarget(null) }}
    actions={target.kind === 'available' && <Button onClick={() => { setTarget(null); requestSettingsSection('updates') }}>{t('settings.updates.notes.goToUpdate')}</Button>} />
}
