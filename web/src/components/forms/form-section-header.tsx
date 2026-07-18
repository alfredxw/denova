import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

interface FormSectionHeaderProps {
  title: ReactNode
  description?: ReactNode
  icon?: LucideIcon
  action?: ReactNode
}

/** Shared heading for grouped feature-form sections. */
export function FormSectionHeader({ title, description, icon: Icon, action }: FormSectionHeaderProps) {
  return (
    <div className="flex min-w-0 items-start gap-2">
      {Icon ? <Icon className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" /> : null}
      <div className="min-w-0 flex-1">
        <h3 className="text-xs font-medium text-foreground">{title}</h3>
        {description ? <p className="mt-1 text-[11px] leading-5 text-muted-foreground">{description}</p> : null}
      </div>
      {action}
    </div>
  )
}
