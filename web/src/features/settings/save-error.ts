import { APIError } from '@/lib/api-client'
import i18n from '@/i18n'
import type { LayeredSettings } from './types'

interface SettingsFileResult {
  path: string
  status: 'saved' | 'failed'
  code?: string
}

/** Keeps a failed multi-file save and its canonical readback together. A failed
 * request may have durable effects; consumers must reconcile before retrying. */
export class SettingsSaveError extends APIError {
  readonly snapshot: LayeredSettings

  constructor(error: APIError, snapshot: LayeredSettings, files: SettingsFileResult[]) {
    const saved = files.filter(file => file.status === 'saved').map(file => file.path).join(', ')
    const failed = files.filter(file => file.status === 'failed').map(file => file.path).join(', ')
    super(i18n.t(saved ? 'settings.partialSaveFailed' : 'settings.filesSaveFailed', { saved, failed }), error)
    this.snapshot = snapshot
    this.cause = error
  }
}

/** Recognizes the settings API's per-file outcome contract, not arbitrary 409s. */
export function settingsSaveFiles(error: unknown): SettingsFileResult[] | null {
  if (!(error instanceof APIError) || error.code !== 'settings_file_save_failed') return null
  const files = error.details?.files
  if (!Array.isArray(files) || !files.length) return null
  if (!files.every(file => file && typeof file.path === 'string'
    && (file.status === 'saved' || file.status === 'failed')
    && (file.code === undefined || typeof file.code === 'string'))) return null
  return files
}
