import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AnimatePresence, motion, useIsPresent, useReducedMotionConfig } from 'motion/react'
import { novaEase } from '@/features/motion/motion-tokens'
import { PanelLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import { AgentChatActivitySidebar, type AgentChatActivitySidebarProps } from './AgentChatActivitySidebar'

/** Grace period before a peek closes, so crossing the button's edge diagonally does not dismiss it. */
const PEEK_CLOSE_DELAY_MS = 160
/** Avoid mounting the full project tree when the pointer is only crossing the tab strip. */
const PEEK_OPEN_DELAY_MS = 120

interface AgentChatSidebarToggleProps {
  visible: boolean
  onToggle: () => void
  tree: AgentChatActivitySidebarProps
}

/** A fixed desktop toggle with a transient overlay; only pinning changes persisted layout. */
export function AgentChatSidebarToggle({ visible, onToggle, tree }: AgentChatSidebarToggleProps) {
  const { t } = useTranslation()
  const [peeking, setPeeking] = useState(false)
  const openTimerRef = useRef<number | null>(null)
  const closeTimerRef = useRef<number | null>(null)

  const cancelOpen = () => {
    if (openTimerRef.current === null) return
    window.clearTimeout(openTimerRef.current)
    openTimerRef.current = null
  }
  const cancelClose = () => {
    if (closeTimerRef.current === null) return
    window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = null
  }
  const schedulePeek = () => {
    cancelClose()
    if (visible || peeking || openTimerRef.current !== null) return
    openTimerRef.current = window.setTimeout(() => {
      openTimerRef.current = null
      setPeeking(true)
    }, PEEK_OPEN_DELAY_MS)
  }
  const closePeek = () => {
    cancelOpen()
    cancelClose()
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null
      setPeeking(false)
    }, PEEK_CLOSE_DELAY_MS)
  }
  const toggleSidebar = () => {
    cancelOpen()
    cancelClose()
    setPeeking(false)
    onToggle()
  }

  useEffect(() => () => {
    cancelOpen()
    cancelClose()
  }, [])

  return (
    <div
      className="pointer-events-none absolute inset-0 z-40"
      data-agent-chat-sidebar-toggle
      onMouseEnter={schedulePeek}
      onMouseLeave={closePeek}
      onFocusCapture={schedulePeek}
      onBlurCapture={closePeek}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          cancelOpen()
          cancelClose()
          setPeeking(false)
        }
      }}
    >
      <Button className="pointer-events-auto absolute left-2 top-1.5" type="button" variant="ghost" size="icon-xs" onClick={toggleSidebar}
        aria-label={t(visible ? 'agentChat.sidebar.hide' : 'agentChat.sidebar.show')}
        aria-pressed={visible}
      >
        <PanelLeft />
      </Button>

      <AnimatePresence>
        {peeking && !visible ? (
          <SidebarPeek>
            <AgentChatActivitySidebar
              {...tree}
              onSelectProject={(project) => {
                setPeeking(false)
                tree.onSelectProject(project)
              }}
              onOpenActivity={(project, activity) => {
                setPeeking(false)
                tree.onOpenActivity(project, activity)
                closeMobilePanes()
              }}
              onOpenSession={(project, session) => {
                setPeeking(false)
                tree.onOpenSession(project, session)
                closeMobilePanes()
              }}
              onCreateSession={(project, customAgentId) => {
                setPeeking(false)
                tree.onCreateSession(project, customAgentId)
                closeMobilePanes()
              }}
              onOpenHistory={(project) => {
                setPeeking(false)
                tree.onOpenHistory(project)
              }}
            />
          </SidebarPeek>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

/** Keep exiting previews inert while their visual exit finishes. */
function SidebarPeek({ children }: { children: ReactNode }) {
  const present = useIsPresent()
  const reducedMotion = useReducedMotionConfig()
  return (
    <motion.div
      data-agent-chat-sidebar-peek
      aria-hidden={!present}
      inert={!present}
      className="absolute bottom-0 left-0 top-9 w-[clamp(240px,20vw,300px)] overflow-hidden rounded-b-xl border border-[var(--nova-border)] bg-[var(--nova-surface)] shadow-xl"
      style={{ pointerEvents: present ? 'auto' : 'none' }}
      initial={{ opacity: 0, x: reducedMotion ? 0 : -12 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: reducedMotion ? 0 : -12 }}
      transition={{ duration: reducedMotion ? 0 : 0.16, ease: novaEase }}
    >
      {children}
    </motion.div>
  )
}
