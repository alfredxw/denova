import { Play, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AgentTaskControlsProps {
  suspended: boolean
  pending?: boolean
  onResume?: () => void
  onAbort?: () => void
}

/** Offers recovery actions after a task is paused; the composer owns stopping output. */
export function AgentTaskControls({ suspended, pending, onResume, onAbort }: AgentTaskControlsProps) {
  const { t } = useTranslation()
  if (!suspended) return null
  return <div className="flex flex-wrap items-center gap-2" aria-busy={pending || undefined}>
    <span className="text-sm text-muted-foreground" role="status">{t('chat.runtime.suspended')}</span>
    <Button size="sm" variant="outline" disabled={pending} onClick={onResume}><Play data-icon="inline-start" />{t('chat.runtime.resume')}</Button>
    <Button size="sm" variant="ghost" disabled={pending} onClick={onAbort}><Square data-icon="inline-start" />{t('chat.runtime.abort')}</Button>
  </div>
}
