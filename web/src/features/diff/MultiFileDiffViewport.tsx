import type { ReactNode } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { InlineCollapsiblePane } from '@/components/layout/panel-motion'
import { DiffFileNavigator } from './DiffFileNavigator'
import type { DiffFileNavigationItem } from './types'
import type { MultiFileDiffNavigation } from './use-multi-file-diff-navigation'
import '@/features/changes/review/review-diff.css'

interface DiffFileRenderState {
  active: boolean
  preRender: boolean
  collapsed: boolean
  sectionRef: (node: HTMLElement | null) => void
  onToggle: () => void
}

interface MultiFileDiffViewportProps<File extends { path: string }> {
  files: readonly File[]
  navigatorFiles: readonly DiffFileNavigationItem[]
  navigation: MultiFileDiffNavigation
  ariaLabel: string
  empty: ReactNode
  renderFile: (file: File, state: DiffFileRenderState) => ReactNode
}

/** Continuous file sections plus the synchronized Review-style navigator. */
export function MultiFileDiffViewport<File extends { path: string }>({ files, navigatorFiles, navigation, ariaLabel, empty, renderFile }: MultiFileDiffViewportProps<File>) {
  return (
    <div className="nova-review-container min-h-0 flex-1">
      <div className="nova-review-layout flex h-full min-h-0">
        <ScrollArea
          type="always"
          data-review-scroll-root="true"
          className="min-h-0 min-w-0 flex-1 overflow-hidden"
          viewportRef={navigation.scrollRef}
          viewportProps={{
            role: 'main',
            tabIndex: 0,
            'aria-label': ariaLabel,
            onScroll: navigation.handleScroll,
            onWheelCapture: navigation.cancelPendingJump,
            onPointerDownCapture: navigation.cancelPendingJump,
            onKeyDownCapture: navigation.cancelPendingJump,
            onTouchStartCapture: navigation.cancelPendingJump,
            className: 'nova-review-scrollbar overscroll-contain focus-visible:ring-0 focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-[-1px] focus-visible:outline-[var(--nova-accent-blue)]',
          }}
        >
          {files.length ? files.map((file) => renderFile(file, {
            active: file.path === navigation.activePath,
            preRender: navigation.preRenderPaths.has(file.path),
            collapsed: navigation.collapsedPaths.has(file.path),
            sectionRef: (node) => navigation.registerFileSection(file.path, node),
            onToggle: () => navigation.toggleFile(file.path),
          })) : empty}
        </ScrollArea>
        <InlineCollapsiblePane
          visible={navigation.navigatorVisible}
          side="right"
          size="clamp(14rem, 19vw, 17rem)"
          className="nova-review-file-navigator-motion"
        >
          <DiffFileNavigator
            files={navigatorFiles}
            selectedPath={navigation.activePath}
            onSelect={navigation.jumpToFile}
            onCollapse={() => navigation.setNavigatorVisible(false)}
          />
        </InlineCollapsiblePane>
      </div>
    </div>
  )
}
