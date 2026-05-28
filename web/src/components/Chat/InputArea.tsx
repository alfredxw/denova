import { useState, useRef, useCallback, useEffect } from 'react'
import { Send, Square } from 'lucide-react'
import { FileReferencePicker } from './FileReferencePicker'
import { ReferenceChips } from './ReferenceChips'
import type { TextSelection } from '@/lib/api'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

/** 可用命令列表 */
const COMMANDS = [
  { cmd: '/plan', desc: '先规划再执行' },
  { cmd: '/clear', desc: '清空对话' },
  { cmd: '/status', desc: '查看状态' },
  { cmd: '/help', desc: '帮助信息' },
  { cmd: '/outline', desc: '生成大纲' },
  { cmd: '/continue', desc: '继续写作' },
  { cmd: '/rewrite', desc: '重写章节' },
]

interface InputAreaProps {
  onSend: (message: string) => void
  onStop?: () => void
  disabled: boolean
  referencedFiles?: string[]
  onReferenceRemove?: (path: string) => void
  fileSuggestions?: string[]
  styleReferences?: string[]
  onStyleReferenceAdd?: (path: string) => void
  onStyleReferenceRemove?: (path: string) => void
  styleSuggestions?: string[]
  textSelections?: TextSelection[]
  onTextSelectionRemove?: (index: number) => void
}

