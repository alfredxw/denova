import { useState } from 'react'
import type { CSSProperties } from 'react'
import { Activity, Check, ChevronDown, ChevronLeft, ChevronRight, Copy, Dice5, GitBranch, ImagePlus, Loader2, PanelRightOpen, Pencil, RefreshCw, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChatMessage, RuleRollChatMessage, UserMessageReference } from '@/lib/api'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import {
  DEFAULT_TOOLTIP_DELAY_MS,
  DEFAULT_TOOLTIP_SKIP_DELAY_MS,
  TooltipProvider,
} from '@/components/ui/tooltip'
import { StreamingContentStage } from './StreamingContentStage'
import { MarkdownContent } from './message-content'
import { buildMarkdownPreview } from './message-tool'
import { subAgentStatusTranslationKey } from './subagent-session'
import { useTrajectoryNavigation } from '@/features/trajectory/trajectory-navigation'

const copyFeedbackDurationMs = 1200
const messageActionTooltipDelayMs = DEFAULT_TOOLTIP_DELAY_MS
const messageActionTooltipSkipDelayMs = DEFAULT_TOOLTIP_SKIP_DELAY_MS
const messageActionTooltipSideOffset = 3

export function SentMessageReferences({ references }: { references?: UserMessageReference[] }) {
  const { t } = useTranslation()
  if (!references?.length) return null
  return (
    <div data-testid="sent-message-references" className="mb-1.5 flex max-w-full flex-col gap-1 border-b border-current/10 pb-1.5 text-[11px] leading-4">
      {references.map((reference, index) => (
        <div key={`${reference.kind}:${reference.id || reference.label}:${index}`} className="flex min-w-0 items-start gap-1.5">
          <span className="shrink-0 rounded bg-black/10 px-1 py-0.5 text-[10px] opacity-75 dark:bg-white/10">
            {t(`chat.reference.${reference.kind}`)}
          </span>
          <span className="min-w-0 break-words">
            <span className="font-medium">{reference.label}{formatReferenceLines(reference)}</span>
            {reference.detail ? <span className="ml-1 opacity-75">— {reference.detail}</span> : null}
          </span>
        </div>
      ))}
    </div>
  )
}

function formatReferenceLines(reference: UserMessageReference): string {
  if (reference.start_line === undefined) return ''
  if (reference.end_line !== undefined && reference.end_line !== reference.start_line) return `:L${reference.start_line}-L${reference.end_line}`
  return `:L${reference.start_line}`
}

export function TraceLinkButton({ runID, onOpenTrace }: { runID?: string; onOpenTrace?: (runID: string) => void }) {
  const { t } = useTranslation()
  if (!runID || !onOpenTrace) return null
  return (
    <button
      type="button"
      onClick={() => onOpenTrace(runID)}
      className="nova-nav-item inline-flex h-6 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] px-1.5 text-[10px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
    >
      <Activity className="h-3 w-3" />
      {t('chat.tracePanel.viewTrace')}
    </button>
  )
}

