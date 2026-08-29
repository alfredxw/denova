import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { cn } from '@/lib/utils'
import { nearestFontSizeStepIndex, TEXT_SIZE_LABEL_KEYS } from './font-size-steps'

interface TextSizeControlProps {
  value: number | null | undefined
  steps: readonly number[]
  defaultValue: number
  ariaLabel: string
  disabled?: boolean
  className?: string
  onValueChange: (value: number) => void
}

/** A discrete text-size picker that previews every option at its resulting size. */
export function TextSizeControl({
  value,
  steps,
  defaultValue,
  ariaLabel,
  disabled,
  className,
  onValueChange,
}: TextSizeControlProps) {
  const { t } = useTranslation()
  const labelId = useId()
  const selectedIndex = nearestFontSizeStepIndex(value, steps, defaultValue)
  const defaultIndex = nearestFontSizeStepIndex(defaultValue, steps, defaultValue)
  const selectedLabel = t(TEXT_SIZE_LABEL_KEYS[selectedIndex] ?? TEXT_SIZE_LABEL_KEYS[defaultIndex])

  return (
    <div className={cn('w-full min-w-0', className)}>
      <ToggleGroup
        type="single"
        variant="outline"
        spacing={0}
        value={String(selectedIndex)}
        aria-label={ariaLabel}
        aria-describedby={labelId}
        className="grid w-full grid-cols-7"
        onValueChange={(indexValue) => {
          if (!indexValue) return
          const nextValue = steps[Number(indexValue)]
          if (nextValue !== undefined) onValueChange(nextValue)
        }}
      >
        {steps.map((step, index) => {
          const optionLabel = t(TEXT_SIZE_LABEL_KEYS[index] ?? TEXT_SIZE_LABEL_KEYS[defaultIndex])
          const accessibleLabel = t('settings.textSize.ariaValue', {
            label: optionLabel,
            position: index + 1,
            count: steps.length,
          })

          return (
            <ToggleGroupItem
              key={step}
              value={String(index)}
              disabled={disabled}
              aria-label={accessibleLabel}
              title={optionLabel}
              className="h-12 min-w-0 px-0"
            >
              <span aria-hidden="true" style={{ fontSize: `${step}px`, lineHeight: 1 }}>
                Aa
              </span>
            </ToggleGroupItem>
          )
        })}
      </ToggleGroup>
      <div
        id={labelId}
        aria-live="polite"
        className="mt-1.5 text-right text-[var(--nova-ui-compact-font-size)] text-[var(--nova-text-muted)]"
      >
        {selectedLabel}
      </div>
    </div>
  )
}