/** 输入区域组件，支持 Enter 发送和命令菜单 */
export function InputArea({
  onSend,
  onStop,
  disabled,
  referencedFiles = [],
  onReferenceRemove,
  fileSuggestions = [],
  styleReferences = [],
  onStyleReferenceAdd,
  onStyleReferenceRemove,
  styleSuggestions = [],
  textSelections = [],
  onTextSelectionRemove,
}: InputAreaProps) {
  const [value, setValue] = useState('')
  const [showCommands, setShowCommands] = useState(false)
  const [filteredCommands, setFilteredCommands] = useState(COMMANDS)
  const [referenceQuery, setReferenceQuery] = useState<string | null>(null)
  const [styleReferenceQuery, setStyleReferenceQuery] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  /** 自动调整高度 */
  const adjustHeight = useCallback(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 160) + 'px'
  }, [])

  useEffect(() => { adjustHeight() }, [value, adjustHeight])

  /** 处理输入变化 */
  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const v = e.target.value
    setValue(v)

    // 检测是否输入了 /
    if (v.startsWith('/')) {
      const query = v.toLowerCase()
      const filtered = COMMANDS.filter(c => c.cmd.startsWith(query))
      setFilteredCommands(filtered)
      setShowCommands(filtered.length > 0)
    } else {
      setShowCommands(false)
    }

    const atMatch = v.match(/(?:^|\s)@([^\s@]*)$/)
    setReferenceQuery(atMatch ? atMatch[1] : null)
    const styleMatch = v.match(/(?:^|\s)#([^\s#]*)$/)
    setStyleReferenceQuery(styleMatch ? styleMatch[1] : null)
  }

  /** 处理键盘事件 */
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const isMod = e.metaKey || e.ctrlKey

    // Enter 发送
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
      return
    }

    // Escape 关闭菜单
    if (e.key === 'Escape') {
      setShowCommands(false)
      setReferenceQuery(null)
      setStyleReferenceQuery(null)
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
    setReferenceQuery(null)
    setStyleReferenceQuery(null)
  }

  /** 选择命令 */
  const selectCommand = (cmd: string) => {
    setValue(cmd + ' ')
    setShowCommands(false)
    textareaRef.current?.focus()
  }

  /** 选择引用文件并插入 @path 标签 */
  const selectReference = (path: string) => {
    setValue((current) => current.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}@${path} `
    }))
    setReferenceQuery(null)
    textareaRef.current?.focus()
  }

  /** 选择风格参考并插入 #path 标签 */
  const selectStyleReference = (path: string) => {
    setValue((current) => current.replace(/(?:^|\s)#([^\s#]*)$/, (match) => {
      const prefix = match.startsWith(' ') ? ' ' : ''
      return `${prefix}#${path} `
    }))
    onStyleReferenceAdd?.(path)
    setStyleReferenceQuery(null)
    textareaRef.current?.focus()
  }

  return (
    <div className="relative border-t border-[#303238] bg-[#202124] p-3">
      <ReferenceChips files={referencedFiles} onRemove={onReferenceRemove} />
      <ReferenceChips files={styleReferences} onRemove={onStyleReferenceRemove} prefix="#" tone="style" />
      {textSelections.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {textSelections.map((sel, idx) => (
            <span
              key={idx}
              className="inline-flex max-w-full items-center gap-1 rounded-md bg-[#2f9e44]/20 px-2 py-0.5 text-xs text-[#b2f2bb]"
            >
              <span className="truncate">
                {sel.fileName}:L{sel.startLine}
                {sel.endLine !== sel.startLine && `-L${sel.endLine}`}
                {' '}
                <span className="text-[#8fbc8f]">
                  {sel.content.length > 30 ? sel.content.slice(0, 30) + '…' : sel.content}
                </span>
              </span>
              {onTextSelectionRemove && (
                <button
                  type="button"
                  className="rounded text-[#8fbc8f] hover:text-white"
                  onClick={() => onTextSelectionRemove(idx)}
                >
                  ×
                </button>
              )}
            </span>
          ))}
        </div>
      )}

      <Popover open={showCommands}>
        <PopoverTrigger asChild>
          <span className="absolute bottom-full left-3 h-0 w-0" />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          side="top"
          className="mb-1 w-[360px] border-[#303238] bg-[#25262a] p-0 text-[#d7dbe2] shadow-xl"
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <Command shouldFilter={false} className="bg-transparent">
            <CommandList>
              <CommandEmpty>未找到命令</CommandEmpty>
              <CommandGroup heading="命令">
                {filteredCommands.map(({ cmd, desc }) => (
                  <CommandItem
                    key={cmd}
                    value={cmd}
                    onSelect={() => selectCommand(cmd)}
                    className="cursor-pointer text-[#d7dbe2] data-[selected=true]:bg-[#303238]"
                  >
                    <span className="font-mono text-[#a8adb7]">{cmd}</span>
                    <span className="text-[#858b96]">{desc}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      <FileReferencePicker
        open={referenceQuery !== null && fileSuggestions.length > 0}
        query={referenceQuery || ''}
        files={fileSuggestions}
        onSelect={selectReference}
      />

      <FileReferencePicker
        open={styleReferenceQuery !== null && styleSuggestions.length > 0}
        query={styleReferenceQuery || ''}
        files={styleSuggestions}
        onSelect={selectStyleReference}
        trigger="#"
        placeholder="搜索风格参考..."
        emptyText="未找到风格参考"
        heading="风格参考"
      />

      <div className="flex items-end gap-2">
        <Textarea
          ref={textareaRef}
          value={value}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={disabled ? 'AI 正在回复…' : '输入消息，Enter 发送，Shift+Enter 换行'}
          disabled={disabled}
          rows={1}
          className="min-h-0 flex-1 resize-none border-[#3a3d44] bg-[#1b1c1f] px-3 py-2 text-sm text-[#d7dbe2] placeholder:text-[#6f7682] focus-visible:border-[#4a4d54] focus-visible:ring-0 disabled:opacity-50"
        />
        <Button
          type="button"
          onClick={disabled ? onStop : handleSend}
          disabled={disabled ? !onStop : !value.trim()}
          size="icon-sm"
          className={`shrink-0 text-white ${
            disabled ? 'bg-[#c95050] hover:bg-[#e05d5d]' : 'bg-[#4a4d54] hover:bg-[#5a5d64]'
          }`}
          aria-label={disabled ? '中断 AI 执行' : '发送'}
        >
          {disabled ? <Square className="h-3.5 w-3.5 fill-current" /> : <Send className="w-4 h-4" />}
        </Button>
      </div>
    </div>
  )
}
