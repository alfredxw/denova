import { useId, type ReactNode } from 'react'
import { Field, FieldDescription, FieldError, FieldLabel, FieldTitle } from '@/components/ui/field'
import { cn } from '@/lib/utils'

interface FormFieldProps {
  label: ReactNode
  children: ReactNode
  htmlFor?: string
  semanticGroup?: boolean
  description?: ReactNode
  error?: ReactNode
  className?: string
}

/** Vertical form field used by feature editors; controls remain caller-owned. */
export function FormField({ label, children, htmlFor, semanticGroup = false, description, error, className }: FormFieldProps) {
  const labelId = useId()
  const descriptionId = useId()
  const errorId = useId()
  const describedBy = [description ? descriptionId : '', error ? errorId : ''].filter(Boolean).join(' ') || undefined
  const namesGroup = !htmlFor && semanticGroup

  return (
    <Field
      role={namesGroup ? 'group' : undefined}
      aria-labelledby={namesGroup ? labelId : undefined}
      aria-describedby={namesGroup ? describedBy : undefined}
      data-invalid={Boolean(error)}
      className={cn('min-w-0 gap-1.5 text-xs', className)}
    >
      {htmlFor
        ? <FieldLabel htmlFor={htmlFor} className="text-[11px] text-muted-foreground">{label}</FieldLabel>
        : <FieldTitle id={namesGroup ? labelId : undefined} className="text-[11px] font-normal text-muted-foreground">{label}</FieldTitle>}
      {children}
      {description ? <FieldDescription id={descriptionId} className="text-[11px]">{description}</FieldDescription> : null}
      {error ? <FieldError id={errorId} className="text-[11px]">{error}</FieldError> : null}
    </Field>
  )
}
