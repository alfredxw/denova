import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { AtSign, BookOpen, FileText, Hash, Palette } from 'lucide-react'
import { CommandGroup, CommandItem } from '@/components/ui/command'
import { useTranslation } from 'react-i18next'
import { INPUT_SUGGESTION_GROUP_CLASS_NAME, InputSuggestionMenu, inputSuggestionItemClassName } from './InputSuggestionMenu'

export interface ReferencePickerItem {
  value: string
  label: string
  description?: string
  kind?: 'file' | 'lore' | 'style'
}

export interface FileReferencePickerHandle {
  moveActive: (direction: 1 | -1) => boolean
  selectActive: () => boolean
}

interface FileReferencePickerProps {
  open: boolean
  query: string
  items: Array<string | ReferencePickerItem>
  onSelect: (item: ReferencePickerItem) => void
  trigger?: '@' | '#'
  placeholder?: string
  emptyText?: string
  heading?: string
}

/** Compact composer picker for @ references and # scene styles. */
export const FileReferencePicker = forwardRef<FileReferencePickerHandle, FileReferencePickerProps>(function FileReferencePicker({
  open,
  query,
  items,
  onSelect,
  trigger = '@',
  placeholder,
  emptyText,
  heading,
}: FileReferencePickerProps, ref) {
  const { t } = useTranslation()
  const placeholderText = placeholder ?? t('chat.fileReference.placeholder')
  const emptyLabel = emptyText ?? t('chat.fileReference.empty')
  const headingLabel = heading ?? t('chat.fileReference.heading')
  const normalizedQuery = query.toLowerCase()
  const visibleItems = items
    .map((item) => normalizeItem(item, trigger))
    .filter((file) => `${file.label}\n${file.value}\n${file.description || ''}`.toLowerCase().includes(normalizedQuery))
    .slice(0, 30)
  const [activeIndex, setActiveIndex] = useState(0)
  const itemRefs = useRef<Array<HTMLDivElement | null>>([])
  const activeItem = visibleItems[activeIndex] ?? visibleItems[0]

  useEffect(() => {
    if (!open) return
    setActiveIndex(0)
  }, [open, query])

  useEffect(() => {
    if (activeIndex >= visibleItems.length) setActiveIndex(0)
  }, [activeIndex, visibleItems.length])

  useEffect(() => {
    if (!open) return
    itemRefs.current[activeIndex]?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex, open, visibleItems.length])

  useImperativeHandle(ref, () => ({
    moveActive(direction) {
      if (!open || visibleItems.length === 0) return false
      setActiveIndex((current) => (current + direction + visibleItems.length) % visibleItems.length)
      return true
    },
    selectActive() {
      if (!open || !activeItem) return false
      onSelect(activeItem)
      return true
    },
  }), [activeItem, onSelect, open, visibleItems.length])

  return (
    <InputSuggestionMenu
      open={open}
      value={activeItem ? itemKey(activeItem) : undefined}
      onValueChange={(value) => {
        const index = visibleItems.findIndex((item) => itemKey(item) === value)
        if (index >= 0) setActiveIndex(index)
      }}
      icon={trigger === '#' ? Hash : AtSign}
      title={headingLabel}
      description={placeholderText}
      shortcut={trigger}
      emptyText={emptyLabel}
    >
      <CommandGroup className={INPUT_SUGGESTION_GROUP_CLASS_NAME}>
        {visibleItems.map((file, index) => {
          const active = index === activeIndex
          const ItemIcon = file.kind === 'lore' ? BookOpen : file.kind === 'style' ? Palette : FileText
          const kindLabel = file.kind === 'lore'
            ? t('chat.fileReference.kindLore')
            : file.kind === 'style'
              ? t('chat.fileReference.kindStyle')
              : t('chat.fileReference.kindFile')
          return (
            <CommandItem
              key={itemKey(file)}
              ref={(element) => { itemRefs.current[index] = element }}
              value={itemKey(file)}
              data-reference-kind={file.kind}
              onMouseEnter={() => setActiveIndex(index)}
              onSelect={() => onSelect(file)}
              aria-label={[`${trigger}${file.label}`, file.description, kindLabel].filter(Boolean).join(' · ')}
              className={inputSuggestionItemClassName(active)}
            >
              <span className={`flex size-6 shrink-0 items-center justify-center rounded-md ${
                active ? 'bg-[var(--nova-surface-2)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)]'
              }`}>
                <ItemIcon className="size-3.5" />
              </span>
              <span className="max-w-[42%] shrink-0 truncate text-xs font-medium text-[var(--nova-text)]" title={file.label}>
                {trigger}{file.label}
              </span>
              <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text-muted)]" title={file.description}>
                {file.description}
              </span>
              <span className="ml-1 shrink-0 text-[11px] text-[var(--nova-text-faint)]">{kindLabel}</span>
            </CommandItem>
          )
        })}
      </CommandGroup>
    </InputSuggestionMenu>
  )
})

function normalizeItem(item: string | ReferencePickerItem, trigger: '@' | '#'): ReferencePickerItem {
  return typeof item === 'string'
    ? { value: item, label: item, kind: trigger === '#' ? 'style' : 'file' }
    : { ...item, kind: item.kind ?? (trigger === '#' ? 'style' : 'file') }
}

function itemKey(item: ReferencePickerItem) {
  return `${item.kind || 'file'}:${item.value}`
}
