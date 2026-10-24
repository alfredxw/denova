import type { VersionEntry } from '@/lib/api'
import type { TFunction } from 'i18next'
import { formatDateTime } from '@/i18n'

export function sourceText(source: VersionEntry['source'], t: TFunction) {
  if (source === 'timer') return t('versions.source.timer')
  if (source === 'agent') return t('versions.source.agent')
  if (source === 'rollback_backup') return t('versions.source.rollbackBackup')
  return t('versions.source.manual')
}

export function localizeVersionMessage(message: string, t: TFunction): string {
  if (!message) return t('versions.emptyMessage')
  let localized = message
  if (localized.startsWith('手动保存')) {
    localized = localized.replace(/^手动保存[：:]?/, t('versions.source.manual') + ': ')
  } else if (localized.startsWith('自动版本')) {
    localized = localized.replace(/^自动版本[：:]?/, t('versions.source.timer') + ': ')
  } else if (localized.startsWith('Agent 自动保存')) {
    localized = localized.replace(/^Agent 自动保存[：:]?/, t('versions.source.agent') + ': ')
  } else if (localized.startsWith('回滚前备份')) {
    localized = localized.replace(/^回滚前备份[：:]?/, t('versions.source.rollbackBackup') + ': ')
  } else if (localized.startsWith('保存版本')) {
    localized = localized.replace(/^保存版本[：:]?/, t('versions.saveCurrent') + ': ')
  }
  localized = localized
    .replace(/新增(\d+)个/g, t('versions.change.added') + ' $1')
    .replace(/修改(\d+)个/g, t('versions.change.modified') + ' $1')
    .replace(/删除(\d+)个/g, t('versions.change.deleted') + ' $1')
    .replace(/更新(\d+)个/g, '$1')
    .replace(/文件/g, '')
    .replace(/（/g, ' (')
    .replace(/）/g, ')')
    .replace(/、/g, ', ')
  return localized
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
