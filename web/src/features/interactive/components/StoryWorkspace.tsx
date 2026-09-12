import { useState, type ReactNode, type KeyboardEventHandler, type PointerEventHandler } from 'react'
import { createPortal } from 'react-dom'
import { GripHorizontal, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Panel } from 'react-resizable-panels'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MobilePaneHost } from '@/components/layout/mobile-pane-host'
import { CollapsiblePanelSeparator, CollapsibleResizablePanel, PanelMotionGroup } from '@/components/layout/panel-motion'
import { usePersistedPanelLayout } from '@/components/layout/use-persisted-panel-layout'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'

/** Retains the story and its supporting console while moving between a phone drawer and desktop split. */
export function StoryWorkspace({ story, console: consoleContent, rightPanelVisible, mobileConsoleOpen, onMobileConsoleOpenChange }: {
  story: ReactNode
  console: ReactNode
  rightPanelVisible: boolean
  mobileConsoleOpen: boolean
  onMobileConsoleOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [storyHost] = useState(() => createStablePortalHost('flex h-full min-h-0 flex-col'))
  const [consoleHost] = useState(() => createStablePortalHost('flex h-full min-h-0 flex-col'))
  const storySlot = <StablePortalSlot host={storyHost} fallback={story} className="h-full min-h-0 w-full min-w-0 flex-1" />
  const consoleSlot = <StablePortalSlot host={consoleHost} fallback={consoleContent} className="h-full min-h-0 w-full min-w-0 flex-1" />
  const storyPanelLayout = usePersistedPanelLayout({ storageKey: 'nova-interactive-horizontal', panelIds: ['story-stage', 'snapshot'] })
  return (
    <div className="flex h-full min-h-0 flex-1 flex-col">
      {isMobile ? (
        <MobilePaneHost
          panes={[{ id: 'director-panel', title: t('directorPanel.title'), side: 'right', swipeToOpen: true, content: consoleSlot }]}
          closeLabel={t('common.close')}
          openPaneId={mobileConsoleOpen ? 'director-panel' : null}
          onOpenPaneChange={(id) => onMobileConsoleOpenChange(id === 'director-panel')}
          className="relative flex min-h-0 flex-1"
        >
          {storySlot}
        </MobilePaneHost>
      ) : (
        <PanelMotionGroup
          id="nova-interactive-horizontal"
          defaultLayout={storyPanelLayout.defaultLayout}
          onLayoutChanged={(layout) => {
            if (rightPanelVisible) storyPanelLayout.persistUserLayout(layout)
          }}
          orientation="horizontal"
          className="min-h-0 flex-1"
        >
          <Panel id="story-stage" minSize="240px" className="min-w-0">
            {storySlot}
          </Panel>
          <InteractiveResizeHandle
            visible={rightPanelVisible}
            direction="vertical"
            label={t('interactiveLayout.resizeDirectorPanel')}
            {...storyPanelLayout.resizeHandleIntentProps}
          />
          <CollapsibleResizablePanel
            id="snapshot"
            visible={rightPanelVisible}
            side="right"
            defaultSize="320px"
            minSize="240px"
            maxSize="45%"
            className="min-w-[240px]"
          >
            {consoleSlot}
          </CollapsibleResizablePanel>
        </PanelMotionGroup>
      )}
      {storyHost && createPortal(story, storyHost)}
      {consoleHost && createPortal(consoleContent, consoleHost)}
    </div>
  )
}

function InteractiveResizeHandle({
  direction,
  label,
  prominent = false,
  visible = true,
  onPointerDownCapture,
  onKeyDownCapture,
}: {
  direction: 'horizontal' | 'vertical'
  label: string
  prominent?: boolean
  visible?: boolean
  onPointerDownCapture?: PointerEventHandler<HTMLElement>
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
}) {
  const Icon = direction === 'vertical' ? GripVertical : GripHorizontal
  const className = direction === 'vertical' ? 'nova-resize-handle group -mx-1 flex w-3 cursor-col-resize items-center justify-center bg-transparent transition-colors' : `nova-resize-handle group ${prominent ? '-my-0.5 h-4' : '-my-1 h-3'} flex cursor-row-resize items-center justify-center bg-transparent transition-colors`

  return (
    <CollapsiblePanelSeparator
      visible={visible}
      aria-label={label}
      className={className}
      onPointerDownCapture={onPointerDownCapture}
      onKeyDownCapture={onKeyDownCapture}
    >
      <span className={`flex items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)] shadow-[0_4px_14px_rgba(0,0,0,0.22)] transition-colors group-hover:border-[var(--nova-active)] group-data-[resize-handle-active]:border-[var(--nova-active)] group-data-[resize-handle-active]:text-[var(--nova-text)] ${direction === 'vertical' ? 'h-9 w-2.5' : 'h-2.5 w-16'}`}>
        <Icon className={direction === 'vertical' ? 'h-3.5 w-3.5' : 'h-3 w-3'} aria-hidden="true" />
      </span>
    </CollapsiblePanelSeparator>
  )
}
