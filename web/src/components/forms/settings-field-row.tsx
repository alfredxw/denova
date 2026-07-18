import type { ReactNode } from 'react'
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from '@/components/ui/field'
import { cn } from '@/lib/utils'

interface SettingsFieldRowProps {
  title: ReactNode
  description?: ReactNode
  meta?: ReactNode
  children: ReactNode
  disabled?: boolean
  invalid?: boolean
  className?: string
  htmlFor?: string
  contentClassName?: string
  controlClassName?: string
}

/** Responsive label/control row shared by user, workspace and agent settings. */
export function SettingsFieldRow({
  title,
  description,
  meta,
  children,
  disabled,
  invalid,
  className,
  htmlFor,
  contentClassName,
  controlClassName,
}: SettingsFieldRowProps) {
  return (
    <Field
      orientation="horizontal"
      data-disabled={disabled || undefined}
      data-invalid={invalid || undefined}
      className={cn('flex-col items-stretch rounded-lg border border-border bg-background p-3 sm:flex-row sm:items-start', className)}
    >
      <FieldContent className={contentClassName}>
        <div className="flex flex-wrap items-center gap-2">
          {htmlFor ? <FieldLabel htmlFor={htmlFor}>{title}</FieldLabel> : <FieldTitle>{title}</FieldTitle>}
          {meta}
        </div>
        {description ? <FieldDescription className="text-[11px]">{description}</FieldDescription> : null}
      </FieldContent>
      <div className={cn('w-full min-w-0 sm:w-auto', controlClassName)}>{children}</div>
    </Field>
  )
}
