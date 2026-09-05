import { createContext, lazy, Suspense, useContext, useState, type ReactNode } from 'react'
import { Maximize2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AskChatMessage, ToolCallChatMessage, ToolResultChatMessage } from '@/lib/api'
import { Dialog, DialogTrigger } from '@/components/ui/dialog'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { cn } from '@/lib/utils'

export type InspectableToolMessage = ToolCallChatMessage | ToolResultChatMessage | AskChatMessage

const InspectorContext = createContext(false)
const ToolInspectorDialog = lazy(() => import('./ToolInspectorDialog'))

/** Owns inspection across streaming renderer changes without copying message state. */
export function ToolInspector({ message, projectId, children }: { message: InspectableToolMessage; projectId: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <InspectorContext.Provider value={true}>{children}</InspectorContext.Provider>
      {open && (
        <Suspense fallback={null}>
          <ToolInspectorDialog message={message} projectId={projectId} />
        </Suspense>
      )}
    </Dialog>
  )
}

/** Collapsible cards expose this secondary action only while their content is open. */
export function ToolInspectorButton({ className }: { className?: string }) {
  const available = useContext(InspectorContext)
  const { t } = useTranslation()
  if (!available) return null
  return (
    <DialogTrigger asChild>
      <TooltipIconButton label={t('chat.tool.inspector.open')} tooltipSide="top" className={cn('shrink-0 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]', className)} onClick={(event) => event.stopPropagation()}>
        <Maximize2 />
      </TooltipIconButton>
    </DialogTrigger>
  )
}
