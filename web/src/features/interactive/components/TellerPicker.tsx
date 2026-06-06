import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useTranslation } from 'react-i18next'
import type { StorySummary, Teller } from '../types'

interface TellerPickerProps {
  story?: StorySummary
  tellers: Teller[]
  onChange: (tellerId: string) => void
  layout?: 'inline' | 'sidebar'
}

export function TellerPicker({ story, tellers, onChange, layout = 'inline' }: TellerPickerProps) {
  const { t } = useTranslation()
  const sidebar = layout === 'sidebar'
  const selector = (
    <Select value={story?.story_teller_id || '__none'} disabled={!story} onValueChange={(value) => { if (value !== '__none') onChange(value) }}>
      <SelectTrigger
        size="sm"
        className={`nova-field ${sidebar ? 'w-full' : 'w-[170px]'} px-3 py-0.5 text-xs focus:ring-0`}
        aria-label={t('tellerPicker.placeholder')}
      >
        <SelectValue placeholder={t('tellerPicker.placeholder')} />
      </SelectTrigger>
      <SelectContent className="nova-panel border text-[var(--nova-text)]">
        <SelectItem value="__none" disabled>{t('tellerPicker.placeholder')}</SelectItem>
        {tellers.map((teller) => (
          <SelectItem key={teller.id} value={teller.id}>{teller.name}</SelectItem>
        ))}
      </SelectContent>
    </Select>
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
