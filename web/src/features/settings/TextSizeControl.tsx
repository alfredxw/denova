import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Slider } from '@/components/ui/slider'
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

/** A discrete, named text-size control; persisted pixel values remain an implementation detail. */
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
      <div className="flex items-center gap-2.5">
        <span aria-hidden="true" className="shrink-0 text-[var(--nova-ui-micro-font-size)] font-medium text-[var(--nova-text-faint)]">
          A
        </span>
        <div className="min-w-28 flex-1">
          <Slider
            value={[selectedIndex]}
            min={0}
            max={steps.length - 1}
            step={1}
            disabled={disabled}
            aria-label={ariaLabel}
            aria-describedby={labelId}
            aria-valuetext={t('settings.textSize.ariaValue', {
              label: selectedLabel,
              position: selectedIndex + 1,
              count: steps.length,
            })}
            onValueChange={([index]) => {
              const nextValue = steps[index]
              if (nextValue !== undefined) onValueChange(nextValue)
            }}
          />
          <div aria-hidden="true" className="mt-1 flex justify-between px-1.5">
            {steps.map((step, index) => (
              <span
                key={step}
                className={cn(
                  'size-1 rounded-full bg-[var(--nova-border)]',
                  index === defaultIndex && 'ring-1 ring-[var(--nova-text-faint)] ring-offset-1 ring-offset-[var(--nova-surface)]',
                )}
              />
            ))}
          </div>
        </div>
        <span aria-hidden="true" className="shrink-0 text-[var(--nova-ui-large-font-size)] font-semibold text-[var(--nova-text-muted)]">
          A
        </span>
        <span
          id={labelId}
          aria-live="polite"
          className="min-w-12 shrink-0 text-right text-[var(--nova-ui-compact-font-size)] text-[var(--nova-text-muted)]"
        >
          {selectedLabel}
        </span>
      </div>
    </div>
  )
}
