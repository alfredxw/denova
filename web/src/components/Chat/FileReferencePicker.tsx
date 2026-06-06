import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useTranslation } from 'react-i18next'

export interface ReferencePickerItem {
  value: string
  label: string
  description?: string
}

interface FileReferencePickerProps {
  open: boolean
  query: string
  files: Array<string | ReferencePickerItem>
  onSelect: (path: string) => void
  trigger?: '@' | '#'
  placeholder?: string
  emptyText?: string
  heading?: string
}

/** 文件引用选择器，用于在 Chat 输入框中补全 @path。 */
export function FileReferencePicker({
  open,
  query,
  files,
  onSelect,
  trigger = '@',
  placeholder,
  emptyText,
  heading,
}: FileReferencePickerProps) {
  const { t } = useTranslation()
  const placeholderText = placeholder ?? t('chat.fileReference.placeholder')
  const emptyLabel = emptyText ?? t('chat.fileReference.empty')
  const headingLabel = heading ?? t('chat.fileReference.heading')
  const normalizedQuery = query.toLowerCase()
  const visibleFiles = files
    .map(normalizeItem)
    .filter((file) => `${file.label}\n${file.value}\n${file.description || ''}`.toLowerCase().includes(normalizedQuery))
    .slice(0, 30)

  return (
    <Popover open={open}>
      <PopoverTrigger asChild>
        <span className="absolute bottom-full left-0 h-0 w-0" />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side="top"
        className="mb-2 w-[360px] border-[#3a3d44] bg-[#25262a] p-0 text-[#d7dbe2]"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <Command shouldFilter={false} className="bg-transparent">
          <CommandInput value={query} readOnly placeholder={placeholderText} />
          <CommandList>
            <CommandEmpty>{emptyLabel}</CommandEmpty>
            <CommandGroup heading={headingLabel}>
              {visibleFiles.map((file) => (
                <CommandItem
                  key={file.value}
                  value={file.value}
                  onSelect={() => onSelect(file.value)}
                  className="flex cursor-pointer flex-col items-start gap-0.5"
                >
                  <span>{trigger}{file.label}</span>
                  {file.description && <span className="text-[11px] text-[#858b96]">{file.description}</span>}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function normalizeItem(item: string | ReferencePickerItem): ReferencePickerItem {
  return typeof item === 'string' ? { value: item, label: item } : item
}
