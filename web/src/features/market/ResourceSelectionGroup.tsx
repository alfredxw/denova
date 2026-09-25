import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { FieldLegend, FieldSet } from '@/components/ui/field'
import { Separator } from '@/components/ui/separator'

// Shared presentation for import and export; each picker owns its selection scope.
export function ResourceSelectionGroup({
  value,
  label,
  count,
  total,
  selectionLabel,
  disabled,
  onCheckedChange,
  children,
}: {
  value: string
  label: string
  count: number
  total: number
  selectionLabel: string
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const countID = useId()
  return (
    <AccordionItem value={value} asChild>
      <FieldSet className="min-w-0 gap-0 px-3">
        <FieldLegend className="sr-only">{label}</FieldLegend>
        <div className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-3">
          <Checkbox
            aria-label={selectionLabel}
            checked={
              count === total ? true : count > 0 ? 'indeterminate' : false
            }
            disabled={disabled}
            onCheckedChange={(checked) => onCheckedChange(checked === true)}
          />
          <AccordionTrigger
            aria-label={label}
            aria-describedby={countID}
            className="min-w-0 items-center gap-3 py-3"
          >
            <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">
              {label}
            </span>
            <Badge
              variant={count > 0 ? 'secondary' : 'outline'}
              aria-hidden="true"
            >
              {count} / {total}
            </Badge>
            <span id={countID} className="sr-only">
              {t('market.export.groupCount', { count, total })}
            </span>
          </AccordionTrigger>
        </div>
        <AccordionContent className="pb-3">
          <Separator className="mb-2" />
          <div className="min-w-0 pl-7">{children}</div>
        </AccordionContent>
      </FieldSet>
    </AccordionItem>
  )
}
