import { randomUUID } from '@/lib/uuid'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { Button } from '@/components/ui/button'
import { ArrowLeft, PanelRightClose, PanelRightOpen } from 'lucide-react'
import { LoreWorkspaceTab } from '@/features/lore/LoreWorkspaceTab'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { CollapsiblePanelSeparator, CollapsibleResizablePanel, PanelMotionGroup } from '@/components/layout/panel-motion'
import { resolvePanelInitialSize, usePersistedPanelLayout } from '@/components/layout/use-persisted-panel-layout'
import { useIsMobile } from '@/hooks/useIsMobile'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { useRemoteAccessLocal } from '@/features/remote-access/api'
import { isPluginPageRoute, type PluginPageRoute, type WorkspaceMode } from '@/stores/workspace-store'
import { GamePlayer, type GamePlayerHandle } from './GamePlayer'
import { PluginPageAgent, type PluginPageLoreReferences } from './PluginPageAgent'
import { usePluginPageLore } from './use-plugin-page-lore'
import type { PagePresentation } from './page-presentation'
import { openInstalledExtension } from './extension-navigation'
import {
  localized,
  management,
  platformError,
  releasePluginConsumer,
  type PluginPage,
  type ProjectPlugin,
  type RuntimeSnapshot,
} from './api'

import { usePluginPages } from './use-plugin-pages'

/** Navigation selects a destination; its consumer and Agent remain bound to the
 * same Project while the user visits other routes. Project changes revoke it. */
export function PluginPageWorkspace({
  projectId,
  route,
  visible,
  loreReferenceLabels,
  loreSuggestions,
  onFlushHandlerChange,
  onChooseProject,
}: {
  projectId?: string
  route: WorkspaceMode
  visible: boolean
  onChooseProject: () => void
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
} & PluginPageLoreReferences) {
  const previous = useRef<PluginPageRoute | undefined>(undefined)
  if (isPluginPageRoute(route)) previous.current = route
  const identity = previous.current?.slice('plugin:'.length)
  const pages = usePluginPages()
  const page = pages.data?.find((page) => `${page.pluginId}/${page.id}` === identity)
  const { t } = useTranslation()
  if (!projectId)
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6">
        <p>{t('platform.selectProject')}</p>
        <Button onClick={onChooseProject}>{t('platform.project')}</Button>
      </div>
    )
  if (!page) return <div className="p-6">{pages.isPending ? t('common.loading') : t('platform.pages.unavailable')}</div>
  return (
    <ProjectPluginPage
      key={`${projectId}/${identity}/${page.releaseId}`}
      page={page}
      projectId={projectId}
      visible={visible}
      loreReferenceLabels={loreReferenceLabels}
      loreSuggestions={loreSuggestions}
      onFlushHandlerChange={onFlushHandlerChange}
    />
  )
}

