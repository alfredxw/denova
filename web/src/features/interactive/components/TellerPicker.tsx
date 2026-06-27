import { useMemo, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import type { StorySummary, Teller } from '../types'

interface TellerPickerProps {
  story?: StorySummary
  tellers: Teller[]
  onChange: (tellerId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function TellerPicker({ story, tellers, onChange, layout = 'inline' }: TellerPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const sidebar = layout === 'sidebar'
  const selectedTeller = useMemo(
    () => tellers.find((teller) => teller.id === story?.story_teller_id) || null,
    [story?.story_teller_id, tellers],
  )

  const selectTeller = (tellerId: string) => {
    setOpen(false)
    if (tellerId !== story?.story_teller_id) onChange(tellerId)
  }

  const selector = (
    <Popover open={open} onOpenChange={(nextOpen) => setOpen(Boolean(story) && nextOpen)}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!story}
          className={`nova-field ${sidebar ? 'w-full' : 'w-[170px]'} justify-between px-3 py-0.5 text-xs font-normal text-[var(--nova-text)] focus:ring-0`}
          aria-label={t('tellerPicker.placeholder')}
          aria-expanded={open}
        >
          <span className="min-w-0 flex-1 truncate text-left">{selectedTeller?.name || t('tellerPicker.placeholder')}</span>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${open ? 'rotate-180' : ''}`} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        className={`${sidebar ? 'w-[min(calc(100vw-2rem),22rem)]' : 'w-[169px]'} max-h-[min(70dvh,28rem)] overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)] shadow-[var(--nova-shadow)]`}
      >
        <div role="listbox" aria-label={t('tellerPicker.placeholder')} className="space-y-1">
          {tellers.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{t('tellerPicker.placeholder')}</div>
          ) : (
            tellers.map((teller) => {
              const selected = teller.id === story?.story_teller_id
              return (
                <button
                  key={teller.id}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  className={`flex w-full min-w-0 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-xs leading-5 ${selected ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
                  onClick={() => selectTeller(teller.id)}
                >
                  <span className="min-w-0 flex-1 truncate">{teller.name}</span>
                  {selected ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
                </button>
              )
            })
          )}
        </div>
      </PopoverContent>
    </Popover>
  )

  if (sidebar) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('tellerPicker.label')}</span>
        {selector}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-1.5">
      <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('tellerPicker.label')}</span>
      {selector}
    </div>
  )
}
