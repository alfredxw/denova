import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import {
  ArrowDownToLine,
  Check,
  CornerDownLeft,
  MessageSquareShare,
  RefreshCw,
  Settings,
  Sparkles,
  Square,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { requestSettingsSection } from '@/features/onboarding/events'
import { useIsMobile } from '@/hooks/useIsMobile'
import { APIError } from '@/lib/api-client/client'
import {
  streamInlineEdit,
  type InlineEditMeta,
  type InlineEditMode,
} from '@/lib/api-client/inline-edit'
import { cn } from '@/lib/utils'
import { computeInlineEditDiff } from './inlineEditDiff'
import { inlineEditPresetsFor } from './inlineEditPresets'
import type { InlineEditApplyAction } from './applyInlineEditToEditor'

export interface InlineEditAnchor {
  top: number
  bottom: number
  left: number
}

export interface InlineEditPopoverProps {
  projectId: string
  mode: InlineEditMode
  selection: string
  before: string
  after: string
  matchedLoreIds: readonly string[]
  storyId?: string
  layout?: 'floating' | 'embedded'
  anchor?: InlineEditAnchor | null
  containerRef?: React.RefObject<HTMLElement | null>
  onApply: (text: string, action: InlineEditApplyAction) => void
  onClose: () => void
  onHandOffToChat?: (instruction: string) => void
}

type InlineEditPhase = 'idle' | 'streaming' | 'preview' | 'error'
type PreviewDisplayMode = 'diff' | 'result'

/**
 * Unified inline AI rewrite/insertion popover shared across Writing, Lore,
 * and Game turn revision editors.
 */
export function InlineEditPopover({
  projectId,
  mode,
  selection,
  before,
  after,
  matchedLoreIds,
  storyId,
  layout = 'floating',
  anchor = null,
  containerRef,
  onApply,
  onClose,
  onHandOffToChat,
}: InlineEditPopoverProps) {
  const { t, i18n } = useTranslation()
  const isMobile = useIsMobile()
  const hasSelection = selection.length > 0
  const presets = useMemo(() => inlineEditPresetsFor(mode, hasSelection), [mode, hasSelection])
  const supportsStyle = mode !== 'lore'

  const [phase, setPhase] = useState<InlineEditPhase>('idle')
  const [instruction, setInstruction] = useState('')
  const [lastInstruction, setLastInstruction] = useState('')
  const [refinement, setRefinement] = useState('')
  const [output, setOutput] = useState('')
  const [previewMode, setPreviewMode] = useState<PreviewDisplayMode>(hasSelection ? 'diff' : 'result')
  const [includeStyle, setIncludeStyle] = useState(supportsStyle)
  const [includeLore, setIncludeLore] = useState(matchedLoreIds.length > 0)
  const [meta, setMeta] = useState<InlineEditMeta | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const [errorCode, setErrorCode] = useState<string | undefined>(undefined)

  const abortRef = useRef<AbortController | null>(null)
  const popoverRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const refineInputRef = useRef<HTMLInputElement>(null)
  const previewScrollRef = useRef<HTMLDivElement>(null)

  const abortActiveStream = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort()
      abortRef.current = null
    }
  }, [])

  useEffect(() => {
    return () => abortActiveStream()
  }, [abortActiveStream])

  useEffect(() => {
    if (phase === 'idle') {
      inputRef.current?.focus({ preventScroll: true })
    }
  }, [phase])

  useEffect(() => {
    if (phase === 'streaming' && previewScrollRef.current) {
      previewScrollRef.current.scrollTop = previewScrollRef.current.scrollHeight
    }
  }, [output, phase])

  const runEdit = useCallback(async (promptText: string, previousResult = '') => {
    const trimmed = promptText.trim()
    if (!trimmed) return

    abortActiveStream()
    const controller = new AbortController()
    abortRef.current = controller

    setLastInstruction(trimmed)
    setErrorMessage('')
    setErrorCode(undefined)
    setOutput('')
    setPhase('streaming')
    setPreviewMode(hasSelection ? 'diff' : 'result')

    try {
      const finalText = await streamInlineEdit(
        {
          projectId,
          mode,
          instruction: trimmed,
          selection,
          before,
          after,
          previous_result: previousResult || undefined,
          lore_ids: includeLore && matchedLoreIds.length > 0 ? [...matchedLoreIds] : undefined,
          include_lore: includeLore && matchedLoreIds.length > 0,
          include_style: supportsStyle && includeStyle,
          story_id: storyId || undefined,
        },
        {
          onMeta: (nextMeta) => {
            if (controller.signal.aborted) return
            setMeta(nextMeta)
          },
          onDelta: (delta) => {
            if (controller.signal.aborted) return
            setOutput((prev) => prev + delta)
          },
        },
        controller.signal,
      )
      if (controller.signal.aborted) return
      setOutput(finalText)
      setRefinement('')
      setPhase('preview')
    } catch (error) {
      if (controller.signal.aborted) return
      if (error instanceof DOMException && error.name === 'AbortError') return
      if (error instanceof APIError) {
        setErrorMessage(error.summary || error.message)
        setErrorCode(error.code)
      } else {
        setErrorMessage(error instanceof Error ? error.message : t('editor.inlineEdit.failed'))
        setErrorCode(undefined)
      }
      setPhase('error')
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null
      }
    }
  }, [
    abortActiveStream,
    after,
    before,
    hasSelection,
    includeLore,
    includeStyle,
    matchedLoreIds,
    mode,
    projectId,
    selection,
    storyId,
    supportsStyle,
    t,
  ])

  const handleClose = useCallback(() => {
    abortActiveStream()
    onClose()
  }, [abortActiveStream, onClose])

  const handleStop = useCallback(() => {
    abortActiveStream()
    if (output.trim()) {
      setPhase('preview')
    } else {
      setPhase('idle')
    }
  }, [abortActiveStream, output])

  const handleAccept = useCallback((action: InlineEditApplyAction) => {
    if (!output.trim()) return
    abortActiveStream()
    onApply(output, action)
  }, [abortActiveStream, onApply, output])

  const handleContainerKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      if (phase === 'streaming') {
        handleStop()
      } else {
        handleClose()
      }
      return
    }
    if (phase === 'preview' && output.trim()) {
      const target = event.target as HTMLElement | null
      const inRefineInput = target === refineInputRef.current
      if (event.key === 'Tab' && !event.shiftKey && !inRefineInput) {
        event.preventDefault()
        event.stopPropagation()
        handleAccept('replace')
        return
      }
      if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        event.stopPropagation()
        handleAccept('replace')
      }
    }
  }, [handleAccept, handleClose, handleStop, output, phase])

  const diffParts = useMemo(() => {
    if (!hasSelection || previewMode !== 'diff') return []
    return computeInlineEditDiff(selection, output, i18n.language)
  }, [hasSelection, i18n.language, output, previewMode, selection])

  const floatingStyle = useMemo<React.CSSProperties | undefined>(() => {
    if (layout !== 'floating') return undefined
    if (isMobile) {
      return {
        position: 'sticky',
        bottom: 8,
        left: 0,
        right: 0,
        marginInline: 'auto',
      }
    }
    if (!anchor) return undefined
    const containerEl = containerRef?.current
    const containerWidth = containerEl?.clientWidth ?? 680
    const containerHeight = containerEl?.clientHeight ?? 520
    const scrollTop = containerEl?.scrollTop ?? 0
    const cardWidth = Math.min(480, Math.max(280, containerWidth - 24))
    const left = Math.max(12, Math.min(anchor.left, containerWidth - cardWidth - 12))
    const estimatedHeight = phase === 'idle' ? 148 : 300
    const visibleBottom = scrollTop + containerHeight
    const placeAbove = anchor.bottom + 8 + estimatedHeight > visibleBottom
      && anchor.top - 8 - estimatedHeight >= scrollTop
    const top = placeAbove
      ? Math.max(scrollTop + 8, anchor.top - estimatedHeight - 8)
      : anchor.bottom + 8
    return {
      position: 'absolute',
      top,
      left,
      width: cardWidth,
    }
  }, [anchor, containerRef, isMobile, layout, phase])

  const isModelRequiredError = errorCode === 'api.inlineEdit.modelRequired'
  const showLorePill = matchedLoreIds.length > 0

  return (
    <div
      ref={popoverRef}
      role="dialog"
      aria-label={t(hasSelection ? 'editor.inlineEdit.titleRewrite' : 'editor.inlineEdit.titleInsert')}
      data-testid="inline-edit-popover"
      style={floatingStyle}
      onKeyDown={handleContainerKeyDown}
      onPointerDown={(event) => event.stopPropagation()}
      className={cn(
        'z-40 flex flex-col overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] text-[var(--nova-text)] shadow-2xl backdrop-blur',
        layout === 'floating' && isMobile && 'w-[calc(100%-8px)]',
        layout === 'embedded' && 'w-full shadow-sm',
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border-soft)] px-3 py-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <Sparkles className="h-3.5 w-3.5 shrink-0 text-amber-500 dark:text-amber-400" />
          <span className="truncate text-xs font-medium">
            {t(hasSelection ? 'editor.inlineEdit.titleRewrite' : 'editor.inlineEdit.titleInsert')}
          </span>
          {supportsStyle ? (
            <button
              type="button"
              disabled={phase === 'streaming'}
              onClick={() => setIncludeStyle((prev) => !prev)}
              aria-pressed={includeStyle}
              className={cn(
                'inline-flex h-5 items-center rounded-full border px-2 text-[10px] transition-colors disabled:opacity-50',
                includeStyle
                  ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                  : 'border-transparent bg-transparent text-[var(--nova-text-faint)] line-through hover:bg-[var(--nova-hover)]',
              )}
              title={t('editor.inlineEdit.toggleStyle')}
            >
              {meta?.style_name && includeStyle ? meta.style_name : t('editor.inlineEdit.styleTag')}
            </button>
          ) : null}
          {showLorePill ? (
            <button
              type="button"
              disabled={phase === 'streaming'}
              onClick={() => setIncludeLore((prev) => !prev)}
              aria-pressed={includeLore}
              className={cn(
                'inline-flex h-5 items-center rounded-full border px-2 text-[10px] transition-colors disabled:opacity-50',
                includeLore
                  ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                  : 'border-transparent bg-transparent text-[var(--nova-text-faint)] line-through hover:bg-[var(--nova-hover)]',
              )}
              title={t('editor.inlineEdit.toggleLore')}
            >
              {t('editor.inlineEdit.loreTag', { count: matchedLoreIds.length })}
            </button>
          ) : null}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={handleClose}
          aria-label={t('common.close')}
          className="text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* Prompt + Presets (idle or error) */}
      {(phase === 'idle' || phase === 'error') ? (
        <div className="flex flex-col gap-2 p-2.5">
          <form
            className="flex items-center gap-1.5"
            onSubmit={(event) => {
              event.preventDefault()
              void runEdit(instruction)
            }}
          >
            <input
              ref={inputRef}
              type="text"
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              placeholder={t(hasSelection ? 'editor.inlineEdit.placeholderRewrite' : 'editor.inlineEdit.placeholderInsert')}
              aria-label={t('editor.inlineEdit.instructionLabel')}
              className="h-8 min-w-0 flex-1 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 text-xs text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-text-muted)] focus:outline-none"
            />
            <Button
              type="submit"
              size="sm"
              disabled={!instruction.trim()}
              className="h-8 shrink-0 gap-1 px-2.5 text-xs"
            >
              <CornerDownLeft className="h-3.5 w-3.5" />
              {t('editor.inlineEdit.run')}
            </Button>
          </form>

          <div className="flex flex-wrap items-center gap-1">
            {presets.map((preset) => (
              <button
                key={preset.id}
                type="button"
                onClick={() => {
                  const promptText = t(preset.instructionKey)
                  setInstruction(promptText)
                  void runEdit(promptText)
                }}
                className="rounded-md border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)] transition-colors hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              >
                {t(preset.labelKey)}
              </button>
            ))}
          </div>

          {phase === 'error' && errorMessage ? (
            <div
              role="alert"
              className="flex flex-col gap-2 rounded-lg border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2.5 py-2 text-xs text-[var(--nova-danger)]"
            >
              <span>{errorMessage}</span>
              <div className="flex flex-wrap items-center gap-1.5">
                {isModelRequiredError ? (
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    onClick={() => {
                      handleClose()
                      requestSettingsSection('model')
                    }}
                    className="h-6 gap-1 text-[11px]"
                  >
                    <Settings className="h-3 w-3" />
                    {t('editor.inlineEdit.openModelSettings')}
                  </Button>
                ) : null}
                {onHandOffToChat ? (
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    onClick={() => {
                      const handoff = instruction.trim() || lastInstruction
                      handleClose()
                      onHandOffToChat(handoff)
                    }}
                    className="h-6 gap-1 text-[11px]"
                  >
                    <MessageSquareShare className="h-3 w-3" />
                    {t('editor.inlineEdit.handOffToChat')}
                  </Button>
                ) : null}
                {!isModelRequiredError && lastInstruction ? (
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    onClick={() => void runEdit(lastInstruction)}
                    className="h-6 gap-1 text-[11px]"
                  >
                    <RefreshCw className="h-3 w-3" />
                    {t('common.retry')}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
      ) : null}

      {/* Streaming / Preview Diff */}
      {(phase === 'streaming' || phase === 'preview') ? (
        <div className="flex flex-col">
          <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-3 py-1">
            <span className="truncate text-[11px] text-[var(--nova-text-faint)]">
              {phase === 'streaming' ? t('editor.inlineEdit.generating') : lastInstruction}
            </span>
            {hasSelection ? (
              <div
                role="group"
                aria-label={t('editor.inlineEdit.viewToggle')}
                className="inline-flex shrink-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0.5 text-[10px]"
              >
                <button
                  type="button"
                  aria-pressed={previewMode === 'diff'}
                  onClick={() => setPreviewMode('diff')}
                  className={cn(
                    'rounded px-1.5 py-0.5 transition-colors',
                    previewMode === 'diff'
                      ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                      : 'text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]',
                  )}
                >
                  {t('editor.inlineEdit.viewDiff')}
                </button>
                <button
                  type="button"
                  aria-pressed={previewMode === 'result'}
                  onClick={() => setPreviewMode('result')}
                  className={cn(
                    'rounded px-1.5 py-0.5 transition-colors',
                    previewMode === 'result'
                      ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                      : 'text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]',
                  )}
                >
                  {t('editor.inlineEdit.viewResult')}
                </button>
              </div>
            ) : null}
          </div>

          <div
            ref={previewScrollRef}
            data-testid="inline-edit-preview"
            className="max-h-56 overflow-y-auto px-3 py-2.5 text-sm leading-relaxed whitespace-pre-wrap break-words"
          >
            {output ? (
              hasSelection && previewMode === 'diff' ? (
                <>
                  {diffParts.map((part, index) => {
                    if (part.kind === 'removed') {
                      return (
                        <del key={index} className="nova-inline-edit-del">
                          {part.value}
                        </del>
                      )
                    }
                    if (part.kind === 'added') {
                      return (
                        <ins key={index} className="nova-inline-edit-add">
                          {part.value}
                        </ins>
                      )
                    }
                    return <span key={index}>{part.value}</span>
                  })}
                </>
              ) : (
                <span>{output}</span>
              )
            ) : (
              <span className="text-xs text-[var(--nova-text-faint)]">
                {t('editor.inlineEdit.generating')}
              </span>
            )}
            {phase === 'streaming' ? (
              <span className="ml-0.5 inline-block h-3.5 w-1.5 animate-pulse rounded-xs bg-[var(--nova-text-muted)] align-middle" />
            ) : null}
          </div>

          {/* Actions */}
          <div className="flex flex-col gap-2 border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-3 py-2">
            {phase === 'streaming' ? (
              <div className="flex items-center justify-end gap-1.5">
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onClick={handleStop}
                  className="h-7 gap-1 text-xs"
                >
                  <Square className="h-3 w-3" />
                  {t('editor.inlineEdit.stop')}
                </Button>
              </div>
            ) : (
              <>
                <div className="flex flex-wrap items-center justify-between gap-1.5">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Button
                      type="button"
                      size="xs"
                      disabled={!output.trim()}
                      onClick={() => handleAccept('replace')}
                      className="h-7 gap-1 px-2.5 text-xs"
                    >
                      <Check className="h-3.5 w-3.5" />
                      {t(hasSelection ? 'editor.inlineEdit.acceptReplace' : 'editor.inlineEdit.acceptInsert')}
                    </Button>
                    {hasSelection ? (
                      <Button
                        type="button"
                        size="xs"
                        variant="outline"
                        disabled={!output.trim()}
                        onClick={() => handleAccept('insert_below')}
                        className="h-7 gap-1 px-2 text-xs"
                      >
                        <ArrowDownToLine className="h-3.5 w-3.5" />
                        {t('editor.inlineEdit.insertBelow')}
                      </Button>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      type="button"
                      size="xs"
                      variant="ghost"
                      onClick={() => void runEdit(lastInstruction)}
                      className="h-7 gap-1 px-2 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
                    >
                      <RefreshCw className="h-3 w-3" />
                      {t('editor.inlineEdit.regenerate')}
                    </Button>
                    {onHandOffToChat ? (
                      <Button
                        type="button"
                        size="xs"
                        variant="ghost"
                        onClick={() => {
                          handleClose()
                          onHandOffToChat(lastInstruction)
                        }}
                        className="h-7 gap-1 px-2 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
                      >
                        <MessageSquareShare className="h-3 w-3" />
                        {t('editor.inlineEdit.handOffToChat')}
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      size="xs"
                      variant="ghost"
                      onClick={handleClose}
                      className="h-7 px-2 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
                    >
                      {t('editor.inlineEdit.discard')}
                    </Button>
                  </div>
                </div>

                {/* Follow-up refinement */}
                <form
                  className="flex items-center gap-1.5"
                  onSubmit={(event) => {
                    event.preventDefault()
                    if (!refinement.trim()) return
                    void runEdit(refinement, output)
                  }}
                >
                  <input
                    ref={refineInputRef}
                    type="text"
                    value={refinement}
                    onChange={(event) => setRefinement(event.target.value)}
                    placeholder={t('editor.inlineEdit.refinePlaceholder')}
                    aria-label={t('editor.inlineEdit.refinePlaceholder')}
                    className="h-7 min-w-0 flex-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-text-muted)] focus:outline-none"
                  />
                  <Button
                    type="submit"
                    size="xs"
                    variant="outline"
                    disabled={!refinement.trim()}
                    className="h-7 shrink-0 px-2 text-xs"
                  >
                    {t('editor.inlineEdit.refine')}
                  </Button>
                </form>
              </>
            )}
          </div>
        </div>
      ) : null}
    </div>
  )
}
