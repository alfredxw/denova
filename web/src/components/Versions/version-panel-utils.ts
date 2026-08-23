import type { VersionEntry } from '@/lib/api'
import type { TFunction } from 'i18next'
import { formatDateTime } from '@/i18n'

export function sourceText(source: VersionEntry['source'], t: TFunction) {
  if (source === 'timer') return t('versions.source.timer')
  if (source === 'agent') return t('versions.source.agent')
  if (source === 'rollback_backup') return t('versions.source.rollbackBackup')
  return t('versions.source.manual')
}

export function workspaceName(path: string) {
  return path.split('/').filter(Boolean).pop() || path
}

export function formatTime(value: string) {
  if (!value) return ''
  return formatDateTime(value) || value
}

export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
