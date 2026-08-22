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

export function fileName(path: string) {
  return path.split('/').pop() || path
}

export function dirName(path: string) {
  const parts = path.split('/')
  parts.pop()
  return parts.join('/')
}

export function statusLabel(status: string) {
  if (status === 'added') return 'A'
  if (status === 'deleted') return 'D'
  return 'M'
}

export function statusText(status: string, t: TFunction) {
  if (status === 'added') return t('versions.change.added')
  if (status === 'deleted') return t('versions.change.deleted')
  return t('versions.change.modified')
}

export function statusColor(status: string) {
  if (status === 'deleted') return 'text-[var(--nova-danger)]'
  if (status === 'added') return 'text-[var(--nova-accent-green)]'
  return 'text-[var(--nova-accent)]'
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
