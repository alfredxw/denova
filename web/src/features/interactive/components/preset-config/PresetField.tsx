import type { ReactNode } from 'react'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { cn } from '@/lib/utils'

interface PresetFieldProps {
  label: ReactNode
  children: ReactNode
  htmlFor?: string
  error?: ReactNode
  errorId?: string
  className?: string
}

/** Accessible field wrapper shared by all interactive preset editors. */
export function PresetField({ label, children, htmlFor, error, errorId, className }: PresetFieldProps) {
  return (
    <Field data-invalid={Boolean(error)} className={cn('min-w-0 gap-1.5 text-xs', className)}>
      {htmlFor ? (
        <>
          <FieldLabel htmlFor={htmlFor} className="min-w-0 truncate font-normal text-[11px] text-[var(--nova-text-faint)]">
            {label}
          </FieldLabel>
          {children}
        </>
      ) : (
        <FieldLabel className="grid w-full min-w-0 gap-1.5 font-normal text-[var(--nova-text-muted)]">
          <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
          {children}
        </FieldLabel>
      )}
      {error ? <FieldError id={errorId} className="text-[11px]">{error}</FieldError> : null}
    </Field>
  )
}
