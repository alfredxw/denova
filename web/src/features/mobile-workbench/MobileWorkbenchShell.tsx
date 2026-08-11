import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useKeyboardInset } from '@/hooks/useKeyboardInset'
import { useIsMobile } from '@/hooks/useIsMobile'
import { MOBILE_WORKBENCH_NAVIGATE_EVENT, mobileWorkbenchDestinationFromEvent } from './navigation'

export interface MobileWorkbenchDestination {
  id: string
  label: string
  title: string
  icon: ReactNode
  content: ReactNode
  onSelect?: () => void
}

interface MobileWorkbenchShellProps {
  persistenceKey: string
  modeKey: 'ide' | 'interactive'
  defaultDestinationId: string
  destinations: MobileWorkbenchDestination[]
  moreLabel: string
  moreIcon: ReactNode
  moreMenu: ReactNode
  moreBadgeCount?: number
  moreBadgeLabel?: string
  sharedContent: ReactNode
  sharedMenuActive: boolean
  navigationLabel: string
  projectSwitcher: ReactNode
}

export function MobileWorkbenchShell({
  persistenceKey,
  modeKey,
  defaultDestinationId,
  destinations,
  moreLabel,
  moreIcon,
  moreMenu,
  moreBadgeCount = 0,
  moreBadgeLabel,
  sharedContent,
  sharedMenuActive,
  navigationLabel,
  projectSwitcher,
}: MobileWorkbenchShellProps) {
  const keyboardInset = useKeyboardInset()
  const isPhone = useIsMobile('(max-width: 639px)')
  const [activeByMode, setActiveByMode] = useState<MobileDestinationMemory>(() => (
    readDestinationMemory(persistenceKey)
  ))
  const [moreOpen, setMoreOpen] = useState(false)

  useEffect(() => {
    setActiveByMode(readDestinationMemory(persistenceKey))
  }, [persistenceKey])

  useEffect(() => {
    storeDestinationMemory(persistenceKey, activeByMode)
  }, [activeByMode, persistenceKey])

  useEffect(() => {
    setMoreOpen(false)
  }, [modeKey])

  const activeDestinationId = useMemo(() => {
    const remembered = activeByMode[modeKey]
    return destinations.some((item) => item.id === remembered) ? remembered! : defaultDestinationId
  }, [activeByMode, defaultDestinationId, destinations, modeKey])
  const moreActive = sharedMenuActive || moreOpen
  const activeDestination = destinations.find((item) => item.id === activeDestinationId) ?? destinations[0]
  const title = moreActive ? moreLabel : activeDestination?.title ?? moreLabel
  const content = sharedMenuActive
    ? sharedContent
    : moreOpen
      ? moreMenu
      : activeDestination?.content

  const selectDestination = (destination: MobileWorkbenchDestination) => {
    setMoreOpen(false)
    setActiveByMode((current) => ({ ...current, [modeKey]: destination.id }))
    destination.onSelect?.()
  }

  useEffect(() => {
    const navigate = (event: Event) => {
      const target = mobileWorkbenchDestinationFromEvent(event)
      if (!target) return
      const targetMode = target.mode ?? modeKey
      if (targetMode !== 'ide' && targetMode !== 'interactive') return
      if (targetMode === modeKey && !destinations.some((item) => item.id === target.destinationId)) return
      setMoreOpen(false)
      setActiveByMode((current) => ({ ...current, [targetMode]: target.destinationId }))
    }
    window.addEventListener(MOBILE_WORKBENCH_NAVIGATE_EVENT, navigate)
    return () => window.removeEventListener(MOBILE_WORKBENCH_NAVIGATE_EVENT, navigate)
  }, [destinations, modeKey])

  const navigation = keyboardInset === 0 ? (
    <nav
      data-placement={isPhone ? 'bottom' : 'side'}
      className={isPhone
        ? 'nova-mobile-nav grid shrink-0 grid-cols-4 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-1 pt-1.5'
        : 'flex w-24 shrink-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-2'}
      aria-label={navigationLabel}
    >
      {destinations.map((destination) => {
        const active = !moreActive && destination.id === activeDestinationId
        return (
          <MobileDestinationButton
            key={destination.id}
            label={destination.label}
            icon={destination.icon}
            active={active}
            placement={isPhone ? 'bottom' : 'side'}
            onClick={() => selectDestination(destination)}
          />
        )
      })}
      <MobileDestinationButton
        label={moreLabel}
        icon={moreIcon}
        badgeCount={moreBadgeCount}
        badgeLabel={moreBadgeLabel}
        active={moreActive}
        placement={isPhone ? 'bottom' : 'side'}
        moreTrigger
        onClick={() => setMoreOpen(true)}
      />
    </nav>
  ) : null

  return (
    <div data-nova-app-shell="true" data-nova-mobile-shell="true" className="flex h-dvh w-screen flex-col overflow-hidden bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <header className="nova-topbar flex min-h-13 shrink-0 items-center gap-3 border-b border-[var(--nova-border)] px-3 py-2 pt-[max(0.5rem,env(safe-area-inset-top))]">
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-[var(--nova-text)]">{title}</div>
          <div className="mt-0.5 flex min-w-0 items-center">{projectSwitcher}</div>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        {!isPhone && navigation}
        <main className="min-h-0 min-w-0 flex-1 overflow-hidden">{content}</main>
      </div>

      {isPhone && navigation}
    </div>
  )
}

type MobileDestinationMemory = Partial<Record<'ide' | 'interactive', string>>

const MOBILE_DESTINATION_STORAGE_PREFIX = 'nova:mobile-workbench:destinations:'

function readDestinationMemory(persistenceKey: string): MobileDestinationMemory {
  if (typeof window === 'undefined') return {}
  try {
    const parsed = JSON.parse(window.localStorage.getItem(`${MOBILE_DESTINATION_STORAGE_PREFIX}${persistenceKey}`) || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return {
      ide: typeof parsed.ide === 'string' ? parsed.ide : undefined,
      interactive: typeof parsed.interactive === 'string' ? parsed.interactive : undefined,
    }
  } catch {
    return {}
  }
}

function storeDestinationMemory(persistenceKey: string, memory: MobileDestinationMemory) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(`${MOBILE_DESTINATION_STORAGE_PREFIX}${persistenceKey}`, JSON.stringify(memory))
}

function MobileDestinationButton({
  label,
  icon,
  active,
  placement,
  onClick,
  badgeCount = 0,
  badgeLabel,
  moreTrigger = false,
}: {
  label: string
  icon: ReactNode
  active: boolean
  placement: 'bottom' | 'side'
  onClick: () => void
  badgeCount?: number
  badgeLabel?: string
  moreTrigger?: boolean
}) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-current={active ? 'page' : undefined}
      data-mobile-more-button={moreTrigger ? 'true' : undefined}
      className={`flex min-w-0 flex-col items-center justify-center gap-0.5 rounded-[var(--nova-radius)] px-1 text-[11px] transition-colors ${placement === 'side' ? 'min-h-16 w-full' : 'min-h-12'} ${active ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]'}`}
      onClick={onClick}
    >
      <span className="relative flex size-5 items-center justify-center">
        <span aria-hidden="true">{icon}</span>
        {badgeCount > 0 && (
          <span
            className="absolute -right-2 -top-1.5 min-w-4 rounded-full bg-[var(--nova-danger)] px-1 text-center text-[9px] font-semibold leading-4 text-white"
            aria-label={badgeLabel}
          >
            {badgeCount > 99 ? '99+' : badgeCount}
          </span>
        )}
      </span>
      <span className="max-w-full break-words text-center leading-4">{label}</span>
    </button>
  )
}
