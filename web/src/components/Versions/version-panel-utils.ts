import type { VersionEntry } from '@/lib/api'
import type { VersionItem } from '@/features/versions/components/version-timeline'

export function versionToTimelineItem(version: VersionEntry): VersionItem {
  return {
    id: version.id,
    title: version.message || '(无说明)',
    description: sourceText(version.source),
    createdAt: formatTime(version.created_at),
    author: `${version.file_count} 文件 · ${formatBytes(version.total_bytes)}`,
  }
}

function sourceText(source: VersionEntry['source']) {
  if (source === 'timer') return '定时'
  if (source === 'agent') return 'Agent'
  if (source === 'rollback_backup') return '回滚前备份'
  return '手动'
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

export function statusText(status: string) {
  if (status === 'added') return '新增'
  if (status === 'deleted') return '删除'
  return '修改'
}

export function statusColor(status: string) {
  if (status === 'deleted') return 'text-red-300'
  if (status === 'added') return 'text-[var(--nova-accent-green)]'
  return 'text-[var(--nova-accent)]'
}

export function formatTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