function ProjectPluginPage({ page, projectId, visible, loreReferenceLabels, loreSuggestions, onFlushHandlerChange }: {
  page: PluginPage; projectId: string; visible: boolean
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
} & PluginPageLoreReferences) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  // Plugin pages run in a loopback runtime; a LAN browser cannot host the frame.
  const hostLocal = useRemoteAccessLocal()
  const plugins = useQuery({
    queryKey: ['platform', 'project-plugins', projectId, 'general', i18n.language],
    queryFn: () =>
      management<ProjectPlugin[]>(
        `/projects/${projectId}/plugins?context=general&locale=${i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'}`,
      ),
  })
  const plugin = plugins.data?.find((item) => item.id === page.pluginId)
  const [runtime, setRuntime] = useState<RuntimeSnapshot>()
  const [error, setError] = useState('')
  const [agentBusy, setAgentBusy] = useState(false)
  const [activation, setActivation] = useState(0)
  const [pageContext, setPageContext] = useState<unknown>(null)
  const [presentation, setPresentation] = useState<PagePresentation>({})
  const [showAssistant, setShowAssistant] = useState(true)
  const [mobileAssistant, setMobileAssistant] = useState(false)
  const isMobile = useIsMobile()
  const assistantVisible = !!page.agent && (isMobile ? mobileAssistant : showAssistant)
  const showAgent = useCallback(() => { setShowAssistant(true); setMobileAssistant(true) }, [])
  const showContent = useCallback(() => setMobileAssistant(false), [])
  const lore = usePluginPageLore({ projectId, agentVisible: visible && assistantVisible, onShowAgent: showAgent, onShowContent: showContent, onFlushHandlerChange })
  const layoutKey = `nova-plugin-page-layout:${page.pluginId}/${page.id}`
  const layout = usePersistedPanelLayout({ storageKey: layoutKey, panelIds: ['main', 'right'] })
  const frame = useRef<GamePlayerHandle>(null)
  const ready = !!plugin?.enabled && !plugin.problem
  useEffect(() => {
    if (!ready || !hostLocal) return
    let disposed = false
    let opened: RuntimeSnapshot | undefined
    setError('')
    void management<RuntimeSnapshot>('/plugin-actions/open', 'POST', {
      projectId,
      pluginId: page.pluginId,
      releaseId: page.releaseId,
      context: 'general',
      kind: 'page',
      actionId: page.id,
      consumerId: randomUUID(),
      locale: i18n.language,
      theme: resolvedTheme,
    })
      .then(async (value) => {
        opened = value
        if (disposed) {
          await releasePluginConsumer(value)
          return
        }
        setRuntime(value)
      })
      .catch((error) => {
        if (!disposed) setError(platformError(error))
      })
    return () => {
      disposed = true
      setRuntime(undefined)
      if (opened)
        void releasePluginConsumer(opened).catch((error) => console.error('[plugins] release page failed', error))
    }
    // Appearance updates use the existing frame protocol, without restarting work.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, hostLocal, activation, page.id, page.pluginId, page.releaseId, projectId])
  const title = localized(page.title, i18n.language)
  return (
    <section className="relative flex h-full min-h-0 flex-col" aria-label={title}>
      {(error || plugins.error) && (
        <InlineErrorNotice className="absolute inset-x-3 top-12 z-40" message={error || platformError(plugins.error)} />
      )}
      {(!ready || !runtime) && <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6">
        <p>{plugins.isPending ? t('common.loading') : !hostLocal ? t('platform.errors.LOCAL_HOST_ONLY') : !plugin?.enabled ? t('platform.plugins.disabled') : plugin.problem ? t(plugin.problem.messageKey) : error || t('common.loading')}</p>
        {error && <Button variant="outline" onClick={() => setActivation(value => value + 1)}>{t('common.retry')}</Button>}
        <Button variant="link" onClick={() => openInstalledExtension('plugin', page.pluginId)}>{t('platform.pages.manage')}</Button>
      </div>}
      {runtime && ready ? (
        <>
          {page.agent && <Button variant="ghost" size="icon-sm"
            className="absolute right-2 top-1 z-40"
            aria-label={t(assistantVisible ? 'platform.pages.hideAgent' : 'platform.pages.showAgent')}
            aria-expanded={assistantVisible} aria-controls="plugin-page-agent"
            onClick={() => isMobile ? setMobileAssistant(value => !value) : setShowAssistant(value => !value)}>
            {assistantVisible ? <PanelRightClose /> : <PanelRightOpen />}
          </Button>}
          <PanelMotionGroup id={layoutKey} orientation="horizontal"
            className="h-full min-h-0 min-w-0 flex-1"
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            defaultLayout={!isMobile && page.agent ? layout.defaultLayout : undefined}
            onLayoutChanged={value => { if (!isMobile && assistantVisible) layout.persistUserLayout(value) }}>
            <CollapsibleResizablePanel id="main" visible={!isMobile || !assistantVisible} side="left"
              minSize={isMobile ? '0px' : '360px'} className="min-w-0" contentClassName="flex flex-col">
              <div hidden={lore.open} className="flex h-full min-h-0 flex-col [&[hidden]]:hidden">
                <GamePlayer ref={frame} runtime={runtime} title={title} visible={visible && !lore.open}
                  variant="plugin" showMenu={false} externalBusy={agentBusy}
                  onContext={setPageContext} onPresentation={setPresentation} onOpenLore={lore.openTab}
                  onStop={() => releasePluginConsumer(runtime)}
                  onExit={() => { setRuntime(undefined); openInstalledExtension('plugin', page.pluginId) }} />
              </div>
              {lore.open && <div className="flex h-full min-h-0 flex-col" data-plugin-host-tab="lore">
                <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3 pr-12">
                  <Button variant="ghost" size="sm" className="min-w-0 shrink" onClick={() => void lore.closeTab().then(saved => { if (saved) frame.current?.refresh() })}>
                    <ArrowLeft /><span className="truncate">{t('platform.pages.backToPage', { title })}</span>
                  </Button>
                  <h2 className="truncate text-sm font-medium">{t('tab.lore')}</h2>
                </header>
                <div className="min-h-0 flex-1">
                  <LoreWorkspaceTab projectId={projectId} documentReview={lore.documentReview}
                    navigationIntent={lore.navigation} onEditorFlushHandlerChange={lore.onEditorFlushHandlerChange}
                    onActiveItemChange={lore.setActiveItemId}
                    onReferenceItem={page.agent ? id => void lore.referenceItem(id) : undefined} />
                </div>
              </div>}
            </CollapsibleResizablePanel>
            {page.agent && <>
              <CollapsiblePanelSeparator visible={!isMobile && assistantVisible}
                aria-label={t('layout.resize.right')}
                className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none"
                {...layout.resizeHandleIntentProps}
                onPointerDownCapture={event => {
                  layout.resizeHandleIntentProps.onPointerDownCapture(event)
                  // The adjacent cross-origin frame can consume the first pointer move before
                  // the resize library captures it. Own the pointer from the start of the drag.
                  event.currentTarget.setPointerCapture(event.pointerId)
                }} />
              <CollapsibleResizablePanel id="right" visible={assistantVisible} side="right"
                defaultSize={isMobile ? '100%' : '380px'}
                initialExpandSize={isMobile ? '100%' : resolvePanelInitialSize(layout.defaultLayout, 'right', '380px')}
                restorationKey={`${layoutKey}/${isMobile ? 'mobile' : 'desktop'}`}
                minSize={isMobile ? '0px' : '300px'} maxSize={isMobile ? '100%' : '60%'}
                groupResizeBehavior="preserve-pixel-size" className="min-w-0">
                <div id="plugin-page-agent" className="h-full min-h-0 [&_.nova-writing-agent-toolbar]:pr-11">
                  <PluginPageAgent projectId={projectId} active={visible && assistantVisible}
                    page={page} context={pageContext} presentation={presentation}
                    loreReferenceLabels={loreReferenceLabels} loreSuggestions={loreSuggestions}
                    lore={{ ...lore.agentProps, open: lore.open, activeItemId: lore.activeItemId }}
                    onPageAction={id => { setMobileAssistant(false); frame.current?.pageAction(id) }}
                    onBusyChange={setAgentBusy} onChange={() => frame.current?.refresh()} />
                </div>
              </CollapsibleResizablePanel>
            </>}
          </PanelMotionGroup>
        </>
      ) : ready ? <p className="p-6" role="status">{t('common.loading')}</p> : null}
    </section>
  )
}
