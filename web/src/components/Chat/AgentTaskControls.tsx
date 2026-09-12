import { Pause, Play, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AgentTaskControlsProps {
  active: boolean
  suspended: boolean
  pending?: boolean
  onSuspend?: () => void
  onResume?: () => void
  onAbort?: () => void
}

/** Controls the durable task while the composer keeps its existing input flow. */
export function AgentTaskControls({ active, suspended, pending, onSuspend, onResume, onAbort }: AgentTaskControlsProps) {
  const { t } = useTranslation()
  if (!active && !suspended) return null
  return <div className="flex flex-wrap items-center gap-2" aria-busy={pending || undefined}>
    {suspended ? <>
      <span className="text-sm text-muted-foreground" role="status">{t('chat.runtime.suspended')}</span>
      <Button size="sm" variant="outline" disabled={pending} onClick={onResume}><Play data-icon="inline-start" />{t('chat.runtime.resume')}</Button>
      <Button size="sm" variant="ghost" disabled={pending} onClick={onAbort}><Square data-icon="inline-start" />{t('chat.runtime.abort')}</Button>
    </> : onSuspend ? <Button size="sm" variant="outline" disabled={pending} onClick={onSuspend}><Pause data-icon="inline-start" />{t('chat.runtime.suspend')}</Button> : null}
  </div>
}
