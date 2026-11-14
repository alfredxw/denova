import { useId, useRef, useState, type CSSProperties, type Dispatch, type ReactNode, type RefObject, type SetStateAction } from 'react'
import type { TFunction } from 'i18next'
import { Archive, BarChart3, ChevronDown, Compass, List, Paperclip, Pencil, RefreshCw, ScrollText, Sparkles, X } from 'lucide-react'
import { AgentComposerControls } from '@/components/Chat/AgentComposerControls'
import { AgentComposerShell } from '@/components/Chat/AgentComposerShell'
import { AgentQueuedCommandList } from '@/components/Chat/AgentQueuedCommandList'
import { ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import { FileReferencePicker, type FileReferencePickerHandle } from '@/components/Chat/FileReferencePicker'
import { InputCommandMenu, type IndexedInputCommandOption } from '@/components/Chat/InputCommandMenu'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { ImageGenerationSettingsMenu } from '@/components/Chat/ImageGenerationSettingsMenu'
import { TokenUsageDialog } from '@/components/Chat/TokenUsagePanel'
import { ComposerMenuItem } from '@/components/Chat/ComposerMenuRow'
import { ComposerTokenInput, type ComposerTokenInputHandle, type ComposerTokenSpec, type ComposerTrigger } from '@/components/Chat/composer-token-input'
import { Button } from '@/components/ui/button'
import { motion, useReducedMotionConfig } from 'motion/react'
import { novaEase } from '@/features/motion/motion-tokens'
import { AgentApprovalModeMenu } from '@/features/agent-approval/AgentApprovalModeMenu'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { AgentRuntimeQueuedCommand, ContextAnalysis } from '@/lib/api'
import type { AgentTokenUsageRecord } from '@/lib/agent-message-view'
import { EditInteractiveReplyDialog } from '../EditInteractiveReplyDialog'
import type { StoryStageCommandItem } from './story-stage-commands'
import { isNativeComposingKeyboardEvent } from './utils'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { ConversationConfigError } from '@/features/conversation-config/ConversationConfigError'
import { supportsRuntimeOperation } from '@/features/conversation-config/types'
import { ComposerAttachmentTray, useComposerAttachments } from '@/components/Chat/ComposerAttachments'
import type { InputAreaSendOptions } from '@/components/Chat/InputArea'

type StateSetter<T> = Dispatch<SetStateAction<T>>

interface StoryStageComposerProps {
  taskControls?: ReactNode
  layout: {
    projectId: string
    creatingStory: boolean
    isMobile: boolean
    inputTextStyle: CSSProperties
    workspace?: string
    inputFloatRef: RefObject<HTMLDivElement | null>
    inputRef: RefObject<ComposerTokenInputHandle | null>
    t: TFunction
    attachmentDraftKey: string
  }
  editor: {
    input: string
    editingTurn: { id: string; content: string } | null
    styleScenes: string[]
    styleSceneQuery: string | null
    styleSceneSuggestions: string[]
    showSkillCommands: boolean
    activeSkillCommandIndex: number
    skillCommands: Array<{ name: string }>
    filteredSkillCommands: StoryStageCommandItem[]
    filteredBuiltInCommandItems: Array<{ command: StoryStageCommandItem; index: number }>
    filteredSkillCommandItems: Array<{ command: StoryStageCommandItem; index: number }>
    setStyleSceneQuery: StateSetter<string | null>
    setShowSkillCommands: StateSetter<boolean>
    setSkillCommandQuery: StateSetter<string | null>
    setActiveSkillCommandIndex: StateSetter<number>
  }
  story: {
    storyId: string
    branchTerminal: boolean
    hotChoices: string[]
    showHotChoices: boolean
    canUseHotChoices: boolean
  }
  runtime: {
    streaming: boolean
    approvalReady: boolean
    conversationConfig: ConversationConfigController
    abortPending: boolean
    recoveryPaused: boolean
    recoveryAbortAvailable: boolean
    pendingInterruptionId: string
    phase: string
    operationId: string
    connection: string
    commandSubmitting: boolean
    queue: AgentRuntimeQueuedCommand[]
    queueActionPendingCommandID: string
    onRuntimeChanged?: () => void | Promise<void>
  }
  dialogs: {
    contextAnalysisOpen: boolean
    contextAnalysisLoading: boolean
    contextAnalysisError: string | null
    contextAnalysis: ContextAnalysis | null
    tokenUsageOpen: boolean
    tokenUsageMessages: AgentTokenUsageRecord[]
    replyEditTarget: {
      turnId: string
      branchId: string
      initialContent: string
      expectedNarrative: string
    } | null
    setContextAnalysisOpen: StateSetter<boolean>
    setTokenUsageOpen: StateSetter<boolean>
    closeReplyEditor: () => void
    saveReply: (narrative: string) => Promise<void>
  }
  actions: {
    abandonPendingTurn: () => Promise<void>
    cancelEditing: () => void
    selectHotChoice: (choice: string) => void
    selectStyleScene: (scene: string) => void
    selectSkillCommand: (name: string) => void
    handleInputChange: (value: string) => void
    handleInputTriggerChange: (trigger: ComposerTrigger | null) => void
    handleTokenRemove: (token: ComposerTokenSpec) => void
    toggleHotChoices: () => void
    openContextAnalysis: () => void
    removeContextCompaction: () => Promise<void>
    send: (options?: InputAreaSendOptions) => Promise<boolean>
    steerQueuedCommand: (item: AgentRuntimeQueuedCommand) => Promise<boolean>
    deleteQueuedCommand: (item: AgentRuntimeQueuedCommand) => Promise<boolean>
    stop: () => Promise<void>
  }
}

export function StoryStageComposer({ layout, editor, story, runtime, dialogs, actions, taskControls }: StoryStageComposerProps) {
  const [actionsOpen, setActionsOpen] = useState(false)
  const choicesId = useId()
  const reducedMotion = useReducedMotionConfig()
  const choicesTransition = { type: 'tween' as const, duration: reducedMotion ? 0 : 0.2, ease: novaEase }
  const { projectId, creatingStory, isMobile, inputTextStyle, workspace, inputFloatRef, inputRef, t } = layout
  const { input, editingTurn, styleScenes, styleSceneQuery, styleSceneSuggestions, showSkillCommands, activeSkillCommandIndex, skillCommands, filteredSkillCommands, filteredBuiltInCommandItems, filteredSkillCommandItems, setStyleSceneQuery, setShowSkillCommands, setSkillCommandQuery, setActiveSkillCommandIndex } = editor
  const { storyId, branchTerminal, hotChoices, showHotChoices, canUseHotChoices } = story
  const { streaming, approvalReady, conversationConfig, abortPending, recoveryPaused, recoveryAbortAvailable, pendingInterruptionId, operationId, connection, commandSubmitting, queue, queueActionPendingCommandID } = runtime
  const { contextAnalysisOpen, contextAnalysisLoading, contextAnalysisError, contextAnalysis, tokenUsageOpen, tokenUsageMessages, replyEditTarget, setContextAnalysisOpen, setTokenUsageOpen, closeReplyEditor, saveReply } = dialogs
  const { cancelEditing, selectHotChoice, selectStyleScene, selectSkillCommand, handleInputChange, handleInputTriggerChange, handleTokenRemove, toggleHotChoices, openContextAnalysis, removeContextCompaction, send, steerQueuedCommand, deleteQueuedCommand, stop } = actions
  const activeControlsDisabled = streaming && (!operationId || connection !== 'connected')
  const nativeRuntime = !conversationConfig.snapshot?.runtime || conversationConfig.snapshot.runtime.kind === 'native'
  const sendBlocked = streaming && !supportsRuntimeOperation(conversationConfig.snapshot, 'queue')
  const resumeAvailable = Boolean(pendingInterruptionId) && !editingTurn
  const attachments = useComposerAttachments(
    !branchTerminal,
    `game:${layout.attachmentDraftKey}`,
  )
  const stylePickerRef = useRef<FileReferencePickerHandle>(null)
  const builtInCommandOptions: IndexedInputCommandOption[] = filteredBuiltInCommandItems.map(({ command, index }) => ({
    index,
    command: {
      cmd: `/${command.name}`,
      description: command.description || command.name,
      hint: command.hint,
      icon: Archive,
      source: 'builtin',
    },
  }))
  const skillCommandOptions: IndexedInputCommandOption[] = filteredSkillCommandItems.map(({ command, index }) => ({
    index,
    command: {
      cmd: `/${command.name}`,
      description: command.description || command.name,
      hint: command.hint,
      icon: Sparkles,
      source: 'skill',
    },
  }))

  if (creatingStory) return null
  const submit = async () => {
    const files = attachments.files
    attachments.clear()
    const accepted = await send(files.length ? { attachments: files } : undefined)
    if (!accepted) attachments.addFiles(files)
    return accepted
  }
  return (
    <div ref={inputFloatRef} className="nova-story-input-float pointer-events-none absolute inset-x-0 bottom-0 z-20 p-3">
      <div className="nova-story-reading-column pointer-events-auto">
        {taskControls ? <div className="mb-2">{taskControls}</div> : null}
        {editingTurn && !streaming ? (
          <div className="mb-3 flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
            <Pencil className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.editingNotice')}</span>
            <Button type="button" variant="ghost" size="icon-xs" className="h-7 w-7 shrink-0 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]" onClick={cancelEditing} aria-label={t('storyStage.cancelEdit')}>
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        ) : null}
        <div className="relative">
          {/* Keep the popup out of the measured composer height, and animate the
              frosted surface itself so an opacity ancestor cannot cut off its backdrop. */}
          <motion.div
            id={choicesId}
            initial={false}
            animate={{ opacity: showHotChoices ? 1 : 0, y: showHotChoices ? 0 : 8 }}
            transition={choicesTransition}
            aria-hidden={!showHotChoices}
            inert={!showHotChoices}
            className="nova-story-choice-panel absolute inset-x-0 bottom-full mb-2 overflow-hidden border"
          >
            <button type="button" className="nova-nav-item flex min-h-8 w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-left text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]" onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-expanded={showHotChoices}>
              <Compass className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
              <span className="shrink-0 font-medium">{t('storyStage.hotChoices.title')}</span>
              <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{t('storyStage.hotChoices.count', { count: hotChoices.length })}</span>
              <motion.span initial={false} animate={{ rotate: showHotChoices ? 180 : 0 }} transition={choicesTransition} className="flex shrink-0 text-[var(--nova-text-faint)]">
                <ChevronDown className="h-3.5 w-3.5" />
              </motion.span>
            </button>
            <div data-testid="story-stage-hot-choices-list" className="mt-1 flex max-h-48 flex-wrap content-start gap-1.5 overflow-y-auto overscroll-contain">
              {hotChoices.map((choice, index) => (
                <button key={`${index}-${choice}`} type="button" className="min-w-0 max-w-full flex-none rounded-[var(--nova-radius)] border border-transparent bg-[var(--nova-hover)] px-2.5 py-1.5 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-active)] hover:text-[var(--nova-text)]" onMouseDown={(event) => event.preventDefault()} onClick={() => selectHotChoice(choice)}>
                  <span className="block max-w-full break-words">{choice}</span>
                </button>
              ))}
            </div>
          </motion.div>
        </div>
        <AgentQueuedCommandList
          items={queue}
          pendingCommandID={queueActionPendingCommandID}
          disabled={abortPending || commandSubmitting}
          activeOperationID={streaming || recoveryPaused ? operationId : ''}
          steerDisabled={activeControlsDisabled}
          onSteer={steerQueuedCommand}
          onDelete={deleteQueuedCommand}
        />
        <div className="relative min-w-0" {...attachments.dropProps}>
          <FileReferencePicker ref={stylePickerRef} open={styleSceneQuery !== null && styleSceneSuggestions.length > 0} query={styleSceneQuery || ''} items={styleSceneSuggestions} onSelect={(item) => selectStyleScene(item.value)} trigger="#" placeholder={t('chat.styleReference.placeholder')} emptyText={t('chat.styleReference.empty')} heading={t('chat.styleReference.heading')} />
          <InputCommandMenu
            open={showSkillCommands && filteredSkillCommands.length > 0}
            skillsOnly={false}
            builtinCommands={builtInCommandOptions}
            skillCommands={skillCommandOptions}
            activeIndex={activeSkillCommandIndex}
            onActiveIndexChange={setActiveSkillCommandIndex}
            onSelect={(command) => selectSkillCommand(command.cmd.replace(/^\//, ''))}
          />
          <ConversationConfigError controller={conversationConfig} agentKey="interactive_story" />
          <AgentComposerShell
            className="nova-story-stage-composer"
            references={attachments.items.length ? <ComposerAttachmentTray items={attachments.items} onRemove={attachments.remove} /> : undefined}
            input={<ComposerTokenInput
              ref={inputRef}
              value={input}
              onChange={handleInputChange}
              onTriggerChange={handleInputTriggerChange}
              onTokenRemove={handleTokenRemove}
              onEditorKeyDown={(event) => {
                const canPickSkill = showSkillCommands && filteredSkillCommands.length > 0
                if (canPickSkill && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
                  event.preventDefault()
                  setActiveSkillCommandIndex((current) => (current + (event.key === 'ArrowDown' ? 1 : -1) + filteredSkillCommands.length) % filteredSkillCommands.length)
                  return true
                }
                if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && stylePickerRef.current?.moveActive(event.key === 'ArrowDown' ? 1 : -1)) {
                  event.preventDefault()
                  return true
                }
                if (event.key === 'Escape') {
                  setStyleSceneQuery(null); setShowSkillCommands(false); setSkillCommandQuery(null); setActiveSkillCommandIndex(0)
                  return true
                }
                if (event.key === 'Tab' && !event.shiftKey) {
                  if (canPickSkill) {
                    event.preventDefault(); selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                    return true
                  }
                  if (stylePickerRef.current?.selectActive()) {
                    event.preventDefault()
                    return true
                  }
                }
                if (event.key === 'Enter' && !event.shiftKey && (!isMobile || event.metaKey || event.ctrlKey)) {
                  if (isNativeComposingKeyboardEvent(event)) return false
                  event.preventDefault()
                  if (canPickSkill) selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                  else if (!stylePickerRef.current?.selectActive()) void submit()
                  return true
                }
                return false
              }}
              knownSkills={skillCommands.map((skill) => skill.name)}
              knownStyleScenes={Array.from(new Set([...styleSceneSuggestions, ...styleScenes]))}
              externalTokens={styleScenes.map((scene) => ({ kind: 'style', value: scene, label: scene }))}
              rows={1}
              minRows={1}
              maxRows={isMobile ? 5 : 10}
              className="nova-agent-composer-textarea nova-agent-token-input min-h-[42px] resize-none border-0 bg-transparent px-1 py-[9px] text-sm leading-6 text-[var(--nova-text)] shadow-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0"
              style={inputTextStyle}
              disabled={branchTerminal}
              inputMode="text"
              enterKeyHint={isMobile ? 'enter' : 'send'}
              autoCapitalize="sentences"
              placeholder={branchTerminal ? t('storyStage.inputPlaceholderTerminal') : !isMobile && skillCommands.length > 0 ? t('storyStage.inputPlaceholderWithSkills') : t('storyStage.inputPlaceholder')}
            />}
            toolbarStart={<>
              <DropdownMenu open={actionsOpen} onOpenChange={setActionsOpen}>
                <DropdownMenuTrigger asChild><Button type="button" variant="outline" size="icon-sm" className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" disabled={branchTerminal || (!storyId && tokenUsageMessages.length === 0)} aria-label={t('chat.input.actions')}><List className="h-3.5 w-3.5" /></Button></DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="top" className="w-80 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                  <DropdownMenuGroup>
                    <ComposerMenuItem
                      icon={Paperclip}
                      label={t('chat.attachment.add')}
                      disabled={branchTerminal}
                      onSelect={attachments.openPicker}
                    />
                  </DropdownMenuGroup>
                  <AgentApprovalModeMenu runActive={streaming} presentation="submenu" conversationConfig={conversationConfig} />
                  <ImageGenerationSettingsMenu projectId={projectId} disabled={streaming}>{null}</ImageGenerationSettingsMenu>
                  <DropdownMenuGroup>
                    <ComposerMenuItem
                      icon={BarChart3}
                      label={t('chat.tokenUsage.action')}
                      detail={t('chat.tokenUsage.subtitle', { count: tokenUsageMessages.length })}
                      detailTone="faint"
                      onSelect={() => setTokenUsageOpen(true)}
                    />
                    {nativeRuntime && <ComposerMenuItem
                      icon={ScrollText}
                      label={t('chat.contextAnalysis.action')}
                      disabled={!storyId || streaming || branchTerminal}
                      onSelect={openContextAnalysis}
                    />}
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
              {attachments.input}
            </>}
            toolbarEnd={<>
              <ModelProfileSwitcher agentKey="interactive_story" workspace={workspace} conversationConfig={conversationConfig} disabled={branchTerminal} runActive={streaming} onRuntimeChanged={runtime.onRuntimeChanged} />
              {pendingInterruptionId && runtime.phase === 'idle' && !streaming && (
                <Button type="button" size="sm" variant="ghost" disabled={commandSubmitting || abortPending} title={t('storyStage.interrupted.abandonHelp')} onClick={() => { void actions.abandonPendingTurn() }}>
                  {t('storyStage.interrupted.abandon')}
                </Button>
              )}
              <Button type="button" variant="outline" className={`nova-agent-composer-pill h-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${showHotChoices ? 'text-[var(--nova-text)]' : ''}`} disabled={!canUseHotChoices} onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-controls={choicesId} aria-expanded={showHotChoices} aria-pressed={showHotChoices} aria-label={showHotChoices ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')}><Compass className="h-3.5 w-3.5" />{t('storyStage.hotChoices.button')}</Button>
            </>}
            submitControl={<AgentComposerControls generationActive={streaming} hasSendableContent={Boolean(input.trim() || attachments.files.length)} resumeAvailable={resumeAvailable} onStop={() => { void stop() }} onSend={() => { void submit() }} sendDisabled={sendBlocked || !approvalReady || !storyId || (!input.trim() && attachments.files.length === 0 && !resumeAvailable)} disabled={branchTerminal} abortPending={abortPending} actionPending={commandSubmitting} activeControlsDisabled={activeControlsDisabled} stopDisabled={streaming && !recoveryAbortAvailable && (recoveryPaused || !operationId || connection !== 'connected')} sendLabel={editingTurn ? t('storyStage.sendRegenerate') : undefined} sendIcon={editingTurn ? <RefreshCw data-icon="inline-start" /> : undefined} />}
          />
        </div>
        <ContextAnalysisDialog open={contextAnalysisOpen} loading={contextAnalysisLoading} error={contextAnalysisError} analysis={contextAnalysis} onOpenChange={setContextAnalysisOpen} onRemoveCompaction={removeContextCompaction} />
        <TokenUsageDialog projectId={projectId} open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} />
        {replyEditTarget ? <EditInteractiveReplyDialog key={replyEditTarget.turnId} turnId={replyEditTarget.turnId} initialContent={replyEditTarget.initialContent} onClose={closeReplyEditor} onSave={saveReply} /> : null}
      </div>
    </div>
  )
}
