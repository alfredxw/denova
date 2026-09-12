import { useEffect, useState, type ReactNode } from 'react'
import { WRITING_AGENT_INIT_EVENT } from '@/features/onboarding/events'
import { FolderOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MobileWorkspaceHeader } from '@/components/layout/mobile-workspace-header'
import { MOBILE_PROJECT_OPEN_EVENT, MOBILE_WRITING_VIEW_EVENT, type MobileWritingView } from '@/components/layout/mobile-pane-events'

/** Mobile view selection is local and does not persist desktop panel geometry. */
export function MobileWritingWorkspace({ editor, agent }: { editor: ReactNode; agent: ReactNode }) {
  const { t } = useTranslation()
  const [view, setView] = useState<MobileWritingView>('editor')
  useEffect(() => {
    const openAgent = () => setView('agent')
    const changeView = (event: Event) => setView((event as CustomEvent<MobileWritingView>).detail)
    window.addEventListener(MOBILE_WRITING_VIEW_EVENT, changeView)
    window.addEventListener(WRITING_AGENT_INIT_EVENT, openAgent)
    return () => {
      window.removeEventListener(MOBILE_WRITING_VIEW_EVENT, changeView)
      window.removeEventListener(WRITING_AGENT_INIT_EVENT, openAgent)
    }
  }, [])
  return (
    <Tabs value={view} onValueChange={(value) => { if (value === 'editor' || value === 'agent') setView(value) }} className="flex h-full min-h-0 flex-col gap-0">
      <MobileWorkspaceHeader route="writing">
        <div className="flex min-w-0 flex-1 justify-center">
          <TabsList className="nova-mobile-view-tabs" aria-label={t('workbench.mobile.writingViews')}>
            <TabsTrigger value="editor" className="min-w-20 px-4">{t('workbench.mobile.editor')}</TabsTrigger>
            <TabsTrigger value="agent" className="min-w-20 px-4">{t('workbench.mobile.agent')}</TabsTrigger>
          </TabsList>
        </div>
        <Button variant="ghost" size="icon" aria-label={t('workbench.mobile.files')} onClick={() => window.dispatchEvent(new Event(MOBILE_PROJECT_OPEN_EVENT))}>
          <FolderOpen />
        </Button>
      </MobileWorkspaceHeader>
      <TabsContent value="editor" forceMount className="mt-0 min-h-0 flex-1 data-[state=inactive]:hidden">{editor}</TabsContent>
      <TabsContent value="agent" forceMount className="mt-0 min-h-0 flex-1 data-[state=inactive]:hidden">{agent}</TabsContent>
    </Tabs>
  )
}
