import type { VersionEntry } from '@/lib/api'
import type { VersionItem } from '@/features/versions/components/version-timeline'
import type { TFunction } from 'i18next'
import { formatDateTime } from '@/i18n'

export function versionToTimelineItem(version: VersionEntry, t: TFunction): VersionItem {
  return {
    id: version.id,
    title: version.message || t('versions.emptyMessage'),
    description: sourceText(version.source, t),
    createdAt: formatTime(version.created_at),
    author: t('versions.filesBytes', { files: version.file_count, bytes: formatBytes(version.total_bytes) }),
  }
}

function sourceText(source: VersionEntry['source'], t: TFunction) {
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
  if (status === 'deleted') return 'text-red-300'
  if (status === 'added') return 'text-[var(--nova-accent-green)]'
  return 'text-[var(--nova-accent)]'
}

export function formatTime(value: string) {
  if (!value) return ''
  return formatDateTime(value) || value
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
