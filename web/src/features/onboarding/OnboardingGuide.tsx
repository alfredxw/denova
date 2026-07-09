import { useCallback, useEffect, useMemo, useState, type CSSProperties } from 'react'
import { Check, ChevronRight, Compass, Loader2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { fetchSettings } from '@/features/settings/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentViewContent, buildAgentMessageViews } from '@/lib/agent-message-view'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import { ONBOARDING_OPEN_EVENT } from './events'
import { hasUsableLanguageModel } from './model-status'
import { readOnboardingState, writeOnboardingState, type OnboardingStoredState } from './state'

type CoreStepId = 'model' | 'book' | 'agent'
type GuidePhase = 'core' | 'tour'
export type OnboardingNavigationTarget =
  | 'settings-model'
  | 'books'
  | 'writing'
  | 'writing-agent'
  | 'interactive'
  | 'lore'
  | 'teller'
  | 'versions'
  | 'skills'
  | 'agents'
  | 'automations'

interface TourStep {
  id: string
  titleKey: string
  bodyKey: string
  anchor: string
  navigationTarget: OnboardingNavigationTarget
}

interface OnboardingGuideProps {
  mode: WorkspaceMode
  rightPanel: RightPanel
  settingsOpen: boolean
  workspace: string
  booksCount: number
  currentBookName: string
  messages: AgentUIMessage[]
  isStreaming: boolean
  onNavigate: (target: OnboardingNavigationTarget, prompt?: string) => void
}

interface AnchorRect {
  top: number
  left: number
  width: number
  height: number
}

const TOUR_STEPS: TourStep[] = [
  { id: 'settings', titleKey: 'onboarding.tour.settings.title', bodyKey: 'onboarding.tour.settings.body', anchor: 'activity-settings', navigationTarget: 'settings-model' },
  { id: 'books', titleKey: 'onboarding.tour.books.title', bodyKey: 'onboarding.tour.books.body', anchor: 'activity-books', navigationTarget: 'books' },
  { id: 'writing', titleKey: 'onboarding.tour.writing.title', bodyKey: 'onboarding.tour.writing.body', anchor: 'activity-writing', navigationTarget: 'writing' },
  { id: 'agent', titleKey: 'onboarding.tour.agent.title', bodyKey: 'onboarding.tour.agent.body', anchor: 'agent-input', navigationTarget: 'writing-agent' },
  { id: 'interactive', titleKey: 'onboarding.tour.interactive.title', bodyKey: 'onboarding.tour.interactive.body', anchor: 'mode-interactive', navigationTarget: 'interactive' },
  { id: 'lore', titleKey: 'onboarding.tour.lore.title', bodyKey: 'onboarding.tour.lore.body', anchor: 'activity-lore', navigationTarget: 'lore' },
  { id: 'teller', titleKey: 'onboarding.tour.teller.title', bodyKey: 'onboarding.tour.teller.body', anchor: 'activity-teller', navigationTarget: 'teller' },
  { id: 'versions', titleKey: 'onboarding.tour.versions.title', bodyKey: 'onboarding.tour.versions.body', anchor: 'activity-versions', navigationTarget: 'versions' },
  { id: 'skills', titleKey: 'onboarding.tour.skills.title', bodyKey: 'onboarding.tour.skills.body', anchor: 'activity-skills', navigationTarget: 'skills' },
  { id: 'agents', titleKey: 'onboarding.tour.agents.title', bodyKey: 'onboarding.tour.agents.body', anchor: 'activity-agents', navigationTarget: 'agents' },
  { id: 'automations', titleKey: 'onboarding.tour.automations.title', bodyKey: 'onboarding.tour.automations.body', anchor: 'activity-automations', navigationTarget: 'automations' },
]

export function OnboardingGuide({
  mode,
  rightPanel,
  settingsOpen,
  workspace,
  booksCount,
  currentBookName,
  messages,
  isStreaming,
  onNavigate,
}: OnboardingGuideProps) {
  const { t } = useTranslation()
  const [storedState, setStoredState] = useState<OnboardingStoredState>(() => readOnboardingState())
  const [open, setOpen] = useState(false)
  const [loadingSettings, setLoadingSettings] = useState(true)
  const [modelReady, setModelReady] = useState(false)
  const [phase, setPhase] = useState<GuidePhase>('core')
  const [coreStep, setCoreStep] = useState<CoreStepId>('model')
  const [tourIndex, setTourIndex] = useState(0)
  const [anchorRect, setAnchorRect] = useState<AnchorRect | null>(null)

  const completedAgentTurn = useMemo(() => {
    if (isStreaming) return false
    return buildAgentMessageViews(messages).some((view) =>
      (view.kind === 'assistant' || view.kind === 'tool-result' || view.kind === 'tool') &&
      (Boolean(agentViewContent(view).trim()) || view.status === 'success'),
    )
  }, [isStreaming, messages])

  const refreshModelState = useCallback(() => {
    setLoadingSettings(true)
    fetchSettings()
      .then((settings) => {
        setModelReady(hasUsableLanguageModel(settings.effective))
      })
      .catch((error) => {
        console.warn('[onboarding] load settings failed', error)
        setModelReady(false)
      })
      .finally(() => setLoadingSettings(false))
  }, [])

  useEffect(() => {
    refreshModelState()
    window.addEventListener('nova:settings-updated', refreshModelState)
    return () => window.removeEventListener('nova:settings-updated', refreshModelState)
  }, [refreshModelState])

  useEffect(() => {
    const openGuide = () => {
      const next = { version: 1 } as OnboardingStoredState
      writeOnboardingState(next)
      setStoredState(next)
      setPhase('core')
      setCoreStep(firstCoreStep(modelReady, workspace))
      setTourIndex(0)
      setOpen(true)
      refreshModelState()
    }
    window.addEventListener(ONBOARDING_OPEN_EVENT, openGuide)
    return () => window.removeEventListener(ONBOARDING_OPEN_EVENT, openGuide)
  }, [modelReady, refreshModelState, workspace])

  useEffect(() => {
    if (open || loadingSettings || storedState.skipped || storedState.completed) return
    if (!modelReady || !workspace || booksCount === 0) {
      setCoreStep(firstCoreStep(modelReady, workspace))
      setPhase('core')
      setOpen(true)
    }
  }, [booksCount, loadingSettings, modelReady, open, storedState.completed, storedState.skipped, workspace])

  useEffect(() => {
    if (!open || phase !== 'core') return
    if (!modelReady) {
      setCoreStep('model')
      return
    }
    if (!workspace) {
      setCoreStep('book')
      return
    }
    if (completedAgentTurn) {
      setPhase('tour')
      setTourIndex(0)
      return
    }
    setCoreStep('agent')
  }, [completedAgentTurn, modelReady, open, phase, workspace])

  const currentStep = useMemo(() => {
    if (phase === 'tour') {
      const step = TOUR_STEPS[Math.min(tourIndex, TOUR_STEPS.length - 1)]
      return {
        id: step.id,
        title: t(step.titleKey),
        body: t(step.bodyKey),
        anchor: tourAnchor(step, mode, rightPanel, settingsOpen),
        actionLabel: t('onboarding.action.openModule'),
        navigationTarget: step.navigationTarget,
        progress: t('onboarding.progress', { current: tourIndex + 1, total: TOUR_STEPS.length }),
      }
    }
    const step = coreStepConfig(coreStep, { mode, rightPanel, settingsOpen, bookName: currentBookName, t })
    return {
      ...step,
      progress: t('onboarding.progress', { current: coreStepProgress(coreStep), total: 3 }),
    }
  }, [coreStep, currentBookName, mode, phase, rightPanel, settingsOpen, t, tourIndex])

  useEffect(() => {
    if (!open) return
    const updateAnchor = () => {
      if (window.matchMedia('(max-width: 767px)').matches) {
        setAnchorRect(null)
        return
      }
      const target = findVisibleAnchor(currentStep.anchor)
      if (!target) {
        setAnchorRect(null)
        return
      }
      const rect = target.getBoundingClientRect()
      if (rect.width <= 0 || rect.height <= 0) {
        setAnchorRect(null)
        return
      }
      setAnchorRect({
        top: rect.top,
        left: rect.left,
        width: rect.width,
        height: rect.height,
      })
    }
    updateAnchor()
    window.addEventListener('resize', updateAnchor)
    window.addEventListener('scroll', updateAnchor, true)
    const id = window.setInterval(updateAnchor, 500)
    return () => {
      window.removeEventListener('resize', updateAnchor)
      window.removeEventListener('scroll', updateAnchor, true)
      window.clearInterval(id)
    }
  }, [currentStep.anchor, open])

  const skip = () => {
    const next = { version: 1, skipped: true } as OnboardingStoredState
    writeOnboardingState(next)
    setStoredState(next)
    setOpen(false)
  }

  const complete = () => {
    const next = { version: 1, completed: true } as OnboardingStoredState
    writeOnboardingState(next)
    setStoredState(next)
    setOpen(false)
  }

  const handleAction = () => {
    const prompt = phase === 'core' && currentStep.navigationTarget === 'writing-agent'
      ? t('onboarding.firstPrompt', { book: currentBookName || t('workbench.untitled') })
      : undefined
    onNavigate(currentStep.navigationTarget, prompt)
  }

  const nextTourStep = () => {
    if (tourIndex >= TOUR_STEPS.length - 1) {
      complete()
      return
    }
    setTourIndex((index) => index + 1)
  }

  if (!open || loadingSettings) return null

  const cardStyle = anchoredCardStyle(anchorRect)
  const bottomSheet = !anchorRect

  return (
    <>
      {anchorRect && (
        <div
          aria-hidden="true"
          className="pointer-events-none fixed z-50 rounded-[calc(var(--nova-radius)+4px)] border border-[var(--nova-accent)] bg-[var(--nova-accent)]/10 shadow-[0_0_0_9999px_rgba(0,0,0,0.18)]"
          style={{
            top: anchorRect.top - 6,
            left: anchorRect.left - 6,
            width: anchorRect.width + 12,
            height: anchorRect.height + 12,
          }}
        />
      )}
      <section
        role="dialog"
        aria-live="polite"
        aria-label={t('onboarding.title')}
        className={`fixed z-50 w-[min(360px,calc(100vw-1.5rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]/95 p-3 text-xs text-[var(--nova-text)] shadow-[var(--nova-shadow)] backdrop-blur-xl ${
          bottomSheet ? 'bottom-3 left-1/2 -translate-x-1/2' : ''
        }`}
        style={bottomSheet ? undefined : cardStyle}
      >
        <div className="mb-2 flex items-start gap-2">
          <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-[9px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            <Compass className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="font-medium">{currentStep.title}</span>
              <span className="ml-auto shrink-0 text-[11px] text-[var(--nova-text-faint)]">{currentStep.progress}</span>
            </div>
            <p className="mt-1 leading-5 text-[var(--nova-text-muted)]">{currentStep.body}</p>
          </div>
          <button
            type="button"
            onClick={skip}
            className="nova-nav-item -mr-1 -mt-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-[8px] text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
            aria-label={t('onboarding.skip')}
            title={t('onboarding.skip')}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="flex items-center justify-between gap-2 border-t border-[var(--nova-border-soft)] pt-2">
          <Button type="button" variant="ghost" size="xs" className="text-[var(--nova-text-muted)]" onClick={skip}>
            {t(phase === 'tour' ? 'onboarding.skipTour' : 'onboarding.skip')}
          </Button>
          <div className="flex items-center gap-2">
            <Button type="button" size="xs" className="border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={handleAction}>
              {phase === 'tour' ? t('onboarding.action.openModule') : currentStep.actionLabel}
            </Button>
            {phase === 'tour' && (
              <Button type="button" size="xs" variant="ghost" className="text-[var(--nova-text-muted)]" onClick={nextTourStep}>
                {tourIndex >= TOUR_STEPS.length - 1 ? (
                  <>
                    <Check className="h-3.5 w-3.5" />
                    {t('onboarding.finish')}
                  </>
                ) : (
                  <>
                    {t('onboarding.next')}
                    <ChevronRight className="h-3.5 w-3.5" />
                  </>
                )}
              </Button>
            )}
            {phase === 'core' && currentStep.id !== 'agent' && (
              <span className="inline-flex items-center gap-1 text-[11px] text-[var(--nova-text-faint)]">
                <Loader2 className="h-3 w-3 animate-spin" />
                {t('onboarding.waiting')}
              </span>
            )}
          </div>
        </div>
      </section>
    </>
  )
}

function firstCoreStep(modelReady: boolean, workspace: string): CoreStepId {
  if (!modelReady) return 'model'
  if (!workspace) return 'book'
  return 'agent'
}

function coreStepProgress(step: CoreStepId) {
  if (step === 'model') return 1
  if (step === 'book') return 2
  return 3
}

function coreStepConfig(step: CoreStepId, ctx: {
  mode: WorkspaceMode
  rightPanel: RightPanel
  settingsOpen: boolean
  bookName: string
  t: ReturnType<typeof useTranslation>['t']
}) {
  if (step === 'model') {
    return {
      id: step,
      title: ctx.t('onboarding.step.model.title'),
      body: ctx.t('onboarding.step.model.body'),
      anchor: ctx.settingsOpen ? 'settings-model' : 'activity-settings',
      actionLabel: ctx.t('onboarding.step.model.action'),
      navigationTarget: 'settings-model' as const,
    }
  }
  if (step === 'book') {
    return {
      id: step,
      title: ctx.t('onboarding.step.book.title'),
      body: ctx.t('onboarding.step.book.body'),
      anchor: ctx.mode === 'books' ? 'books-create' : 'activity-books',
      actionLabel: ctx.t('onboarding.step.book.action'),
      navigationTarget: 'books' as const,
    }
  }
  return {
    id: step,
    title: ctx.t('onboarding.step.agent.title'),
    body: ctx.t('onboarding.step.agent.body', { book: ctx.bookName || ctx.t('workbench.untitled') }),
    anchor: ctx.mode === 'ide' && ctx.rightPanel === 'ai' ? 'agent-input' : 'activity-writing',
    actionLabel: ctx.t('onboarding.step.agent.action'),
    navigationTarget: 'writing-agent' as const,
  }
}

function tourAnchor(step: TourStep, mode: WorkspaceMode, rightPanel: RightPanel, settingsOpen: boolean) {
  if (step.id === 'settings') return settingsOpen ? 'settings-model' : 'activity-settings'
  if (step.id === 'books') return mode === 'books' ? 'books-create' : 'activity-books'
  if (step.id === 'agent') return mode === 'ide' && rightPanel === 'ai' ? 'agent-input' : 'activity-writing'
  if (step.id === 'interactive') return mode === 'interactive' ? 'activity-story' : 'mode-interactive'
  return step.anchor
}

function findVisibleAnchor(anchor: string) {
  const targets = document.querySelectorAll<HTMLElement>(`[data-onboarding-anchor="${anchor}"]`)
  for (const target of targets) {
    const rect = target.getBoundingClientRect()
    if (rect.width > 0 && rect.height > 0) return target
  }
  return null
}

function anchoredCardStyle(rect: AnchorRect | null): CSSProperties | undefined {
  if (!rect || typeof window === 'undefined') return undefined
  const width = Math.min(360, window.innerWidth - 24)
  const left = clamp(rect.left, 12, window.innerWidth - width - 12)
  const below = rect.top + rect.height + 14
  const above = rect.top - 14
  const estimatedHeight = 190
  const top = below + estimatedHeight < window.innerHeight
    ? below
    : Math.max(12, above - estimatedHeight)
  return { top, left, width }
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}
