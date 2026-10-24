import { useEffect, useMemo, useState } from 'react'
import { SlidersHorizontal, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { LoreItem } from '@/lib/api'
import { useLoreCategories } from './use-lore-categories'
import { EMPTY_LORE_FILTERS, loreTagFilterValue, type LoreFilters } from './lore-filters'
import { IMPORTANCE_OPTIONS, LOAD_MODE_OPTIONS } from './options'
import { isLoreProtagonistTag } from './tags'

interface LoreFilterProps {
  projectId: string
  filters: LoreFilters
  onChange: (filters: LoreFilters) => void
}

/** Both library views edit the owner's conditions; tag choices always use the full catalog. */
export function LoreFiltersButton({
  projectId, items, filters, onChange, presentation = 'label',
}: LoreFilterProps & { items: LoreItem[]; presentation?: 'icon' | 'label' }) {
  const { t } = useTranslation()
  const { sections: categorySections, isSuccess } = useLoreCategories(projectId)
  useEffect(() => {
    if (isSuccess && filters.category !== 'all' && !categorySections.some((section) => section.id === filters.category)) {
      onChange({ ...filters, category: 'all' })
    }
  }, [isSuccess, categorySections, filters, onChange])
  const [tagQuery, setTagQuery] = useState('')
  const active = Object.values(filters).some((value) => value !== 'all')
  const tags = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of items) {
      for (const tag of new Set((item.tags || []).map(loreTagFilterValue))) {
        counts.set(tag, (counts.get(tag) || 0) + 1)
      }
    }
    return [...counts].sort(([a], [b]) => a.localeCompare(b))
  }, [items])
  const tagLabel = (tag: string) => isLoreProtagonistTag(tag) ? t('lore.library.protagonist') : tag
  const toggleTag = (tag: string) => {
    const selected = Array.isArray(filters.tags) ? filters.tags : []
    const next = selected.includes(tag) ? selected.filter((value) => value !== tag) : [...selected, tag]
    onChange({ ...filters, tags: next.length ? next : 'all' })
  }

  return (
    <Popover onOpenChange={() => setTagQuery('')}>
      <PopoverTrigger asChild>
        <Button size={presentation === 'icon' ? 'icon-sm' : 'sm'} variant={active ? 'secondary' : 'outline'}
          aria-label={t('lore.filters.open')} title={t('lore.filters.open')}>
          <SlidersHorizontal data-icon="inline-start" />
          {presentation === 'label' && t('lore.filters.open')}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" aria-label={t('lore.filters.title')}
        className="w-[min(24rem,calc(100vw-1rem))] max-h-[min(80vh,var(--radix-popover-content-available-height))] overflow-y-auto p-4">
        <div className="flex items-center justify-between gap-2">
          <span className="font-medium">{t('lore.filters.title')}</span>
          <Button variant="ghost" size="sm" disabled={!active} onClick={() => onChange(EMPTY_LORE_FILTERS)}>
            {t('lore.library.clearFilters')}
          </Button>
        </div>
        <FieldGroup className="grid grid-cols-2 gap-3">
          <Field>
            <FieldLabel>{t('lore.library.category')}</FieldLabel>
            <Select value={filters.category} onValueChange={(category) => onChange({ ...filters, category })}>
              <SelectTrigger className="w-full min-w-0" aria-label={t('lore.library.category')}><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                <SelectItem value="all">{t('lore.library.allCategories')}</SelectItem>
                {categorySections.map((section) => <SelectItem key={section.id} value={section.id}>{(section.name || t(section.labelKey))}</SelectItem>)}
              </SelectGroup></SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>{t('settingPanel.field.loadMode')}</FieldLabel>
            <Select value={filters.loadMode} onValueChange={(loadMode) => onChange({ ...filters, loadMode: loadMode as LoreFilters['loadMode'] })}>
              <SelectTrigger className="w-full min-w-0" aria-label={t('settingPanel.field.loadMode')}><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                <SelectItem value="all">{t('lore.filters.allLoadModes')}</SelectItem>
                {LOAD_MODE_OPTIONS.map(({ value }) => <SelectItem key={value} value={value}>{t(`lore.loadMode.${value}`)}</SelectItem>)}
              </SelectGroup></SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>{t('lore.filters.enabled')}</FieldLabel>
            <Select value={filters.enabled} onValueChange={(enabled) => onChange({ ...filters, enabled: enabled as LoreFilters['enabled'] })}>
              <SelectTrigger className="w-full min-w-0" aria-label={t('lore.filters.enabled')}><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                {(['all', 'enabled', 'disabled'] as const).map((value) => <SelectItem key={value} value={value}>{t(`lore.filters.enabled.${value}`)}</SelectItem>)}
              </SelectGroup></SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>{t('settingPanel.field.importance')}</FieldLabel>
            <Select value={filters.importance} onValueChange={(importance) => onChange({ ...filters, importance: importance as LoreFilters['importance'] })}>
              <SelectTrigger className="w-full min-w-0" aria-label={t('settingPanel.field.importance')}><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                <SelectItem value="all">{t('lore.filters.allImportance')}</SelectItem>
                {IMPORTANCE_OPTIONS.map(({ value }) => <SelectItem key={value} value={value}>{t(`lore.importance.${value}`)}</SelectItem>)}
              </SelectGroup></SelectContent>
            </Select>
          </Field>
          <Field className="col-span-2">
            <FieldLabel>{t('lore.library.coverFilter')}</FieldLabel>
            <Select value={filters.cover} onValueChange={(cover) => onChange({ ...filters, cover: cover as LoreFilters['cover'] })}>
              <SelectTrigger className="w-full min-w-0" aria-label={t('lore.library.coverFilter')}><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                {(['all', 'with', 'without'] as const).map((value) => <SelectItem key={value} value={value}>{t(`lore.library.cover.${value}`)}</SelectItem>)}
              </SelectGroup></SelectContent>
            </Select>
          </Field>
          <FieldSet className="col-span-2 min-w-0 gap-2">
            <FieldLegend variant="label">{t('settingPanel.field.tags')}</FieldLegend>
            <FieldDescription>{t('lore.filters.tagsHint')}</FieldDescription>
            <Input value={tagQuery} onChange={(event) => setTagQuery(event.target.value)}
              placeholder={t('lore.filters.searchTags')} aria-label={t('lore.filters.searchTags')} />
            <FieldGroup className="max-h-40 gap-1 overflow-y-auto">
              <FieldLabel className="w-full cursor-pointer py-1">
                <Checkbox checked={filters.tags === 'untagged'}
                  onCheckedChange={(checked) => onChange({ ...filters, tags: checked ? 'untagged' : 'all' })} />
                {t('lore.filters.untagged')}
              </FieldLabel>
              {tags.filter(([tag]) => `${tag} ${tagLabel(tag)}`.toLocaleLowerCase().includes(tagQuery.trim().toLocaleLowerCase()))
                .map(([tag, count]) => (
                  <FieldLabel key={tag} className="w-full min-w-0 cursor-pointer py-1">
                    <Checkbox checked={Array.isArray(filters.tags) && filters.tags.includes(tag)} onCheckedChange={() => toggleTag(tag)} />
                    <span className="min-w-0 flex-1 break-all">{tagLabel(tag)}</span>
                    <span className="text-muted-foreground">{count}</span>
                  </FieldLabel>
                ))}
            </FieldGroup>
          </FieldSet>
        </FieldGroup>
      </PopoverContent>
    </Popover>
  )
}

