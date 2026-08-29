import type { ComponentProps, ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

type DetailTone = 'muted' | 'faint'

interface ComposerMenuRowContentProps {
  icon: LucideIcon
  iconClassName?: string
  label: ReactNode
  detail?: ReactNode
  detailTone?: DetailTone
}

type ComposerMenuItemProps = ComposerMenuRowContentProps
  & Omit<ComponentProps<typeof DropdownMenuItem>, 'children'>

type ComposerMenuCheckboxItemProps = ComposerMenuRowContentProps
  & Omit<ComponentProps<typeof DropdownMenuCheckboxItem>, 'children'>

type ComposerMenuSubTriggerProps = ComposerMenuRowContentProps
  & Omit<ComponentProps<typeof DropdownMenuSubTrigger>, 'children'>

const rowClassName = 'grid min-h-8 cursor-pointer grid-cols-[1.25rem_minmax(0,1fr)_minmax(0,auto)_1rem] items-center gap-x-2 px-2 py-1 text-sm [&>svg:first-of-type]:justify-self-center'

/** Keeps every top-level composer action on the same visual columns. */
export function ComposerMenuItem({ icon, iconClassName, label, detail, detailTone, className, ...props }: ComposerMenuItemProps) {
  return (
    <DropdownMenuItem className={cn(rowClassName, className)} {...props}>
      <ComposerMenuRowContent icon={icon} iconClassName={iconClassName} label={label} detail={detail} detailTone={detailTone} />
    </DropdownMenuItem>
  )
}

/** Uses the final column for the checkbox indicator without shifting metadata. */
export function ComposerMenuCheckboxItem({ icon, iconClassName, label, detail, detailTone, className, ...props }: ComposerMenuCheckboxItemProps) {
  return (
    <DropdownMenuCheckboxItem
      className={cn(
        rowClassName,
        'pr-2 [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:static [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:col-start-4 [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:row-start-1 [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:place-self-center',
        className,
      )}
      {...props}
    >
      <ComposerMenuRowContent icon={icon} iconClassName={iconClassName} label={label} detail={detail} detailTone={detailTone} />
    </DropdownMenuCheckboxItem>
  )
}

/** Pins the submenu chevron to the same final column as checkbox indicators. */
export function ComposerMenuSubTrigger({ icon, iconClassName, label, detail, detailTone, className, ...props }: ComposerMenuSubTriggerProps) {
  return (
    <DropdownMenuSubTrigger
      className={cn(
        rowClassName,
        '[&>svg:last-child]:col-start-4 [&>svg:last-child]:row-start-1 [&>svg:last-child]:ml-0 [&>svg:last-child]:justify-self-center',
        className,
      )}
      {...props}
    >
      <ComposerMenuRowContent icon={icon} iconClassName={iconClassName} label={label} detail={detail} detailTone={detailTone} />
    </DropdownMenuSubTrigger>
  )
}

export function ComposerMenuShortcut({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
      {children}
    </kbd>
  )
}

function ComposerMenuRowContent({ icon: Icon, iconClassName, label, detail, detailTone = 'muted' }: ComposerMenuRowContentProps) {
  return (
    <>
      <Icon aria-hidden className={iconClassName} />
      <span className="min-w-0 truncate">{label}</span>
      {detail !== undefined ? (
        <span className={cn(
          'max-w-36 justify-self-end truncate text-xs font-normal',
          detailTone === 'faint' ? 'text-[var(--nova-text-faint)]' : 'text-muted-foreground',
        )}>
          {detail}
        </span>
      ) : null}
    </>
  )
}
