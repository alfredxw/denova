import { useState, useRef, useEffect, useLayoutEffect, useMemo, useCallback, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Archive, BadgeHelp, BarChart3, ClipboardList, Eraser, List, Paperclip, ScrollText, Sparkles, Target } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FileReferencePicker, type FileReferencePickerHandle, type ReferencePickerItem } from './FileReferencePicker'
import { TokenUsageDialog, type TokenUsageRecord } from './TokenUsagePanel'
import type { AgentRuntimeQueuedCommand, TextSelection } from '@/lib/api'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { Button } from '@/components/ui/button'
import { AgentComposerShell } from './AgentComposerShell'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'
import { ComposerTokenInput, type ComposerTokenInputHandle, type ComposerTokenSpec, type ComposerTrigger } from './composer-token-input'
import { workspaceFileName } from '@/lib/workspace-path'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useKeyboardInset } from '@/hooks/useKeyboardInset'
import { useIsMobile } from '@/hooks/useIsMobile'
import { ReviewFeedbackTray, reviewFeedbackCommentCount, type ReviewFeedbackBatch, type ReviewFeedbackComment, type ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import { AgentComposerControls } from './AgentComposerControls'
import { AgentQueuedCommandList } from './AgentQueuedCommandList'
import { AgentGoalCard } from './AgentGoalCard'
import { useAgentApprovalMode } from '@/features/agent-approval/AgentApprovalProvider'
import { AgentApprovalModeMenu } from '@/features/agent-approval/AgentApprovalModeMenu'
import { InputCommandMenu, type InputCommandOption } from './InputCommandMenu'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import { cn } from '@/lib/utils'
import type { ConversationGoal } from '@/features/agent-goal/types'
import { ComposerModeChip } from './ComposerModeChip'
import { ComposerAttachmentTray, useComposerAttachments } from './ComposerAttachments'

/** 可用命令列表 */
const COMMANDS: Array<{ cmd: string; descKey: string; hintKey: string; icon: LucideIcon }> = [
  { cmd: '/goal', descKey: 'chat.command.goal.desc', hintKey: 'chat.command.goal.hint', icon: Target },
  { cmd: '/plan', descKey: 'chat.command.plan.desc', hintKey: 'chat.command.plan.hint', icon: ClipboardList },
  { cmd: '/clear', descKey: 'chat.command.clear.desc', hintKey: 'chat.command.clear.hint', icon: Eraser },
  { cmd: '/compact', descKey: 'chat.command.compact.desc', hintKey: 'chat.command.compact.hint', icon: Archive },
  { cmd: '/status', descKey: 'chat.command.status.desc', hintKey: 'chat.command.status.hint', icon: Sparkles },
  { cmd: '/help', descKey: 'chat.command.help.desc', hintKey: 'chat.command.help.hint', icon: BadgeHelp },
]

interface SkillCommand {
  name: string
  description: string
}

type CommandScope = 'all' | 'skills' | 'none'
type BuiltinCommand = typeof COMMANDS[number]['cmd']
const MAX_TOKEN_USAGE_MENU_COUNT = 10
const inputDrafts = new Map<string, string>()

export interface InputAreaSendOptions {
  attachments?: File[]
}

interface InputAreaProps {
  onSend: (message: string, options?: InputAreaSendOptions) => boolean | void | Promise<boolean | void>
  onStop?: () => void
  disabled: boolean
  /** Agent execution and editor availability are independent: active runs can still accept instructions. */
  generationActive: boolean
  queuedCommands?: AgentRuntimeQueuedCommand[]
  queueActionPendingCommandID?: string
  onQueuedCommandSteer?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
  onQueuedCommandDelete?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
  onQueuedCommandEdit?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
  abortPending?: boolean
  commandSubmitting?: boolean
  activeControlsDisabled?: boolean
  activeStopDisabled?: boolean
  sendBlocked?: boolean
  planMode?: boolean
  onTogglePlanMode?: () => void
  goal?: ConversationGoal | null
  goalPending?: boolean
  onGoalSubmit?: (objective: string, options?: InputAreaSendOptions) => boolean | void | Promise<boolean | void>
  onGoalPause?: () => void | Promise<void>
  onGoalClear?: () => void | Promise<void>
  draftKey?: string
  inputPrefill?: { prompt: string; nonce: number } | null
  onInputPrefillConsumed?: () => void
  referencedFiles?: string[]
  onReferenceRemove?: (path: string) => void
  fileSuggestions?: string[]
  loreReferences?: string[]
  loreReferenceLabels?: Record<string, string>
  onLoreReferenceAdd?: (id: string) => void
  onLoreReferenceRemove?: (id: string) => void
  loreSuggestions?: ReferencePickerItem[]
  styleScenes?: string[]
  onStyleSceneAdd?: (scene: string) => void
  onStyleSceneRemove?: (scene: string) => void
  styleSceneSuggestions?: string[]
  textSelections?: TextSelection[]
  onTextSelectionRemove?: (index: number) => void
  reviewFeedback?: ReviewFeedbackBatch | null
  onReviewFeedbackOpen?: (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => void
  onReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  skills?: SkillCommand[]
  commandsEnabled?: boolean
  commandScope?: CommandScope
  builtinCommands?: BuiltinCommand[]
  placeholder?: string
  disabledPlaceholder?: string
  onContextAnalyze?: (message: string) => void | Promise<void>
  tokenUsageMessages?: TokenUsageRecord[]
  onOpenTrace?: (runID: string) => void
  agentKey?: VisibleAgentKey
  workspace?: string
  conversationBinding?: ConversationConfigBinding
  composerSettingsControl?: ReactNode
  attachmentsEnabled?: boolean
  onboardingAnchor?: string
  floating?: boolean
  onHeightChange?: (height: number) => void
  /** Keeps the composer and its attached UI aligned with the conversation timeline. */
  contentClassName?: string
}

/** 输入区域组件，支持 Enter 发送和命令菜单 */
export function InputArea({
  onSend,
  onStop,
  disabled,
  generationActive,
  queuedCommands = [],
  queueActionPendingCommandID = '',
  onQueuedCommandSteer,
  onQueuedCommandDelete,
  onQueuedCommandEdit,
  abortPending = false,
  commandSubmitting = false,
  activeControlsDisabled = false,
  activeStopDisabled,
  sendBlocked = false,
  planMode = false,
  onTogglePlanMode,
  goal,
  goalPending = false,
  onGoalSubmit,
  onGoalPause,
  onGoalClear,
  draftKey,
  inputPrefill,
  onInputPrefillConsumed,
  referencedFiles = [],
  onReferenceRemove,
  fileSuggestions = [],
  loreReferences = [],
  loreReferenceLabels = {},
  onLoreReferenceAdd,
  onLoreReferenceRemove,
  loreSuggestions = [],
  styleScenes = [],
  onStyleSceneAdd,
  onStyleSceneRemove,
  styleSceneSuggestions = [],
  textSelections = [],
  onTextSelectionRemove,
  reviewFeedback,
  onReviewFeedbackOpen,
  onReviewFeedbackRemove,
  skills = [],
  commandsEnabled = true,
  commandScope = 'all',
  builtinCommands,
  placeholder,
  disabledPlaceholder,
  onContextAnalyze,
  tokenUsageMessages = [],
  onOpenTrace,
  agentKey,
  workspace,
  conversationBinding,
  composerSettingsControl,
  attachmentsEnabled = false,
  onboardingAnchor,
  floating = false,
  onHeightChange,
  contentClassName,
}: InputAreaProps) {
  const { t } = useTranslation()
  const defaultApproval = useAgentApprovalMode()
  const conversationConfig = useConversationConfig(conversationBinding)
  const approvalReady = conversationBinding
    ? conversationConfig.initialized && !conversationConfig.saving
    : defaultApproval.initialized && !defaultApproval.saving
  const keyboardInset = useKeyboardInset()
  const isMobile = useIsMobile()
  const [value, setValue] = useState(() => draftKey ? inputDrafts.get(draftKey) || '' : '')
  const [tokenUsageOpen, setTokenUsageOpen] = useState(false)
  const [showCommands, setShowCommands] = useState(false)
  const [commandQuery, setCommandQuery] = useState<string | null>(null)
  const [activeCommandIndex, setActiveCommandIndex] = useState(0)
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [goalMode, setGoalMode] = useState(false)
  const inputRef = useRef<ComposerTokenInputHandle>(null)
  const referencePickerRef = useRef<FileReferencePickerHandle>(null)
  const stylePickerRef = useRef<FileReferencePickerHandle>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const submittingRef = useRef(false)
  const attachments = useComposerAttachments(attachmentsEnabled && !disabled, draftKey ? `chat:${draftKey}` : undefined)
  const effectiveCommandScope: CommandScope = commandsEnabled ? commandScope : 'none'
  const defaultPlaceholder = skills.length > 0 && effectiveCommandScope !== 'none'
    ? t('chat.input.placeholderWithSkills')
    : t('chat.input.placeholder')
  const allCommands = useMemo<InputCommandOption[]>(() => {
    const allowedBuiltinCommands = builtinCommands ? new Set<string>(builtinCommands) : null
    const staticCommands = effectiveCommandScope === 'all'
      ? COMMANDS
        .filter(({ cmd }) => cmd !== '/goal' || Boolean(onGoalSubmit))
        .filter(({ cmd }) => !allowedBuiltinCommands || allowedBuiltinCommands.has(cmd))
        .map(({ cmd, descKey, hintKey, icon }) => ({
          cmd,
          description: t(descKey),
          hint: t(hintKey),
          icon,
          source: 'builtin' as const,
        }))
      : []
    const seen = new Set(staticCommands.map((command) => command.cmd))
    const skillCommands = skills
      .map((skill) => ({
        cmd: `/${skill.name}`,
        description: skill.description || skill.name,
        hint: t('chat.command.skill.hint'),
        icon: Sparkles,
        source: 'skill' as const,
      }))
      .filter((command) => {
        if (seen.has(command.cmd)) return false
        seen.add(command.cmd)
        return true
      })
    if (effectiveCommandScope === 'skills') return skillCommands
    if (effectiveCommandScope === 'none') return []
    return [...staticCommands, ...skillCommands]
  }, [builtinCommands, effectiveCommandScope, onGoalSubmit, skills, t])
  const filteredCommands = useMemo(() => {
    if (commandQuery === null) return []
    const query = `/${commandQuery}`.toLowerCase()
    return allCommands.filter((command) => command.cmd.toLowerCase().startsWith(query))
  }, [allCommands, commandQuery])
  const filteredBuiltinCommands = useMemo(() => filteredCommands
    .map((command, index) => ({ command, index }))
    .filter(({ command }) => command.source === 'builtin'), [filteredCommands])
  const filteredSkillCommands = useMemo(() => filteredCommands
    .map((command, index) => ({ command, index }))
    .filter(({ command }) => command.source === 'skill'), [filteredCommands])
  const hasReviewFeedback = Boolean(reviewFeedback && reviewFeedbackCommentCount(reviewFeedback) > 0)
  const hasReferences = textSelections.length > 0 || hasReviewFeedback || attachments.items.length > 0
  const knownFileTokens = useMemo(() => Array.from(new Set([...fileSuggestions, ...referencedFiles])), [fileSuggestions, referencedFiles])
  const knownLoreTokens = useMemo(() => {
    const byID = new Map<string, string>()
    for (const item of loreSuggestions) byID.set(item.value, item.label)
    for (const id of loreReferences) byID.set(id, loreReferenceLabels[id] || byID.get(id) || id)
    return Array.from(byID.entries()).map(([id, label]) => ({ id, label }))
  }, [loreReferenceLabels, loreReferences, loreSuggestions])
  const referencePickerItems = useMemo<ReferencePickerItem[]>(() => [
    ...loreSuggestions.map((item) => ({ ...item, kind: 'lore' as const })),
    ...fileSuggestions.map((path) => {
      const label = workspaceFileName(path)
      return { value: path, label, description: label === path ? undefined : path, kind: 'file' as const }
    }),
  ], [fileSuggestions, loreSuggestions])
  const externalTokens = useMemo<ComposerTokenSpec[]>(() => [
    ...referencedFiles.map((path) => ({ kind: 'file' as const, value: path, label: workspaceFileName(path) })),
    ...loreReferences.map((id) => ({ kind: 'lore' as const, value: id, label: loreReferenceLabels[id] || knownLoreTokens.find((item) => item.id === id)?.label || id })),
    ...styleScenes.map((scene) => ({ kind: 'style' as const, value: scene, label: scene })),
  ], [knownLoreTokens, loreReferenceLabels, loreReferences, referencedFiles, styleScenes])
  const tokenUsageCount = useMemo(
    () => Math.min(MAX_TOKEN_USAGE_MENU_COUNT, tokenUsageMessages.filter((message) => (!message.role || message.role === 'token_usage') && Number(message.model_calls || 0) > 0).length),
    [tokenUsageMessages],
  )
  useEffect(() => {
    if (!draftKey) return
    setValue(inputDrafts.get(draftKey) || '')
    setShowCommands(false)
    setCommandQuery(null)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
    setGoalMode(false)
  }, [draftKey])

  useEffect(() => {
    if (planMode) setGoalMode(false)
  }, [planMode])

  useEffect(() => {
    if (!draftKey) return
    if (value) inputDrafts.set(draftKey, value)
    else inputDrafts.delete(draftKey)
  }, [draftKey, value])

  // The initial AI SDK promise spans the response stream. Once generation is
  // visibly active, release the request-level composer lock so operation-scoped
  // Follow Up/Steer commands can be submitted independently.
  useEffect(() => {
    if (!generationActive || !submittingRef.current) return
    submittingRef.current = false
    setSubmitting(false)
  }, [generationActive])

  useEffect(() => {
    if (activeCommandIndex >= filteredCommands.length) setActiveCommandIndex(0)
  }, [activeCommandIndex, filteredCommands.length])

  useEffect(() => {
    if (!inputPrefill) return
    setValue(inputPrefill.prompt)
    setShowCommands(false)
    setCommandQuery(null)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
    window.requestAnimationFrame(() => inputRef.current?.focus())
    onInputPrefillConsumed?.()
  }, [inputPrefill, onInputPrefillConsumed])

  const syncHeight = useCallback(() => {
    const element = rootRef.current
    if (!element) return
    const height = Math.ceil(element.getBoundingClientRect().height)
    // Floating composers pin to the layout-viewport bottom, so on iOS the
    // on-screen keyboard covers them. They lift by `keyboardInset` (see the
    // root style below), and the clearance a message list must reserve is the
    // composer height plus that inset. Non-floating composers are in normal
    // flow and ignore the inset.
    onHeightChange?.(floating ? height + keyboardInset : height)
  }, [onHeightChange, floating, keyboardInset])

  useLayoutEffect(() => {
    syncHeight()
  }, [value, hasReferences, showCommands, referenceQuery, styleSceneQuery, externalTokens, attachments.items.length, syncHeight])

  useEffect(() => {
    if (!onHeightChange) return
    const element = rootRef.current
    if (!element || typeof ResizeObserver === 'undefined') {
      syncHeight()
      return
    }
    const observer = new ResizeObserver(syncHeight)
    observer.observe(element)
    return () => observer.disconnect()
  }, [onHeightChange, syncHeight])

  /** 处理输入变化 */
  const handleChange = (nextValue: string) => {
    setValue(nextValue)
  }

  const handleTriggerChange = (trigger: ComposerTrigger | null) => {
    if (effectiveCommandScope !== 'none' && trigger?.kind === 'slash') {
      setCommandQuery(trigger.query)
      setShowCommands(true)
      setActiveCommandIndex(0)
    } else {
      setCommandQuery(null)
      setShowCommands(false)
      setActiveCommandIndex(0)
    }
    setReferenceQuery(trigger?.kind === 'reference' ? trigger.query : null)
    setStyleSceneQuery(trigger?.kind === 'style' ? trigger.query : null)
  }

  const setGoalModeExclusive = (next: boolean) => {
    setGoalMode(next)
    if (next && planMode) onTogglePlanMode?.()
  }

  const togglePlanModeExclusive = () => {
    if (!planMode) setGoalMode(false)
    onTogglePlanMode?.()
  }

  /** 处理键盘事件 */
  const handleKeyDown = (e: KeyboardEvent) => {
    const isMod = e.metaKey || e.ctrlKey
    const canPickCommand = effectiveCommandScope !== 'none' && showCommands && filteredCommands.length > 0

    if (e.key === 'Tab' && e.shiftKey && onTogglePlanMode && !disabled) {
      e.preventDefault()
      togglePlanModeExclusive()
      return true
    }

    if (canPickCommand && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault()
      setActiveCommandIndex((current) => {
        const direction = e.key === 'ArrowDown' ? 1 : -1
        return (current + direction + filteredCommands.length) % filteredCommands.length
      })
      return true
    }

    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const picker = referenceQuery !== null ? referencePickerRef.current : styleSceneQuery !== null ? stylePickerRef.current : null
      if (picker?.moveActive(e.key === 'ArrowDown' ? 1 : -1)) {
        e.preventDefault()
        return true
      }
    }

    // Enter 发送
    if (e.key === 'Enter' && !e.shiftKey) {
      if (isNativeComposingKeyboardEvent(e)) return false
      e.preventDefault()
      if (canPickCommand) {
        selectCommand(filteredCommands[activeCommandIndex]?.cmd || filteredCommands[0].cmd)
        return true
      }
      if (referenceQuery !== null && referencePickerRef.current?.selectActive()) return true
      if (styleSceneQuery !== null && stylePickerRef.current?.selectActive()) return true
      handleSend()
      return true
    }

    if (e.key === 'Tab' && !e.shiftKey) {
      if (canPickCommand) {
        e.preventDefault()
        selectCommand(filteredCommands[activeCommandIndex]?.cmd || filteredCommands[0].cmd)
        return true
      }
      const picker = referenceQuery !== null ? referencePickerRef.current : styleSceneQuery !== null ? stylePickerRef.current : null
      if (picker?.selectActive()) {
        e.preventDefault()
        return true
      }
    }

    // Escape 关闭菜单
    if (e.key === 'Escape') {
      setShowCommands(false)
      setCommandQuery(null)
      setActiveCommandIndex(0)
      setReferenceQuery(null)
      setStyleSceneQuery(null)
      return true
    }

    // Cmd+A：全选输入框内容（阻止冒泡，防止被全局事件拦截）
    if (isMod && e.key === 'a') {
      e.stopPropagation()
      inputRef.current?.select()
      return true
    }

    // Cmd+Backspace：删除光标到行首
    if (isMod && e.key === 'Backspace') {
      e.preventDefault()
      inputRef.current?.deleteToLineStart()
      return true
    }

    // Cmd+Shift+K：删除整行
    if (isMod && e.shiftKey && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      inputRef.current?.deleteCurrentLine()
      return true
    }

    // Cmd+D：选择当前词（类 VSCode 行为）
    if (isMod && e.key.toLowerCase() === 'd') {
      e.preventDefault()
      inputRef.current?.selectCurrentWord()
      return true
    }
    return false
  }

  /** 发送消息 */
  const handleSend = () => {
    const trimmed = value.trim()
    if ((!trimmed && !hasReviewFeedback && attachments.files.length === 0) || disabled || !approvalReady || submittingRef.current) return
    const submittedValue = value
    const submittedAttachments = attachments.files
    submittingRef.current = true
    setSubmitting(true)
    let result: ReturnType<typeof onSend>
    try {
      if (goalMode && onGoalSubmit) {
        result = submittedAttachments.length
          ? onGoalSubmit(trimmed, { attachments: submittedAttachments })
          : onGoalSubmit(trimmed)
      } else {
        result = submittedAttachments.length
          ? onSend(trimmed, { attachments: submittedAttachments })
          : onSend(trimmed)
      }
    } catch {
      submittingRef.current = false
      setSubmitting(false)
      return
    }
    setValue('')
    attachments.clear()
    setShowCommands(false)
    setCommandQuery(null)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
    if (result && typeof (result as PromiseLike<boolean | void>).then === 'function') {
      void Promise.resolve(result).then((accepted) => {
        if (accepted === false) {
          setValue((current) => current || submittedValue)
          attachments.addFiles(submittedAttachments)
        }
        else if (goalMode) setGoalMode(false)
      }).catch(() => {
        setValue((current) => current || submittedValue)
        attachments.addFiles(submittedAttachments)
      }).finally(() => {
        submittingRef.current = false
        setSubmitting(false)
      })
    } else if (result === false) {
      setValue(submittedValue)
      attachments.addFiles(submittedAttachments)
      submittingRef.current = false
      setSubmitting(false)
    } else {
      if (goalMode) setGoalMode(false)
      submittingRef.current = false
      setSubmitting(false)
    }
  }

  const handleContextAnalyze = () => {
    if (disabled) return
    void onContextAnalyze?.(value)
  }
  /** 选择命令 */
  const selectCommand = (cmd: string) => {
    const command = allCommands.find((item) => item.cmd === cmd)
    if (cmd === '/goal' && onGoalSubmit) {
      inputRef.current?.replaceActiveTriggerText('')
      setGoalModeExclusive(true)
    } else if (command?.source === 'skill') {
      const name = cmd.replace(/^\//, '')
      inputRef.current?.replaceActiveTriggerWithToken({ kind: 'skill', value: name, label: name })
    } else {
      inputRef.current?.replaceActiveTriggerText(`${cmd} `)
    }
    setShowCommands(false)
    setCommandQuery(null)
    setActiveCommandIndex(0)
    inputRef.current?.focus()
  }

  const editGoal = () => {
    if (!goal) return
    setValue(goal.objective)
    setGoalModeExclusive(true)
    window.requestAnimationFrame(() => inputRef.current?.focus())
  }

  /** 选择引用文件：输入框只显示文件名，发送值仍保留完整 workspace 路径。 */
  const selectReference = (item: ReferencePickerItem) => {
    if (item.kind === 'lore') {
      inputRef.current?.replaceActiveTriggerWithToken({ kind: 'lore', value: item.value, label: item.label })
      onLoreReferenceAdd?.(item.value)
    } else {
      inputRef.current?.replaceActiveTriggerWithToken({ kind: 'file', value: item.value, label: item.label })
    }
    setReferenceQuery(null)
    inputRef.current?.focus()
  }

  /** 选择场景风格并插入 #scene 标签 */
  const selectStyleScene = ({ value: scene }: ReferencePickerItem) => {
    inputRef.current?.replaceActiveTriggerWithToken({ kind: 'style', value: scene, label: scene })
    onStyleSceneAdd?.(scene)
    setStyleSceneQuery(null)
    inputRef.current?.focus()
  }

  const handleTokenRemove = (token: ComposerTokenSpec) => {
    if (token.kind === 'file' && referencedFiles.includes(token.value)) onReferenceRemove?.(token.value)
    if (token.kind === 'lore' && loreReferences.includes(token.value)) onLoreReferenceRemove?.(token.value)
    if (token.kind === 'style' && styleScenes.includes(token.value)) onStyleSceneRemove?.(token.value)
  }

  return (
    <div
      ref={rootRef}
      {...attachments.dropProps}
      data-onboarding-anchor={onboardingAnchor}
      style={floating ? { bottom: keyboardInset } : undefined}
      className={cn(
        floating ? 'nova-chat-input-area nova-chat-input-area-floating' : 'nova-chat-input-area relative border-t border-[var(--nova-border)] p-3',
        floating && contentClassName && 'nova-chat-input-area-content-aligned',
      )}
    >
      <div className={cn('relative', contentClassName, contentClassName && 'px-6')}>
        <InputCommandMenu
          open={showCommands && filteredCommands.length > 0}
          skillsOnly={effectiveCommandScope === 'skills'}
          builtinCommands={filteredBuiltinCommands}
          skillCommands={filteredSkillCommands}
          activeIndex={activeCommandIndex}
          onActiveIndexChange={setActiveCommandIndex}
          onSelect={(command) => selectCommand(command.cmd)}
        />

        <FileReferencePicker
          ref={referencePickerRef}
          open={referenceQuery !== null && referencePickerItems.length > 0}
          query={referenceQuery || ''}
          items={referencePickerItems}
          onSelect={selectReference}
        />

        <FileReferencePicker
          ref={stylePickerRef}
          open={styleSceneQuery !== null && styleSceneSuggestions.length > 0}
          query={styleSceneQuery || ''}
          items={styleSceneSuggestions}
          onSelect={selectStyleScene}
          trigger="#"
          placeholder={t('chat.styleReference.placeholder')}
          emptyText={t('chat.styleReference.empty')}
          heading={t('chat.styleReference.heading')}
        />

        <AgentQueuedCommandList
          items={queuedCommands}
          pendingCommandID={queueActionPendingCommandID}
          disabled={activeControlsDisabled || abortPending || commandSubmitting}
          onSteer={onQueuedCommandSteer}
          onDelete={onQueuedCommandDelete}
          onEdit={onQueuedCommandEdit}
        />

        {goal && onGoalPause && onGoalClear ? (
          <AgentGoalCard
            goal={goal}
            pending={goalPending}
            disabled={disabled || activeControlsDisabled}
            onEdit={editGoal}
            onPause={onGoalPause}
            onClear={onGoalClear}
          />
        ) : null}

        <AgentComposerShell
          references={hasReferences ? (
            <>
              <ComposerAttachmentTray items={attachments.items} onRemove={attachments.remove} />
              {reviewFeedback && onReviewFeedbackRemove ? (
                <ReviewFeedbackTray feedback={reviewFeedback} onOpen={onReviewFeedbackOpen} onRemove={onReviewFeedbackRemove} />
              ) : null}
              {textSelections.length > 0 && (
                <div className="mb-2 flex flex-wrap gap-1.5">
                  {textSelections.map((sel, idx) => (
                    <span
                      key={idx}
                      className="inline-flex max-w-full items-center gap-1 rounded-md bg-[var(--nova-success-bg)] px-2 py-0.5 text-xs text-[var(--nova-success)]"
                    >
                      <span className="truncate">
                        {sel.fileName}:L{sel.startLine}
                        {sel.endLine !== sel.startLine && `-L${sel.endLine}`}
                        {' '}
                        <span className="text-[var(--nova-success-muted)]">
                          {sel.content.length > 30 ? sel.content.slice(0, 30) + '…' : sel.content}
                        </span>
                      </span>
                      {onTextSelectionRemove && (
                        <button
                          type="button"
                          className="rounded text-[var(--nova-success-muted)] hover:text-[var(--nova-text)]"
                          onClick={() => onTextSelectionRemove(idx)}
                        >
                          ×
                        </button>
                      )}
                    </span>
                  ))}
                </div>
              )}
            </>
          ) : undefined}
          input={
            <ComposerTokenInput
              ref={inputRef}
              value={value}
              onChange={handleChange}
              onTriggerChange={handleTriggerChange}
              onTokenRemove={handleTokenRemove}
              onEditorKeyDown={handleKeyDown}
              knownSkills={skills.map((skill) => skill.name)}
              knownFiles={knownFileTokens}
              knownLore={knownLoreTokens}
              knownStyleScenes={styleSceneSuggestions}
              externalTokens={externalTokens}
              placeholder={disabled
                ? (disabledPlaceholder ?? t('chat.input.disabledPlaceholder'))
                : goalMode ? t('chat.goal.placeholder') : (placeholder ?? defaultPlaceholder)}
              disabled={disabled}
              rows={1}
              minRows={1}
              maxRows={isMobile ? 5 : 10}
              multilineMode="always"
              enterKeyHint="send"
              className="nova-agent-composer-textarea nova-agent-token-input min-h-[42px] resize-none border-0 bg-transparent px-1 py-[9px] text-sm leading-6 text-[var(--nova-text)] shadow-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0 disabled:opacity-50"
            />
          }
          toolbarStart={
            <>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    size="icon-sm"
                    className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45"
                    disabled={!attachmentsEnabled && !onGoalSubmit && !onTogglePlanMode && !composerSettingsControl && !onContextAnalyze && tokenUsageMessages.length === 0}
                    aria-label={t('chat.input.actions')}
                  >
                    <List className="h-3.5 w-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="top" className="w-80 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                  {attachmentsEnabled ? (
                    <DropdownMenuItem
                      disabled={disabled}
                      onSelect={attachments.openPicker}
                      className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                    >
                      <Paperclip className="h-3.5 w-3.5" />
                      {t('chat.attachment.add')}
                    </DropdownMenuItem>
                  ) : null}
                  {onGoalSubmit || onTogglePlanMode ? (
                    <>
                      {onGoalSubmit ? (
                        <DropdownMenuCheckboxItem
                          checked={goalMode}
                          disabled={disabled}
                          onCheckedChange={(checked) => setGoalModeExclusive(checked === true)}
                          className="cursor-pointer pr-1.5 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)] [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:static [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:order-2 [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:size-4"
                        >
                          <Target className="h-3.5 w-3.5" />
                          <span className="min-w-0 flex-1">{t('chat.goal.short')}</span>
                        </DropdownMenuCheckboxItem>
                      ) : null}
                      {onTogglePlanMode ? (
                        <DropdownMenuCheckboxItem
                          checked={planMode}
                          disabled={disabled || generationActive}
                          onCheckedChange={togglePlanModeExclusive}
                          className="cursor-pointer pr-1.5 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)] [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:static [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:order-2 [&_[data-slot=dropdown-menu-checkbox-item-indicator]]:size-4"
                        >
                          <ClipboardList className="h-3.5 w-3.5" />
                          <span className="min-w-0 flex-1">{t('chat.plan.short')}</span>
                          <span className="order-3 ml-auto shrink-0 text-[10px] text-[var(--nova-text-faint)]">Shift+Tab</span>
                        </DropdownMenuCheckboxItem>
                      ) : null}
                    </>
                  ) : null}
                  {composerSettingsControl}
                  <DropdownMenuItem
                    onSelect={() => setTokenUsageOpen(true)}
                    className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                  >
                    <BarChart3 className="h-3.5 w-3.5" />
                    <span className="min-w-0 flex-1">{t('chat.tokenUsage.action')}</span>
                    <span className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.subtitle', { count: tokenUsageCount })}</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={disabled || generationActive}
                    onSelect={handleContextAnalyze}
                    className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                  >
                    <ScrollText className="h-3.5 w-3.5" />
                    {t('chat.contextAnalysis.action')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <AgentApprovalModeMenu runActive={generationActive} conversationConfig={conversationBinding ? conversationConfig : undefined} />
              {planMode && onTogglePlanMode ? (
                <ComposerModeChip
                  icon={ClipboardList}
                  label={t('chat.plan.short')}
                  ariaLabel={t('chat.plan.exit')}
                  disabled={disabled || generationActive}
                  onClose={() => {
                    togglePlanModeExclusive()
                    window.requestAnimationFrame(() => inputRef.current?.focus())
                  }}
                />
              ) : goalMode && onGoalSubmit ? (
                <ComposerModeChip
                  icon={Target}
                  label={t('chat.goal.short')}
                  ariaLabel={t('chat.goal.exitMode')}
                  disabled={disabled}
                  onClose={() => {
                    setGoalMode(false)
                    window.requestAnimationFrame(() => inputRef.current?.focus())
                  }}
                />
              ) : null}
              <TokenUsageDialog open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} onOpenTrace={onOpenTrace} />
              {attachments.input}
            </>
          }
          toolbarEnd={<ModelProfileSwitcher agentKey={agentKey} workspace={workspace} conversationConfig={conversationBinding ? conversationConfig : undefined} disabled={disabled || generationActive} />}
          submitControl={(
            <AgentComposerControls
              generationActive={generationActive}
              hasSendableContent={Boolean(value.trim() || hasReviewFeedback || attachments.files.length)}
              onStop={onStop}
              onSend={handleSend}
              sendDisabled={sendBlocked || !approvalReady || submitting || (!value.trim() && !hasReviewFeedback && attachments.files.length === 0)}
              disabled={disabled}
              abortPending={abortPending}
              actionPending={commandSubmitting}
              activeControlsDisabled={activeControlsDisabled}
              stopDisabled={activeStopDisabled}
            />
          )}
        />
      </div>
    </div>
  )

}

function isNativeComposingKeyboardEvent(event: KeyboardEvent) {
  return event.isComposing || event.key === 'Process' || event.keyCode === 229
}