/** Removable conditions remain visible after the popover closes, including an empty result. */
export function LoreFilterSummary({ projectId, filters, onChange, query, onQueryChange, matched, total }: LoreFilterProps & {
  query: string
  onQueryChange: (query: string) => void
  matched: number
  total: number
}) {
  const { t } = useTranslation()
  const { sections: categorySections } = useLoreCategories(projectId)
  const chips: Array<{ id: string; label: string; clear: () => void }> = []
  if (query.trim()) chips.push({ id: 'query', label: t('lore.filters.searchChip', { query }), clear: () => onQueryChange('') })
  if (filters.category !== 'all') chips.push({ id: 'category', label: categorySections.find((section) => section.id === filters.category)?.name || t(`lore.type.${filters.category}`), clear: () => onChange({ ...filters, category: 'all' }) })
  if (filters.loadMode !== 'all') chips.push({ id: 'loadMode', label: t(`lore.loadMode.${filters.loadMode}`), clear: () => onChange({ ...filters, loadMode: 'all' }) })
  if (filters.enabled !== 'all') chips.push({ id: 'enabled', label: t(`lore.filters.enabled.${filters.enabled}`), clear: () => onChange({ ...filters, enabled: 'all' }) })
  if (filters.importance !== 'all') chips.push({ id: 'importance', label: t(`lore.importance.${filters.importance}`), clear: () => onChange({ ...filters, importance: 'all' }) })
  if (filters.cover !== 'all') chips.push({ id: 'cover', label: t(`lore.library.cover.${filters.cover}`), clear: () => onChange({ ...filters, cover: 'all' }) })
  if (filters.tags === 'untagged') chips.push({ id: 'tags', label: t('lore.filters.untagged'), clear: () => onChange({ ...filters, tags: 'all' }) })
  if (Array.isArray(filters.tags)) {
    const selected = filters.tags
    selected.forEach((tag) => chips.push({
      id: `tag:${tag}`,
      label: t('lore.filters.tagChip', { tag: isLoreProtagonistTag(tag) ? t('lore.library.protagonist') : tag }),
      clear: () => {
        const next = selected.filter((value) => value !== tag)
        onChange({ ...filters, tags: next.length ? next : 'all' })
      },
    }))
  }
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5" aria-label={t('lore.filters.active')}>
      <span className="text-xs text-muted-foreground" aria-live="polite">{t('lore.filters.results', { matched, total })}</span>
      {chips.map((chip) => (
        <Badge asChild variant="secondary" key={chip.id} className="h-6 min-w-0 max-w-full">
          <button type="button" onClick={chip.clear} title={chip.label} aria-label={t('lore.filters.remove', { filter: chip.label })}>
            <span className="truncate">{chip.label}</span><X data-icon="inline-end" className="shrink-0" />
          </button>
        </Badge>
      ))}
      {chips.length > 0 && <Button variant="ghost" size="sm" onClick={() => { onChange(EMPTY_LORE_FILTERS); onQueryChange('') }}>
        {t('lore.library.clearFilters')}
      </Button>}
    </div>
  )
}
