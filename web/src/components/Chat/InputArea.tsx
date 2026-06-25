import { useState, useRef, useEffect, useMemo, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Archive, BadgeHelp, BarChart3, ClipboardList, Command as CommandIcon, Eraser, Layers3, List, ListTree, PenLine, ScrollText, Send, Sparkles, Square, WandSparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FileReferencePicker, type ReferencePickerItem } from './FileReferencePicker'
import { ReferenceChips } from './ReferenceChips'
import { TokenUsageDialog } from './TokenUsagePanel'
import type { ChatMessage, TextSelection } from '@/lib/api'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { AgentComposerShell } from './AgentComposerShell'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { isComposingKeyboardEvent } from '@/lib/keyboard'

/** 可用命令列表 */
const COMMANDS: Array<{ cmd: string; descKey: string; hintKey: string; icon: LucideIcon }> = [
  { cmd: '/plan', descKey: 'chat.command.plan.desc', hintKey: 'chat.command.plan.hint', icon: ClipboardList },
  { cmd: '/clear', descKey: 'chat.command.clear.desc', hintKey: 'chat.command.clear.hint', icon: Eraser },
  { cmd: '/compact', descKey: 'chat.command.compact.desc', hintKey: 'chat.command.compact.hint', icon: Archive },
  { cmd: '/status', descKey: 'chat.command.status.desc', hintKey: 'chat.command.status.hint', icon: Sparkles },
  { cmd: '/help', descKey: 'chat.command.help.desc', hintKey: 'chat.command.help.hint', icon: BadgeHelp },
  { cmd: '/outline', descKey: 'chat.command.outline.desc', hintKey: 'chat.command.outline.hint', icon: ListTree },
  { cmd: '/group-plan', descKey: 'chat.command.groupPlan.desc', hintKey: 'chat.command.groupPlan.hint', icon: Layers3 },
  { cmd: '/continue', descKey: 'chat.command.continue.desc', hintKey: 'chat.command.continue.hint', icon: PenLine },
  { cmd: '/rewrite', descKey: 'chat.command.rewrite.desc', hintKey: 'chat.command.rewrite.hint', icon: WandSparkles },
]

export interface SkillCommand {
  name: string
  description: string
}

type CommandOption = {
  cmd: string
  description: string
  hint: string
  icon: LucideIcon
}

type CommandScope = 'all' | 'skills' | 'none'
const inputDrafts = new Map<string, string>()

interface InputAreaProps {
  onSend: (message: string) => void
  onStop?: () => void
  disabled: boolean
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
  skills?: SkillCommand[]
  commandsEnabled?: boolean
  commandScope?: CommandScope
  placeholder?: string
  disabledPlaceholder?: string
  onContextAnalyze?: (message: string) => void | Promise<void>
  tokenUsageMessages?: ChatMessage[]
  agentKey?: VisibleAgentKey
  workspace?: string
  writingSkillControl?: ReactNode
  floating?: boolean
}

