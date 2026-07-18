import { Building2, FileText, Library, MapPin, ScrollText, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { LoreItem } from '@/lib/api'

export type LoreType = LoreItem['type']
export type LoreLoadModeFilter = 'all' | 'resident' | 'on_demand'

export interface KnowledgeSection {
  id: string
  labelKey: string
  icon: LucideIcon
  types: LoreType[]
  createType: LoreType
  /** 新建条目默认名称的 i18n key */
  createNameKey: string
  tag?: string
  excludeTag?: string
}

/** 资料库目录的固定分组定义，SettingPanel 与目录组件共用这一份。 */
export const KNOWLEDGE_SECTIONS: KnowledgeSection[] = [
  { id: 'characters', labelKey: 'lore.type.character', icon: UserRound, types: ['character'], createType: 'character', createNameKey: 'settingPanel.lore.newCharacter' },
  { id: 'locations', labelKey: 'lore.type.location', icon: MapPin, types: ['location'], createType: 'location', createNameKey: 'settingPanel.lore.newLocation' },
  { id: 'factions', labelKey: 'lore.type.faction', icon: Building2, types: ['faction'], createType: 'faction', createNameKey: 'settingPanel.lore.newFaction' },
  { id: 'rules', labelKey: 'lore.type.rule', icon: ScrollText, types: ['world', 'rule'], createType: 'rule', createNameKey: 'settingPanel.lore.newRule' },
  { id: 'templates', labelKey: 'settingPanel.section.templates', icon: FileText, types: ['other'], createType: 'other', createNameKey: 'settingPanel.lore.newTemplate', tag: '模板' },
  { id: 'assets', labelKey: 'settingPanel.section.assets', icon: Library, types: ['item', 'other'], createType: 'item', createNameKey: 'settingPanel.lore.newAsset', excludeTag: '模板' },
]

/** 按分组归属 + 加载策略 + 搜索词过滤资料条目。 */
export function sectionItems(items: LoreItem[], section: KnowledgeSection, query = '', loadModeFilter: LoreLoadModeFilter = 'all') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (!section.types.includes(item.type)) return false
    if (loadModeFilter === 'resident' && item.load_mode !== 'resident') return false
    if (loadModeFilter === 'on_demand' && item.load_mode === 'resident') return false
    const tags = item.tags || []
    if (section.tag && !tags.includes(section.tag)) return false
    if (section.excludeTag && tags.includes(section.excludeTag)) return false
    if (normalizedQuery) {
      const haystack = [item.name, item.brief_description || '', item.content || '', tags.join('\n')].join('\n').toLowerCase()
      if (!haystack.includes(normalizedQuery)) return false
    }
    return true
  })
}

/** 目录中第一个可见条目的 id（按分组顺序），用于加载后的默认选中。 */
export function firstVisibleLoreItemId(items: LoreItem[]): string | null {
  for (const section of KNOWLEDGE_SECTIONS) {
    const first = sectionItems(items, section)[0]
    if (first) return first.id
  }
  return null
}
