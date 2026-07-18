import type { LucideIcon } from 'lucide-react'
import { Check, Loader2 } from 'lucide-react'
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

export interface PersistedSettingsMenuOption {
  id: string
  label: string
  meta?: string
}

interface PersistedSettingsMenuSubProps {
  icon: LucideIcon
  label: string
  title: string
  currentLabel: string
  value: string
  options: PersistedSettingsMenuOption[]
  saving?: boolean
  disabled?: boolean
  emptyLabel?: string
  onValueChange: (value: string) => void | Promise<unknown>
}

/** Shared dropdown presentation for composer settings persisted by a caller-owned hook. */
export function PersistedSettingsMenuSub({
  icon: Icon,
  label,
  title,
  currentLabel,
  value,
  options,
  saving = false,
  disabled = false,
  emptyLabel,
  onValueChange,
}: PersistedSettingsMenuSubProps) {
  return (
    <>
      <DropdownMenuGroup>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger disabled={disabled || saving} className="cursor-pointer text-xs" title={title} aria-label={label}>
            <Icon />
            <span className="min-w-0 flex-1">{label}</span>
            <span className="max-w-36 truncate text-[10px] text-muted-foreground">{currentLabel}</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="w-72 p-2 text-xs">
            <DropdownMenuGroup>
              {options.map((option) => {
                const selected = option.id === value
                return (
                  <DropdownMenuItem
                    key={option.id}
                    disabled={saving}
                    onSelect={(event) => {
                      event.preventDefault()
                      void onValueChange(option.id)
                    }}
                  >
                    {saving && selected ? <Loader2 className="animate-spin" /> : <Check className={cn(selected ? 'opacity-100' : 'opacity-0')} />}
                    <span className="min-w-0 flex-1 truncate">{option.label}</span>
                    {option.meta ? <span className="text-[10px] text-muted-foreground">{option.meta}</span> : null}
                  </DropdownMenuItem>
                )
              })}
              {options.length === 0 && emptyLabel ? <DropdownMenuItem disabled>{emptyLabel}</DropdownMenuItem> : null}
            </DropdownMenuGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
    </>
  )
}