/** 输入区域组件，支持 Enter 发送和命令菜单 */
export function InputArea({
  onSend,
  onStop,
  disabled,
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
  skills = [],
  commandsEnabled = true,
  commandScope = 'all',
  placeholder,
  disabledPlaceholder,
  onContextAnalyze,
  tokenUsageMessages = [],
  agentKey,
  workspace,
  writingSkillControl,
  floating = false,
}: InputAreaProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState(() => draftKey ? inputDrafts.get(draftKey) || '' : '')
  const [tokenUsageOpen, setTokenUsageOpen] = useState(false)
  const [showCommands, setShowCommands] = useState(false)
  const [activeCommandIndex, setActiveCommandIndex] = useState(0)
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const commandItemRefs = useRef<Array<HTMLDivElement | null>>([])
  const effectiveCommandScope: CommandScope = commandsEnabled ? commandScope : 'none'
  const defaultPlaceholder = skills.length > 0 && effectiveCommandScope !== 'none'
    ? t('chat.input.placeholderWithSkills')
    : t('chat.input.placeholder')
  const allCommands = useMemo<CommandOption[]>(() => {
    const staticCommands = effectiveCommandScope === 'all' ? COMMANDS.map(({ cmd, descKey, hintKey, icon }) => ({
      cmd,
      description: t(descKey),
      hint: t(hintKey),
      icon,
    })) : []
    const seen = new Set(staticCommands.map((command) => command.cmd))
    const skillCommands = skills
      .map((skill) => ({
        cmd: `/${skill.name}`,
        description: skill.description || skill.name,
        hint: t('chat.command.skill.hint'),
        icon: Sparkles,
      }))
      .filter((command) => {
        if (seen.has(command.cmd)) return false
        seen.add(command.cmd)
        return true
      })
    if (effectiveCommandScope === 'skills') return skillCommands
    if (effectiveCommandScope === 'none') return []
    return [...staticCommands, ...skillCommands]
  }, [effectiveCommandScope, skills, t])
  const filteredCommands = useMemo(() => {
    if (!value.startsWith('/')) return []
    const query = value.toLowerCase()
    return allCommands.filter((command) => command.cmd.toLowerCase().startsWith(query))
  }, [allCommands, value])

  useEffect(() => {
    if (!draftKey) return
    setValue(inputDrafts.get(draftKey) || '')
    setShowCommands(false)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
  }, [draftKey])

  useEffect(() => {
    if (!draftKey) return
    if (value) inputDrafts.set(draftKey, value)
    else inputDrafts.delete(draftKey)
  }, [draftKey, value])

  useEffect(() => {
    if (activeCommandIndex >= filteredCommands.length) setActiveCommandIndex(0)
  }, [activeCommandIndex, filteredCommands.length])

  useEffect(() => {
    if (!showCommands || filteredCommands.length === 0) return
    commandItemRefs.current[activeCommandIndex]?.scrollIntoView({ block: 'nearest' })
  }, [activeCommandIndex, filteredCommands.length, showCommands])

  useEffect(() => {
    if (!inputPrefill) return
    setValue(inputPrefill.prompt)
    setShowCommands(false)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
    window.requestAnimationFrame(() => textareaRef.current?.focus())
    onInputPrefillConsumed?.()
  }, [inputPrefill, onInputPrefillConsumed])

  /** 处理输入变化 */
  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const v = e.target.value
    setValue(v)

    // 检测是否输入了 /
    if (effectiveCommandScope !== 'none' && v.startsWith('/')) {
      setShowCommands(true)
      setActiveCommandIndex(0)
    } else {
      setShowCommands(false)
      setActiveCommandIndex(0)
    }

    const atMatch = v.match(/(?:^|\s)@([^\s@]*)$/)
    setReferenceQuery(atMatch ? atMatch[1] : null)
    const styleMatch = v.match(/(?:^|\s)#([^\s#]*)$/)
    setStyleSceneQuery(styleMatch ? styleMatch[1] : null)
  }

  /** 处理键盘事件 */
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const isMod = e.metaKey || e.ctrlKey
    const canPickCommand = effectiveCommandScope !== 'none' && showCommands && filteredCommands.length > 0

    if (canPickCommand && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault()
      setActiveCommandIndex((current) => {
        const direction = e.key === 'ArrowDown' ? 1 : -1
        return (current + direction + filteredCommands.length) % filteredCommands.length
      })
      return
    }

    // Enter 发送
    if (e.key === 'Enter' && !e.shiftKey) {
      if (isComposingKeyboardEvent(e)) return
      e.preventDefault()
      if (canPickCommand) {
        selectCommand(filteredCommands[activeCommandIndex]?.cmd || filteredCommands[0].cmd)
        return
      }
      handleSend()
      return
    }

    if (canPickCommand && e.key === 'Tab') {
      e.preventDefault()
      selectCommand(filteredCommands[activeCommandIndex]?.cmd || filteredCommands[0].cmd)
      return
    }

    // Escape 关闭菜单
    if (e.key === 'Escape') {
      setShowCommands(false)
      setActiveCommandIndex(0)
      setReferenceQuery(null)
      setStyleSceneQuery(null)
      return
    }

    // Cmd+A：全选输入框内容（阻止冒泡，防止被全局事件拦截）
    if (isMod && e.key === 'a') {
      e.stopPropagation()
      textareaRef.current?.select()
      return
    }

    // Cmd+Backspace：删除光标到行首
    if (isMod && e.key === 'Backspace') {
      e.preventDefault()
      const el = textareaRef.current
      if (!el) return
      const pos = el.selectionStart
      const before = el.value.substring(0, pos)
      const lineStart = before.lastIndexOf('\n') + 1
      const newValue = el.value.substring(0, lineStart) + el.value.substring(pos)
      setValue(newValue)
      requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = lineStart })
      return
    }

    // Cmd+Shift+K：删除整行
    if (isMod && e.shiftKey && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      const el = textareaRef.current
      if (!el) return
      const pos = el.selectionStart
      const lineStart = el.value.lastIndexOf('\n', pos - 1) + 1
      let lineEnd = el.value.indexOf('\n', pos)
      if (lineEnd === -1) lineEnd = el.value.length
      else lineEnd += 1 // 包括换行符
      const newValue = el.value.substring(0, lineStart) + el.value.substring(lineEnd)
      setValue(newValue)
      requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = lineStart })
      return
    }

    // Cmd+D：选择当前词（类 VSCode 行为）
    if (isMod && e.key.toLowerCase() === 'd') {
      e.preventDefault()
      const el = textareaRef.current
      if (!el) return
      const text = el.value
      const pos = el.selectionStart
      const wordBoundary = /[\s,.:;!?'"(){}[\]@#$%^&*+=<>/\\|~`\-]/
      let start = pos
      while (start > 0 && !wordBoundary.test(text[start - 1])) start--
      let end = pos
      while (end < text.length && !wordBoundary.test(text[end])) end++
      el.selectionStart = start
      el.selectionEnd = end
      return
    }
  }

  /** 发送消息 */
  const handleSend = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    setShowCommands(false)
    setActiveCommandIndex(0)
    setReferenceQuery(null)
    setStyleSceneQuery(null)
  }

  const handleContextAnalyze = () => {
    if (disabled) return
    void onContextAnalyze?.(value)
  }
  const hasReferences = referencedFiles.length > 0 || loreReferences.length > 0 || styleScenes.length > 0 || textSelections.length > 0

  /** 选择命令 */
  const selectCommand = (cmd: string) => {
    setValue(cmd + ' ')
    setShowCommands(false)
    setActiveCommandIndex(0)
    textareaRef.current?.focus()
  }

  /** 选择引用文件并插入 @path 标签 */
  const selectReference = (path: string) => {
    setValue((current) => current.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      const loreItem = loreSuggestions.find((item) => item.value === path)
      return `${prefix}@${loreItem ? `资料:${loreItem.label}` : path} `
    }))
    if (loreSuggestions.some((item) => item.value === path)) {
      onLoreReferenceAdd?.(path)
    }
    setReferenceQuery(null)
    textareaRef.current?.focus()
  }

  /** 选择场景风格并插入 #scene 标签 */
  const selectStyleScene = (scene: string) => {
    setValue((current) => current.replace(/(?:^|\s)#([^\s#]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}#${scene} `
    }))
    onStyleSceneAdd?.(scene)
    setStyleSceneQuery(null)
    textareaRef.current?.focus()
  }

  return (
    <div className={floating ? 'nova-chat-input-area nova-chat-input-area-floating' : 'nova-chat-input-area relative border-t border-[var(--nova-border)] p-3'}>
      <Popover open={showCommands && filteredCommands.length > 0}>
        <PopoverTrigger asChild>
          <span className="absolute bottom-full left-3 h-0 w-0" />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          side="top"
          className="nova-command-menu mb-2 w-[384px] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <Command shouldFilter={false} className="bg-transparent">
            <div className="border-b border-[var(--nova-border-soft)] px-3 py-2">
              <div className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
                    <CommandIcon className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0">
                    <div className="text-xs font-medium text-[var(--nova-text)]">{t('chat.commands.title')}</div>
                    <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.commands.description')}</div>
                  </div>
                </div>
                <kbd className="shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">/</kbd>
              </div>
            </div>
            <CommandList className="max-h-[312px] p-1.5">
              <CommandEmpty className="py-5 text-center text-xs text-[var(--nova-text-faint)]">{t('chat.commands.empty')}</CommandEmpty>
              <CommandGroup heading={t('chat.commands.group')} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]">
                {filteredCommands.map(({ cmd, description, hint, icon: Icon }, index) => {
                  const active = index === activeCommandIndex
                  return (
                    <CommandItem
                      key={cmd}
                      ref={(element) => { commandItemRefs.current[index] = element }}
                      value={cmd}
                      onMouseEnter={() => setActiveCommandIndex(index)}
                      onSelect={() => selectCommand(cmd)}
                      className={`group min-h-12 cursor-pointer rounded-md border px-2.5 py-2 text-[var(--nova-text-muted)] ${
                        active
                          ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                          : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'
                      }`}
                    >
                    <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-[var(--nova-surface-2)] ${
                      active ? 'border-[var(--nova-border)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] text-[var(--nova-text-faint)]'
                    }`}>
                      <Icon className="h-3.5 w-3.5" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span className="font-mono text-xs text-[var(--nova-text)]">{cmd}</span>
                        <span className="truncate text-xs text-[var(--nova-text-muted)]">{description}</span>
                      </span>
                      <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{hint}</span>
                    </span>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      <FileReferencePicker
        open={referenceQuery !== null && (fileSuggestions.length > 0 || loreSuggestions.length > 0)}
        query={referenceQuery || ''}
        files={[
          ...loreSuggestions,
          ...fileSuggestions,
        ]}
        onSelect={selectReference}
      />

      <FileReferencePicker
        open={styleSceneQuery !== null && styleSceneSuggestions.length > 0}
        query={styleSceneQuery || ''}
        files={styleSceneSuggestions}
        onSelect={selectStyleScene}
        trigger="#"
        placeholder={t('chat.styleReference.placeholder')}
        emptyText={t('chat.styleReference.empty')}
        heading={t('chat.styleReference.heading')}
      />

      <AgentComposerShell
        references={hasReferences ? (
          <>
            <ReferenceChips files={referencedFiles} onRemove={onReferenceRemove} />
            <ReferenceChips
              files={loreReferences.map((id) => loreReferenceLabels[id] || id)}
              onRemove={onLoreReferenceRemove ? (label) => {
                const target = loreReferences.find((id) => (loreReferenceLabels[id] || id) === label)
                if (target) onLoreReferenceRemove(target)
              } : undefined}
              prefix="@资料:"
              tone="lore"
            />
            <ReferenceChips files={styleScenes} onRemove={onStyleSceneRemove} prefix="#" tone="style" />
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
          <Textarea
            ref={textareaRef}
            autoResize
            multilineMode="sticky-until-empty"
            value={value}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            placeholder={disabled ? (disabledPlaceholder ?? t('chat.input.disabledPlaceholder')) : (placeholder ?? defaultPlaceholder)}
            disabled={disabled}
            rows={1}
            className="nova-agent-composer-textarea min-h-[42px] resize-none border-0 bg-transparent px-1 py-[9px] text-sm leading-6 text-[var(--nova-text)] shadow-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0 disabled:opacity-50"
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
                  disabled={!writingSkillControl && !onContextAnalyze && tokenUsageMessages.length === 0 && !(agentKey && workspace)}
                  aria-label={t('chat.input.actions')}
                  title={t('chat.input.actions')}
                >
                  <List className="h-3.5 w-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" side="top" className="w-80 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                {writingSkillControl}
                <ModelProfileSwitcher agentKey={agentKey} workspace={workspace} disabled={disabled} />
                <DropdownMenuItem
                  onSelect={() => setTokenUsageOpen(true)}
                  className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                >
                  <BarChart3 className="h-3.5 w-3.5" />
                  <span className="min-w-0 flex-1">{t('chat.tokenUsage.action')}</span>
                  <span className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.subtitle', { count: tokenUsageMessages.length })}</span>
                </DropdownMenuItem>
                <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
                <DropdownMenuItem
                  disabled={disabled}
                  onSelect={handleContextAnalyze}
                  className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                >
                  <ScrollText className="h-3.5 w-3.5" />
                  {t('chat.contextAnalysis.action')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <TokenUsageDialog open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} />
          </>
        }
        submitControl={
          <Button
            type="button"
            onClick={disabled ? onStop : handleSend}
            disabled={disabled ? !onStop : !value.trim()}
            size="icon-sm"
            className={`nova-agent-composer-submit h-9 w-9 shrink-0 rounded-[10px] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] ${
              disabled ? 'bg-[var(--nova-danger-bg)] hover:bg-[var(--nova-danger-bg)]' : 'bg-[var(--nova-active)] hover:bg-[var(--nova-hover)] disabled:bg-[var(--nova-active)]'
            }`}
            aria-label={disabled ? t('chat.input.stop') : t('chat.input.send')}
          >
            {disabled ? <Square className="h-3.5 w-3.5 fill-current" /> : <Send className="h-4 w-4" />}
          </Button>
        }
      />
    </div>
  )
}