export function RuleRollBlock({ message }: { message: RuleRollChatMessage }) {
  const { t } = useTranslation()
  const roll = message.rule_roll
  if (!roll) return null
  const rolls = roll.rolls?.length ? roll.rolls.join(', ') : '-'
  const kept = Number.isFinite(roll.kept_roll) ? roll.kept_roll : undefined
  const bonus = Number.isFinite(roll.bonus_total) ? roll.bonus_total : undefined
  const total = Number.isFinite(roll.total) ? roll.total : undefined
  const target = Number.isFinite(roll.target) ? roll.target : undefined
  const cost = roll.cost || roll.stakes || ''
  const stateChanges = roll.state_changes || []
  return (
    <div className="flex justify-start">
      <div className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-xs shadow-[var(--nova-shadow)]">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Dice5 className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 truncate font-semibold text-[var(--nova-text)]">{roll.label || t('snapshot.ruleRoll.title')}</span>
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{roll.difficulty || t('snapshot.noRecord')}</span>
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{[roll.dice, roll.roll_mode].filter(Boolean).join(' ') || t('snapshot.noRecord')}</span>
          {roll.outcome ? <span className={`ml-auto shrink-0 font-semibold ${ruleRollOutcomeClass(roll.outcome)}`}>{roll.outcome}</span> : null}
        </div>
        <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-[var(--nova-text-muted)]">
          <span>{t('snapshot.field.rolls')}: {rolls}</span>
          {kept !== undefined ? <span>{t('snapshot.field.kept_roll')}: {formatRuleRollNumber(kept)}</span> : null}
          {bonus !== undefined ? <span>{t('snapshot.field.bonus_total')}: {formatSignedRuleRollNumber(bonus)}</span> : null}
          {total !== undefined || target !== undefined ? <span>{t('snapshot.ruleRoll.totalTarget', { total: total !== undefined ? formatRuleRollNumber(total) : '-', target: target !== undefined ? formatRuleRollNumber(target) : '-' })}</span> : null}
          {Number.isFinite(roll.base_target) ? <span>{t('snapshot.field.base_target')}: {formatRuleRollNumber(roll.base_target || 0)}</span> : null}
        </div>
        {roll.result ? <div className="mt-1.5 text-[var(--nova-text)]">{roll.result}</div> : null}
        {cost ? <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('snapshot.ruleRoll.cost')}: {cost}</div> : null}
        {stateChanges.length ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {stateChanges.map((change, index) => (
              <span key={`${change.actor_id}:${change.field_id}:${index}`} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">
                {change.actor_id} / {change.field_id} {formatSignedRuleRollNumber(change.change)}
              </span>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function ruleRollOutcomeClass(outcome: string) {
  if (outcome.includes('success')) return 'text-[var(--nova-success)]'
  if (outcome.includes('failure')) return 'text-[var(--nova-danger)]'
  return 'text-[var(--nova-text-muted)]'
}

function formatRuleRollNumber(value: number) {
  if (!Number.isFinite(value)) return '-'
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function formatSignedRuleRollNumber(value: number) {
  if (!Number.isFinite(value)) return '-'
  const formatted = formatRuleRollNumber(value)
  return value > 0 ? `+${formatted}` : formatted
}

export function MessageInlineMeta({ projectId, message, content, align, reserveSpace = false, hideActions = false, onEdit, editLabelKey = 'chat.action.editTurn', onCreateBranch, onGenerateInteractiveImage, generatingInteractiveImage = false, interactiveImageGenerationDisabled = false, onRegenerate, onSwitchVersion, versionIndex = -1, versionCount = 0 }: { projectId?: string; message: ChatMessage; content: string; align: 'left' | 'right'; reserveSpace?: boolean; hideActions?: boolean; onEdit?: (message: ChatMessage) => void; editLabelKey?: 'chat.action.editTurn' | 'chat.action.editAssistantReply'; onCreateBranch?: (message: ChatMessage) => void; onGenerateInteractiveImage?: (message: ChatMessage) => void; generatingInteractiveImage?: boolean; interactiveImageGenerationDisabled?: boolean; onRegenerate?: (message: ChatMessage) => void; onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void; versionIndex?: number; versionCount?: number }) {
  const { t } = useTranslation()
  const trajectoryNavigation = useTrajectoryNavigation()
  const [copied, setCopied] = useState(false)
  const formatted = formatMessageHoverTime(message.created_at)
  const runID = message.run_id?.trim()
  const canSwitchVersion = Boolean(onSwitchVersion && versionCount > 1 && versionIndex >= 0)
  const canOpenTrajectory = !hideActions && trajectoryNavigation.enabled && Boolean(projectId && runID)
  const hasMessageAction = !hideActions && Boolean(onEdit || onCreateBranch || onGenerateInteractiveImage || onRegenerate || canSwitchVersion || canOpenTrajectory)
  const showCopyAction = !hideActions && Boolean(content.trim())
  const metaTooltip = {
    tooltipSide: 'top' as const,
    tooltipSideOffset: messageActionTooltipSideOffset,
    useTooltipProvider: false,
  }
  if (!formatted && !showCopyAction && !hasMessageAction) {
    if (!reserveSpace) return null
    return (
      <div className={`nova-message-meta nova-message-meta-${align} nova-message-meta-spacer`} aria-hidden="true">
        <span />
      </div>
    )
  }
  return (
    <TooltipProvider delayDuration={messageActionTooltipDelayMs} skipDelayDuration={messageActionTooltipSkipDelayMs} disableHoverableContent>
      <div className={`nova-message-meta nova-message-meta-${align}`} aria-label={formatted}>
        {formatted ? <span className="nova-message-time">{formatted}</span> : null}
        {showCopyAction && (
          <TooltipIconButton
            label={copied ? t('chat.action.copyMessageDone') : t('chat.action.copyMessage')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              setCopied(true)
              window.setTimeout(() => setCopied(false), copyFeedbackDurationMs)
              void copyText(content)
            }}
          >
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          </TooltipIconButton>
        )}
        {canOpenTrajectory && projectId && runID && (
          <TooltipIconButton
            label={t('trajectory.openRun')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              trajectoryNavigation.open({ projectId, runId: runID })
            }}
          >
            <Route className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {onEdit && (
          <TooltipIconButton
            label={t(editLabelKey)}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onEdit(message)
            }}
          >
            <Pencil className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {onCreateBranch && (
          <TooltipIconButton
            label={t('chat.action.createBranch')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onCreateBranch(message)
            }}
          >
            <GitBranch className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {onGenerateInteractiveImage && (
          <TooltipIconButton
            label={message.role === 'assistant' && (message.interactive_images?.length || message.interactive_image) ? t('chat.interactiveImage.regenerate') : t('chat.action.generateInteractiveImage')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-45"
            disabled={interactiveImageGenerationDisabled}
            onClick={(event) => {
              event.stopPropagation()
              onGenerateInteractiveImage(message)
            }}
          >
            {generatingInteractiveImage ? <Loader2 className="h-3 w-3 animate-spin" /> : <ImagePlus className="h-3 w-3" />}
          </TooltipIconButton>
        )}
        {onRegenerate && (
          <TooltipIconButton
            label={t('chat.action.regenerateTurn')}
            {...metaTooltip}
            className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
            onClick={(event) => {
              event.stopPropagation()
              onRegenerate(message)
            }}
          >
            <RefreshCw className="h-3 w-3" />
          </TooltipIconButton>
        )}
        {canSwitchVersion && onSwitchVersion && (
          <>
            <TooltipIconButton
              label={t('chat.action.prevVersion')}
              {...metaTooltip}
              className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
              disabled={versionIndex <= 0}
              onClick={(event) => {
                event.stopPropagation()
                onSwitchVersion(message, -1)
              }}
            >
              <ChevronLeft className="h-3 w-3" />
            </TooltipIconButton>
            <span className="min-w-7 text-center font-mono text-[10px] leading-5 text-[var(--nova-text-faint)]">
              {versionIndex + 1}/{versionCount}
            </span>
            <TooltipIconButton
              label={t('chat.action.nextVersion')}
              {...metaTooltip}
              className="h-5 w-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-30"
              disabled={versionIndex >= versionCount - 1}
              onClick={(event) => {
                event.stopPropagation()
                onSwitchVersion(message, 1)
              }}
            >
              <ChevronRight className="h-3 w-3" />
            </TooltipIconButton>
          </>
        )}
      </div>
    </TooltipProvider>
  )
}

function formatMessageHoverTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const time = `${padTime(date.getHours())}:${padTime(date.getMinutes())}`
  const now = new Date()
  const sameDay = date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  if (sameDay) return time
  return `${date.getFullYear()}-${padTime(date.getMonth() + 1)}-${padTime(date.getDate())} ${time}`
}

function padTime(value: number) {
  return value.toString().padStart(2, '0')
}

async function copyText(content: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(content)
      return true
    } catch {
      // Fall through to the legacy path for embedded/local browser surfaces.
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = content
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

export function SubAgentOutputWindow({
  message,
  content,
  highlightDialogue,
  messageStyle,
  projectId,
  onOpen,
  active,
}: {
  message: ChatMessage
  content: string
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  projectId: string
  onOpen?: (message: ChatMessage) => void
  active?: boolean
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const name = message.agent_name || message.subagent_type || t('chat.subagent.label')
  const preview = buildMarkdownPreview(content, 220)
  const hasContent = Boolean(content.trim())
  const statusLabel = t(subAgentStatusTranslationKey(message.subagent_status, message.streaming === true))
  const detailMode = Boolean(onOpen)
  const actionLabel = detailMode ? t('chat.subagent.openSession') : (expanded ? t('chat.subagent.collapse') : t('chat.subagent.expand'))
  const shownContent = detailMode || !expanded ? preview : content
  const shownTargetContent = message.streaming_target_content && shownContent === content ? message.streaming_target_content : undefined
  const contentScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: message.streaming === true,
    resetKey: `${message.id || message.created_at || name}:subagent-output`,
    contentKey: `${message.streaming ? 'streaming' : 'idle'}:${detailMode ? 'detail' : 'inline'}:${expanded ? 'expanded' : 'collapsed'}:${shownContent.length}`,
  })

  return (
    <div className="flex justify-start">
      <div className={`w-full overflow-hidden rounded-lg border bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)] ${active ? 'border-[var(--nova-accent)] ring-1 ring-[var(--nova-accent)]/40' : 'border-[var(--nova-border)]'}`}>
        <button
          type="button"
          className="flex min-h-10 w-full min-w-0 items-center gap-2 px-3 py-2 text-left"
          onClick={() => {
            if (onOpen) {
              onOpen(message)
              return
            }
            setExpanded(!expanded)
          }}
          aria-expanded={expanded}
          aria-label={t('chat.subagent.outputFrom', { name })}
        >
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            {detailMode ? <PanelRightOpen className="h-3.5 w-3.5" /> : expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium text-[var(--nova-text)]">{t('chat.subagent.outputFrom', { name })}</span>
            <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{statusLabel}</span>
          </span>
          <span className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">
            {actionLabel}
          </span>
        </button>
        <div
          ref={contentScrollLock.ref}
          onScroll={contentScrollLock.onScroll}
          onWheel={contentScrollLock.onWheel}
          onKeyDown={contentScrollLock.onKeyDown}
          data-nova-scroll-lock="subagent-output"
          className={`${detailMode ? 'max-h-28' : expanded ? 'max-h-96' : 'max-h-28'} overflow-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 [overflow-anchor:none]`}
        >
          {hasContent ? (
            <div className="chat-agent-message text-sm text-[var(--nova-text)]" style={messageStyle}>
              <StreamingContentStage content={shownContent} targetContent={shownTargetContent} streaming={message.streaming === true}>
                {(value) => (
                  <MarkdownContent
                    content={value}
                    highlightDialogue={highlightDialogue}
                    projectId={projectId}
                    streaming={message.streaming === true}
                  />
                )}
              </StreamingContentStage>
            </div>
          ) : (
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.subagent.empty')}</div>
          )}
        </div>
      </div>
    </div>
  )
}
