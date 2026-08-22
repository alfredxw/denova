import { useRef, type CSSProperties, type Dispatch, type RefObject, type SetStateAction } from 'react'
import type { TFunction } from 'i18next'
import { Activity, Archive, BarChart3, ChevronDown, ChevronUp, Compass, List, Loader2, Paperclip, Pencil, Plus, RefreshCw, ScrollText, Sparkles, Target, X } from 'lucide-react'
import { AgentComposerControls } from '@/components/Chat/AgentComposerControls'
import { AgentComposerShell } from '@/components/Chat/AgentComposerShell'
import { AgentGoalCard } from '@/components/Chat/AgentGoalCard'
import { AgentQueuedCommandList } from '@/components/Chat/AgentQueuedCommandList'
import { ComposerModeChip } from '@/components/Chat/ComposerModeChip'
import { ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import { FileReferencePicker, type FileReferencePickerHandle } from '@/components/Chat/FileReferencePicker'
import { InputCommandMenu, type IndexedInputCommandOption } from '@/components/Chat/InputCommandMenu'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { ImageGenerationSettingsMenu } from '@/components/Chat/ImageGenerationSettingsMenu'
import { TokenUsageDialog } from '@/components/Chat/TokenUsagePanel'
import { AgentTracePanel } from '@/components/Chat/AgentTracePanel'
import { ComposerTokenInput, type ComposerTokenInputHandle, type ComposerTokenSpec, type ComposerTrigger } from '@/components/Chat/composer-token-input'
import { Button } from '@/components/ui/button'
import { AgentApprovalModeMenu } from '@/features/agent-approval/AgentApprovalModeMenu'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { AgentRuntimeQueuedCommand, ContextAnalysis } from '@/lib/api'
import type { AgentTokenUsageRecord } from '@/lib/agent-message-view'
import type { DirectorPlanStatus, ImagePreset, StoryImageSettings, StorySummary } from '../../types'
import { EditInteractiveReplyDialog } from '../EditInteractiveReplyDialog'
import { InteractiveImageSettingsMenu, StoryImagePresetMenu } from './ImageSettingsMenus'
import type { StoryStageCommandItem } from './story-stage-commands'
import { isNativeComposingKeyboardEvent } from './utils'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import type { ConversationGoal } from '@/features/agent-goal/types'
import { ComposerAttachmentTray, useComposerAttachments } from '@/components/Chat/ComposerAttachments'
import type { InputAreaSendOptions } from '@/components/Chat/InputArea'

type StateSetter<T> = Dispatch<SetStateAction<T>>

interface StoryStageComposerProps {
  layout: {
    projectId: string
    creatingStory: boolean
    isMobile: boolean
    keyboardInset: number
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
    story?: StorySummary
    imagePresets: ImagePreset[]
    onImageSettingsChange?: (settings: StoryImageSettings) => void | Promise<void>
    branchTerminal: boolean
    directorBlocking: boolean
    directorPlanStatus?: DirectorPlanStatus
    directorStatusVisible: boolean
    directorRetrying: boolean
    directorRetryError: string
    hotChoices: string[]
    hotChoicesExpanded: boolean
    showHotChoices: boolean
    canUseHotChoices: boolean
    setHotChoicesExpanded: StateSetter<boolean>
  }
  runtime: {
    streaming: boolean
    approvalReady: boolean
    conversationConfig: ConversationConfigController
    abortPending: boolean
    recoveryPaused: boolean
    recoveryAbortAvailable: boolean
    operationId: string
    connection: string
    commandSubmitting: boolean
    queue: AgentRuntimeQueuedCommand[]
    queueActionPendingCommandID: string
  }
  goal: {
    value: ConversationGoal | null
    mode: boolean
    pending: boolean
    enter: () => void
    exit: () => void
    edit: () => void
    pause: () => void | Promise<void>
    clear: () => void | Promise<void>
  }
  dialogs: {
    contextAnalysisOpen: boolean
    contextAnalysisLoading: boolean
    contextAnalysisError: string | null
    contextAnalysis: ContextAnalysis | null
    tokenUsageOpen: boolean
    tokenUsageMessages: AgentTokenUsageRecord[]
    traceOpen: boolean
    selectedTraceRunId: string
    replyEditTarget: {
      turnId: string
      branchId: string
      initialContent: string
      expectedNarrative: string
    } | null
    setContextAnalysisOpen: StateSetter<boolean>
    setTokenUsageOpen: StateSetter<boolean>
    setTraceOpen: StateSetter<boolean>
    closeReplyEditor: () => void
    saveReply: (narrative: string) => Promise<void>
  }
  actions: {
    cancelEditing: () => void
    retryDirectorPlanning: () => Promise<void>
    selectHotChoice: (choice: string) => void
    selectStyleScene: (scene: string) => void
    selectSkillCommand: (name: string) => void
    handleInputChange: (value: string) => void
    handleInputTriggerChange: (trigger: ComposerTrigger | null) => void
    handleTokenRemove: (token: ComposerTokenSpec) => void
    toggleHotChoices: () => void
    openMobileNavigation: () => void
    openContextAnalysis: () => void
    removeContextCompaction: () => Promise<void>
    openTraceRun: (runId: string) => void
    send: (options?: InputAreaSendOptions) => Promise<boolean>
    steerQueuedCommand: (item: AgentRuntimeQueuedCommand) => Promise<boolean>
    deleteQueuedCommand: (item: AgentRuntimeQueuedCommand) => Promise<boolean>
    stop: () => Promise<void>
  }
}

export function StoryStageComposer({ layout, editor, story, runtime, goal, dialogs, actions }: StoryStageComposerProps) {
  const { projectId, creatingStory, isMobile, keyboardInset, inputTextStyle, workspace, inputFloatRef, inputRef, t } = layout
  const { input, editingTurn, styleScenes, styleSceneQuery, styleSceneSuggestions, showSkillCommands, activeSkillCommandIndex, skillCommands, filteredSkillCommands, filteredBuiltInCommandItems, filteredSkillCommandItems, setStyleSceneQuery, setShowSkillCommands, setSkillCommandQuery, setActiveSkillCommandIndex } = editor
  const { storyId, story: currentStory, imagePresets, onImageSettingsChange, branchTerminal, directorBlocking, directorPlanStatus, directorStatusVisible, directorRetrying, directorRetryError, hotChoices, hotChoicesExpanded, showHotChoices, canUseHotChoices, setHotChoicesExpanded } = story
  const { streaming, approvalReady, conversationConfig, abortPending, recoveryPaused, recoveryAbortAvailable, operationId, connection, commandSubmitting, queue, queueActionPendingCommandID } = runtime
  const { contextAnalysisOpen, contextAnalysisLoading, contextAnalysisError, contextAnalysis, tokenUsageOpen, tokenUsageMessages, traceOpen, selectedTraceRunId, replyEditTarget, setContextAnalysisOpen, setTokenUsageOpen, setTraceOpen, closeReplyEditor, saveReply } = dialogs
  const { cancelEditing, retryDirectorPlanning, selectHotChoice, selectStyleScene, selectSkillCommand, handleInputChange, handleInputTriggerChange, handleTokenRemove, toggleHotChoices, openMobileNavigation, openContextAnalysis, removeContextCompaction, openTraceRun, send, steerQueuedCommand, deleteQueuedCommand, stop } = actions
  const activeControlsDisabled = streaming && (!operationId || connection !== 'connected')
  const attachments = useComposerAttachments(
    !branchTerminal && !directorBlocking && approvalReady && !goal.pending,
    `game:${layout.attachmentDraftKey}`,
  )
  const stylePickerRef = useRef<FileReferencePickerHandle>(null)
  const builtInCommandOptions: IndexedInputCommandOption[] = filteredBuiltInCommandItems.map(({ command, index }) => ({
    index,
    command: {
      cmd: `/${command.name}`,
      description: command.description || command.name,
      hint: command.hint,
      icon: command.name === 'goal' ? Target : Archive,
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
    <div ref={inputFloatRef} style={{ bottom: keyboardInset }} className="nova-story-input-float pointer-events-none absolute inset-x-0 bottom-0 z-20 p-3">
      <div className="pointer-events-auto mx-auto max-w-5xl">
        {editingTurn && !streaming ? (
          <div className="mb-3 flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
            <Pencil className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.editingNotice')}</span>
            <Button type="button" variant="ghost" size="icon-xs" className="h-7 w-7 shrink-0 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]" onClick={cancelEditing} aria-label={t('storyStage.cancelEdit')}>
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        ) : null}
        {directorStatusVisible && directorPlanStatus ? (
          <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]/90 px-3 py-2 text-xs text-[var(--nova-text-muted)] shadow-[var(--nova-shadow)] backdrop-blur-xl">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              {directorPlanStatus.status === 'running' ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-[var(--nova-text-faint)]" /> : <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
              <span className="min-w-0 flex-1 font-medium text-[var(--nova-text)]">{t('storyStage.director.title')}</span>
              <span className="shrink-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-faint)]">
                {t('storyStage.director.progress', { completed: directorPlanStatus.completed_docs, planned: directorPlanStatus.planned_docs })}
              </span>
              {directorPlanStatus.status === 'failed' ? (
                <Button type="button" variant="outline" size="xs" className="h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)]" disabled={directorRetrying || !storyId} onClick={() => void retryDirectorPlanning()}>
                  {directorRetrying ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
                  {t('storyStage.director.retry')}
                </Button>
              ) : null}
            </div>
            <div className="mt-1 leading-5 text-[var(--nova-text-faint)]">
              {directorRetryError || directorPlanStatus.error || directorPlanStatus.summary || t('storyStage.director.description')}
            </div>
          </div>
        ) : null}
        {showHotChoices ? (
          <div className="mb-2 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <div className="flex min-h-8 items-center gap-1.5 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
              <button type="button" className="nova-nav-item flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]" onMouseDown={(event) => event.preventDefault()} onClick={() => setHotChoicesExpanded((value) => !value)} aria-expanded={hotChoicesExpanded}>
                <Compass className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                <span className="shrink-0 font-medium text-[var(--nova-text-muted)]">{t('storyStage.hotChoices.title')}</span>
                <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{t('storyStage.hotChoices.count', { count: hotChoices.length })}</span>
                {hotChoicesExpanded ? <ChevronUp className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
              </button>
            </div>
            {hotChoicesExpanded ? (
              <div className="border-t border-[var(--nova-border)] px-2 py-2">
                <div data-testid="story-stage-hot-choices-list" className="flex max-h-48 flex-wrap content-start gap-1.5 overflow-y-auto overscroll-contain pr-1">
                  {hotChoices.map((choice, index) => (
                    <button key={`${index}-${choice}`} type="button" className="min-w-0 max-w-full flex-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onMouseDown={(event) => event.preventDefault()} onClick={() => selectHotChoice(choice)}>
                      <span className="block max-w-full break-words">{choice}</span>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
        <AgentQueuedCommandList
          items={queue}
          pendingCommandID={queueActionPendingCommandID}
          disabled={activeControlsDisabled || abortPending || commandSubmitting}
          onSteer={steerQueuedCommand}
          onDelete={deleteQueuedCommand}
        />
        {goal.value ? (
          <AgentGoalCard
            goal={goal.value}
            pending={goal.pending}
            disabled={branchTerminal || directorBlocking || !approvalReady}
            onEdit={goal.edit}
            onPause={goal.pause}
            onClear={goal.clear}
          />
        ) : null}
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
                if (event.key === 'Enter' && !event.shiftKey) {
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
              disabled={branchTerminal || directorBlocking || !approvalReady || goal.pending}
              inputMode="text"
              enterKeyHint="send"
              autoCapitalize="sentences"
              placeholder={branchTerminal ? t('storyStage.inputPlaceholderTerminal') : directorBlocking ? t('storyStage.director.inputBlocked') : goal.mode ? t('chat.goal.placeholder') : !isMobile && skillCommands.length > 0 ? t('storyStage.inputPlaceholderWithSkills') : t('storyStage.inputPlaceholder')}
            />}
            toolbarStart={<>
              <DropdownMenu>
                <DropdownMenuTrigger asChild><Button type="button" variant="outline" size="icon-sm" className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" disabled={branchTerminal || directorBlocking || (!storyId && tokenUsageMessages.length === 0)} aria-label={t('chat.input.actions')}><List className="h-3.5 w-3.5" /></Button></DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="top" className="w-80 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                  <DropdownMenuItem disabled={branchTerminal || directorBlocking || goal.pending} onSelect={attachments.openPicker} className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"><Paperclip className="h-3.5 w-3.5" />{t('chat.attachment.add')}</DropdownMenuItem>
                  <DropdownMenuItem disabled={streaming || branchTerminal || directorBlocking || goal.pending} onSelect={goal.mode ? goal.exit : goal.enter} className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"><Target className="h-3.5 w-3.5" />{t(goal.mode ? 'chat.goal.exitMode' : 'chat.goal.enterMode')}</DropdownMenuItem>
                  <AgentApprovalModeMenu runActive={streaming} presentation="submenu" conversationConfig={conversationConfig} />
                  <ImageGenerationSettingsMenu projectId={projectId} disabled={streaming || directorBlocking}>
                    <StoryImagePresetMenu story={currentStory} presets={imagePresets} disabled={!storyId || streaming || directorBlocking || !onImageSettingsChange} onChange={onImageSettingsChange} />
                  </ImageGenerationSettingsMenu>
                  <InteractiveImageSettingsMenu story={currentStory} disabled={!storyId || streaming || directorBlocking || !onImageSettingsChange} onChange={onImageSettingsChange} />
                  <DropdownMenuItem onSelect={() => setTokenUsageOpen(true)} className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"><BarChart3 className="h-3.5 w-3.5" /><span className="min-w-0 flex-1">{t('chat.tokenUsage.action')}</span><span className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.subtitle', { count: tokenUsageMessages.length })}</span></DropdownMenuItem>
                  <DropdownMenuItem disabled={!storyId || streaming || branchTerminal || directorBlocking} onSelect={openContextAnalysis} className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"><ScrollText className="h-3.5 w-3.5" />{t('chat.contextAnalysis.action')}</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              {goal.mode ? <ComposerModeChip icon={Target} label={t('chat.goal.short')} ariaLabel={t('chat.goal.exitMode')} disabled={goal.pending} onClose={goal.exit} /> : null}
              {attachments.input}
            </>}
            toolbarEnd={<>
              <ModelProfileSwitcher agentKey="interactive_story" workspace={workspace} conversationConfig={conversationConfig} disabled={streaming || directorBlocking || !approvalReady} />
              <Button type="button" variant="outline" className={`nova-agent-composer-pill h-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${hotChoicesExpanded ? 'text-[var(--nova-text)]' : ''}`} disabled={!canUseHotChoices} onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-label={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')}><Compass className="h-3.5 w-3.5" />{!isMobile ? t('storyStage.hotChoices.button') : null}</Button>
              {isMobile ? <Button type="button" variant="outline" className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-0 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onMouseDown={(event) => event.preventDefault()} onClick={openMobileNavigation} aria-label={t('workbench.mobile.navigationMenu')}><Plus className="h-3.5 w-3.5" /></Button> : null}
            </>}
            submitControl={<AgentComposerControls generationActive={streaming} hasSendableContent={Boolean(input.trim() || attachments.files.length)} onStop={() => { void stop() }} onSend={() => { void submit() }} sendDisabled={!approvalReady || !storyId || (!input.trim() && attachments.files.length === 0) || goal.pending} disabled={branchTerminal || directorBlocking} abortPending={abortPending} actionPending={commandSubmitting || goal.pending} activeControlsDisabled={activeControlsDisabled} stopDisabled={streaming && !recoveryAbortAvailable && (recoveryPaused || !operationId || connection !== 'connected')} sendLabel={editingTurn ? t('storyStage.sendRegenerate') : undefined} sendIcon={editingTurn ? <RefreshCw /> : undefined} />}
          />
        </div>
        <ContextAnalysisDialog open={contextAnalysisOpen} loading={contextAnalysisLoading} error={contextAnalysisError} analysis={contextAnalysis} onOpenChange={setContextAnalysisOpen} onRemoveCompaction={removeContextCompaction} />
        <TokenUsageDialog open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} onOpenTrace={openTraceRun} />
        <Dialog open={traceOpen} onOpenChange={setTraceOpen}>
          <DialogContent className="flex h-[min(88vh,760px)] max-w-[min(96vw,1120px)] flex-col gap-0 overflow-hidden border-[var(--nova-border)] bg-[var(--nova-bg)] p-0 text-[var(--nova-text)]">
            <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3"><DialogTitle className="flex items-center gap-2 text-sm"><Activity className="h-4 w-4 text-[var(--nova-text-muted)]" />{t('chat.tracePanel.title')}</DialogTitle></DialogHeader>
            <div className="min-h-0 flex-1"><AgentTracePanel projectId={projectId} selectedRunId={selectedTraceRunId} /></div>
          </DialogContent>
        </Dialog>
        {replyEditTarget ? <EditInteractiveReplyDialog key={replyEditTarget.turnId} turnId={replyEditTarget.turnId} initialContent={replyEditTarget.initialContent} onClose={closeReplyEditor} onSave={saveReply} /> : null}
      </div>
    </div>
  )
}
