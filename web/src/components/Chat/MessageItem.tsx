import { Children, Fragment, cloneElement, isValidElement, memo, useEffect, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Bot, CheckCircle2, ChevronDown, ChevronRight, Circle, CircleDot, Clock3, FileText, ListTodo } from 'lucide-react'
import type { ChatMessage } from '@/lib/api'

interface MessageItemProps {
  message: ChatMessage
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
}

/** 单条消息组件，根据 role 渲染不同样式 */
export const MessageItem = memo(function MessageItem({ message, highlightDialogue = false, messageStyle }: MessageItemProps) {
  const { role, content = '' } = message

  switch (role) {
    case 'user':
      return (
        <div className="flex justify-end">
          <div className="max-w-[88%] rounded bg-[#4a4d54] px-3 py-2 text-sm text-white whitespace-pre-wrap" style={messageStyle}>
            {content}
          </div>
        </div>
      )

    case 'assistant':
      return (
        <div className="flex justify-start">
          <div className="chat-agent-message w-full text-sm text-[#c8ccd4]" style={messageStyle}>
            <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[#d7dbe2]">
              <span className="flex h-5 w-5 items-center justify-center rounded border border-[#5a5d64]/50 bg-[#1b1c1f] text-[#d7dbe2]">
                <Bot className="h-3.5 w-3.5" />
              </span>
              Nova
            </div>
            {message.streaming ? (
              <StreamingMarkdown content={content} highlightDialogue={highlightDialogue} />
            ) : (
              <MarkdownContent content={content} highlightDialogue={highlightDialogue} />
            )}
          </div>
        </div>
      )

    case 'thinking':
      return <ThinkingBlock content={content} streaming={message.streaming === true} />

    case 'tool_call':
      if ((message.name || '') === 'write_todos') {
        return <TodoListBlock message={message} />
      }
      return <ToolExecutionBlock message={message} />

    case 'tool_result':
      return <ToolResultBlock content={content} />

    case 'system':
      return (
        <div className="flex justify-center">
          <span className="rounded-full border border-[#303238] bg-[#25262a] px-3 py-1 text-xs text-[#858b96]">
            {content}
          </span>
        </div>
      )

    case 'error':
      return (
        <div className="flex justify-center">
          <span className="rounded-full border border-[#5c2a2a] bg-[#2a1f1f] px-3 py-1 text-xs text-[#ff6b6b]">
            {content}
          </span>
        </div>
      )

    default:
      return null
  }
})

/** 工具执行中的轻量状态卡片 */
export function ToolActivityBlock({ content }: { content: string }) {
  const activity = parseActivityContent(content)

  return (
    <div className="flex justify-start">
      <div className="w-full rounded-lg border border-[#303238] bg-[#202124] px-3 py-2 text-xs shadow-[0_8px_24px_rgba(0,0,0,0.18)]">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[#5a5d64]/40 bg-[#25262a] text-[#d7dbe2]">
            <Clock3 className="h-3.5 w-3.5 animate-pulse" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2 text-[#d7dbe2]">
              <span className="font-medium">{activity.title}</span>
              {activity.toolName && (
                <code className="rounded border border-[#454956] bg-[#1b1c20] px-1.5 py-0.5 font-mono text-[11px] text-[#d7dbe2]">
                  {activity.toolName}
                </code>
              )}
            </div>
            {activity.detail && <div className="mt-1 truncate text-[#9aa1ad]">{activity.detail}</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

/** 工具执行卡片，默认以单行展示运行态和结果态。 */
export function ToolExecutionBlock({ message }: { message: ChatMessage }) {
  const [expanded, setExpanded] = useState(false)
  const info = parseToolCallContent(message.content || '')
  const name = message.name || info.name
  const rawArgs = message.args !== undefined ? message.args : info.args
  const args = formatMaybeJSON(rawArgs)
  const status = message.status || 'running'
  const result = message.result || ''
  const hasResult = status === 'success'
  const isStreamingContent = status === 'running' && isContentTool(name) && rawArgs.length > 50
  const streamPreview = isStreamingContent ? extractStreamingContent(rawArgs) : ''
  const summary = buildToolArgSummary(args) || (isStreamingContent ? '正在写入…' : '准备执行工具请求')
  const resultPreview = buildPreview(result, 80)
  const hasDetail = Boolean(args || result)

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-md border border-[#303238] bg-[#23252a] text-xs">
        <div className="flex h-9 min-w-0 items-center gap-2 px-2.5">
          <ToolStatusIcon status={status} />
          <span className="shrink-0 font-medium text-[#d7dbe2]">调用工具</span>
          <code className="shrink-0 rounded border border-[#454956] bg-[#1b1c20] px-1.5 py-0.5 font-mono text-[11px] text-[#d7dbe2]">
            {name}
          </code>
          <span className="min-w-0 flex-1 truncate text-[#9aa1ad]">
            {hasResult ? resultPreview || '无返回内容' : summary}
          </span>
          {hasDetail && !isStreamingContent && (
            <button
              type="button"
              className="shrink-0 text-[#aeb4bf] hover:text-[#f0f2f5]"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? '收起' : '详情'}
            </button>
          )}
        </div>
        {/* 流式写入时展示实时内容预览 */}
        {isStreamingContent && streamPreview && (
          <div className="max-h-32 overflow-auto border-t border-[#303238] bg-[#1b1c20] px-3 py-2 font-mono text-[11px] leading-relaxed text-[#b8d4a8] whitespace-pre-wrap">
            {streamPreview}
          </div>
        )}
        {expanded && !isStreamingContent && (
          <div className="grid max-h-48 gap-2 overflow-auto border-t border-[#303238] bg-[#1b1c20] px-3 py-2 font-mono text-[11px] leading-relaxed text-[#aeb4bf]">
            {args && <pre className="whitespace-pre-wrap">{args}</pre>}
            {result && <pre className="whitespace-pre-wrap text-[#91d99f]">{result}</pre>}
          </div>
        )}
      </div>
    </div>
  )
}

interface TodoItem {
  content: string
  activeForm?: string
  status: 'pending' | 'in_progress' | 'completed' | string
}

/** Agentic Loop write_todos 工具卡片：渲染为可读的待办列表，兼容流式不完整 args */
export function TodoListBlock({ message }: { message: ChatMessage }) {
  const args = message.args || ''
  const todos = parseTodosFromArgs(args)
  const status = message.status || 'running'
  const total = todos.length
  const completed = todos.filter(t => t.status === 'completed').length
  const inProgress = todos.find(t => t.status === 'in_progress')
  const headline = inProgress?.activeForm || inProgress?.content || (status === 'success' ? '已更新待办列表' : '正在更新待办列表…')

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-md border border-[#303238] bg-[#23252a] text-xs">
        <div className="flex h-9 min-w-0 items-center gap-2 px-2.5">
          <ListTodo className="h-3.5 w-3.5 shrink-0 text-[#a8adb7]" />
          <span className="shrink-0 font-medium text-[#d7dbe2]">待办列表</span>
          {total > 0 && (
            <span className="shrink-0 rounded-full border border-[#454956] bg-[#1b1c20] px-1.5 py-0.5 font-mono text-[11px] text-[#9aa1ad]">
              {completed}/{total}
            </span>
          )}
          <span className="min-w-0 flex-1 truncate text-[#9aa1ad]">{headline}</span>
        </div>
        {todos.length > 0 && (
          <ul className="border-t border-[#303238] bg-[#1b1c20] px-3 py-2">
            {todos.map((todo, index) => (
              <TodoListItem key={index} todo={todo} />
            ))}
          </ul>
        )}
        {todos.length === 0 && (
          <div className="border-t border-[#303238] bg-[#1b1c20] px-3 py-2 text-[#9aa1ad]">
            {status === 'running' ? '解析中…' : '空待办'}
          </div>
        )}
      </div>
    </div>
  )
}

function TodoListItem({ todo }: { todo: TodoItem }) {
  const text = todo.status === 'in_progress' && todo.activeForm ? todo.activeForm : todo.content
  if (todo.status === 'completed') {
    return (
      <li className="flex items-start gap-2 py-0.5 leading-6">
        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[#7bd88f]" />
        <span className="text-[#7f8593] line-through">{text}</span>
      </li>
    )
  }
  if (todo.status === 'in_progress') {
    return (
      <li className="flex items-start gap-2 py-0.5 leading-6">
        <CircleDot className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-pulse text-[#d7dbe2]" />
        <span className="text-[#e4e7ee]">{text}</span>
      </li>
    )
  }
  return (
    <li className="flex items-start gap-2 py-0.5 leading-6">
      <Circle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[#5b6070]" />
      <span className="text-[#c8ccd4]">{text}</span>
    </li>
  )
}

/** 解析 write_todos 工具参数，对流式中可能不完整的 JSON 做容错 */
function parseTodosFromArgs(args: string): TodoItem[] {
  if (!args) return []
  const trimmed = args.trim()
  if (!trimmed) return []
  // 优先尝试完整 JSON
  try {
    const data = JSON.parse(trimmed) as { todos?: TodoItem[] }
    if (Array.isArray(data?.todos)) return data.todos
  } catch {
    // 流式中常见：args 不完整或被截断
  }
  // 回退：从 todos 数组中提取已经完整的对象
  const arrayMatch = trimmed.match(/"todos"\s*:\s*\[([\s\S]*)$/)
  if (!arrayMatch) return []
  const body = arrayMatch[1]
  const items: TodoItem[] = []
  let depth = 0
  let start = -1
  let inString = false
  let escape = false
  for (let i = 0; i < body.length; i++) {
    const ch = body[i]
    if (escape) { escape = false; continue }
    if (ch === '\\') { escape = true; continue }
    if (ch === '"') { inString = !inString; continue }
    if (inString) continue
    if (ch === '{') {
      if (depth === 0) start = i
      depth++
    } else if (ch === '}') {
      depth--
      if (depth === 0 && start >= 0) {
        const piece = body.slice(start, i + 1)
        try {
          items.push(JSON.parse(piece) as TodoItem)
        } catch {
          // 单个对象解析失败时跳过
        }
        start = -1
      }
    }
  }
  return items
}

function ToolStatusIcon({ status }: { status: ChatMessage['status'] }) {
  if (status === 'success') {
    return <span className="shrink-0 text-sm leading-none">✅</span>
  }
  if (status === 'error') {
    return <span className="flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full border border-[#ff6b6b] text-[10px] text-[#ff6b6b]">!</span>
  }
  return <span className="h-3.5 w-3.5 shrink-0 animate-spin rounded-full border-2 border-[#5a5d64]/35 border-t-[#d7dbe2]" />
}

/** 工具结果卡片，默认展示摘要，避免大段结果挤占对话区 */
function ToolResultBlock({ content }: { content: string }) {
  const [expanded, setExpanded] = useState(false)
  const preview = buildPreview(content, 160)
  const canExpand = content.trim().replace(/\s+/g, ' ').length > 160

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[#2f3d35] bg-[#202824] text-xs shadow-[0_8px_24px_rgba(0,0,0,0.12)]">
        <div className="flex items-start gap-3 px-3 py-2.5">
          <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[#3d7a55]/40 bg-[#1e3327] text-[#7bd88f]">
            <CheckCircle2 className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-[#d7dbe2]">工具执行完成</span>
              <span className="rounded-full border border-[#3d7a55]/40 bg-[#1b2a21] px-2 py-0.5 text-[11px] text-[#91d99f]">
                success
              </span>
            </div>
            <div className="mt-1 flex min-w-0 items-center gap-2 text-[#9aa1ad]">
              <FileText className="h-3.5 w-3.5 shrink-0 text-[#7bd88f]" />
              <span className="truncate">{preview || '无返回内容'}</span>
              {canExpand && (
                <button
                  type="button"
                  className="shrink-0 text-[#aeb4bf] hover:text-[#f0f2f5]"
                  onClick={() => setExpanded(!expanded)}
                >
                  {expanded ? '收起' : '展开'}
                </button>
              )}
            </div>
          </div>
        </div>
        {expanded && (
          <pre className="max-h-56 overflow-auto border-t border-[#2f3d35] bg-[#1b1f1d] px-3 py-2 font-mono text-[11px] leading-relaxed text-[#aeb4bf]">
            {content}
          </pre>
        )}
      </div>
    </div>
  )
}

function parseToolCallContent(content: string) {
  const [rawName = 'unknown_tool', ...rest] = content.split('\n')
  const name = rawName.trim() || 'unknown_tool'
  const args = formatMaybeJSON(rest.join('\n').trim())

  return {
    name,
    args,
    summary: buildToolArgSummary(args),
  }
}

function parseActivityContent(content: string) {
  const toolMatch = content.match(/^正在执行工具：([^\n]+)(?:\n([\s\S]*))?$/)
  if (toolMatch) {
    const args = formatMaybeJSON((toolMatch[2] || '').trim())
    return {
      title: '正在执行工具',
      toolName: toolMatch[1].trim(),
      detail: buildToolArgSummary(args) || '等待工具返回结果',
    }
  }

  const doneMatch = content.match(/^工具执行完成：?([\s\S]*)$/)
  if (doneMatch) {
    return {
      title: '工具执行完成',
      toolName: '',
      detail: buildPreview(doneMatch[1] || '', 120),
    }
  }

  return {
    title: content,
    toolName: '',
    detail: '',
  }
}

function formatMaybeJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function buildToolArgSummary(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const path = data.file_path || data.path || data.cwd || data.command
    if (typeof path === 'string' && path) return path
  } catch {
    // 非 JSON 参数使用通用预览。
  }
  return buildPreview(args, 120)
}

function buildPreview(content: string, maxLength: number) {
  const normalized = content.trim().replace(/\s+/g, ' ')
  if (normalized.length <= maxLength) return normalized
  return `${normalized.slice(0, maxLength)}...`
}

/** 判断是否为会产生大量内容参数的工具（适合流式预览） */
function isContentTool(name: string): boolean {
  return ['write_file', 'edit_file'].includes(name)
}

/** 从不完整的 JSON args 中提取 content/new_string 字段的流式文本 */
function extractStreamingContent(rawArgs: string): string {
  // 尝试提取 "content": "..." 或 "new_string": "..."
  const match = rawArgs.match(/"(?:content|new_string)"\s*:\s*"([\s\S]*)$/m)
  if (!match) return ''
  // 解码已有的 JSON 转义字符，末尾可能不完整
  let text = match[1]
  try {
    // 尝试解析 JSON 字符串（加上闭合引号使其合法）
    text = JSON.parse(`"${text}"`)
  } catch {
    // 不完整时做简单转义还原
    text = text.replace(/\\n/g, '\n').replace(/\\t/g, '\t').replace(/\\"/g, '"').replace(/\\\\/g, '\\')
  }
  // 只展示最后 500 字符以保持性能
  if (text.length > 500) {
    return '...' + text.slice(-500)
  }
  return text
}

/** 流式 Markdown 渲染，避免高频重建完整 Markdown AST。 */
function StreamingMarkdown({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return <StreamingMarkdownContent content={content} highlightDialogue={highlightDialogue} />
}

const MarkdownContent = memo(function MarkdownContent({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={highlightDialogue ? dialogueMarkdownComponents : undefined}
    >
      {content}
    </ReactMarkdown>
  )
})

/** 轻量流式 Markdown，只处理常见块级语法，保证输出即时不卡顿。 */
const StreamingMarkdownContent = memo(function StreamingMarkdownContent({ content, highlightDialogue }: { content: string; highlightDialogue: boolean }) {
  const lines = content.split('\n')
  const nodes = []
  let codeLines: string[] = []
  let inCodeBlock = false
  let codeBlockIndex = 0

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    const codeFence = line.match(/^```/)
    if (codeFence) {
      if (inCodeBlock) {
        nodes.push(
          <pre key={`code-${codeBlockIndex}`} className="my-2 overflow-x-auto rounded border border-[#303238] bg-[#1b1c20] px-3 py-2 text-xs leading-relaxed text-[#d7dbe2]">
            <code>{codeLines.join('\n')}</code>
          </pre>,
        )
        codeBlockIndex += 1
        codeLines = []
        inCodeBlock = false
      } else {
        inCodeBlock = true
      }
      continue
    }

    if (inCodeBlock) {
      codeLines.push(line)
      continue
    }

    nodes.push(renderStreamingMarkdownLine(line, index, highlightDialogue))
  }

  if (inCodeBlock) {
    nodes.push(
      <pre key={`code-open-${codeBlockIndex}`} className="my-2 overflow-x-auto rounded border border-[#303238] bg-[#1b1c20] px-3 py-2 text-xs leading-relaxed text-[#d7dbe2]">
        <code>{codeLines.join('\n')}</code>
      </pre>,
    )
  }

  return <div className="streaming-markdown">{nodes}</div>
})

function renderStreamingMarkdownLine(line: string, index: number, highlightDialogue: boolean) {
  if (!line.trim()) {
    return <div key={`blank-${index}`} className="h-3" />
  }

  const heading = line.match(/^(#{1,6})\s+(.+)$/)
  if (heading) {
    const level = heading[1].length
    const className = level <= 2
      ? 'mt-3 mb-1 text-base font-semibold text-[#e4e7ee]'
      : 'mt-2 mb-1 text-sm font-semibold text-[#d7dbe2]'
    return <div key={`h-${index}`} className={className}>{renderInlineMarkdown(heading[2], highlightDialogue)}</div>
  }

  const listItem = line.match(/^(\s*)([-*+]|\d+\.)\s+(.+)$/)
  if (listItem) {
    const depth = Math.min(Math.floor(listItem[1].length / 2), 4)
    return (
      <div key={`li-${index}`} className="flex gap-2 leading-7 text-[#c8ccd4]" style={{ paddingLeft: `${depth * 1.25}rem` }}>
        <span className="shrink-0 text-[#858b96]">{listItem[2].match(/\d+\./) ? listItem[2] : '•'}</span>
        <span>{renderInlineMarkdown(listItem[3], highlightDialogue)}</span>
      </div>
    )
  }

  const quote = line.match(/^>\s?(.*)$/)
  if (quote) {
    return <div key={`quote-${index}`} className="border-l border-[#454956] pl-3 leading-7 text-[#aeb4bf]">{renderInlineMarkdown(quote[1], highlightDialogue)}</div>
  }

  return <div key={`p-${index}`} className="leading-7 text-[#c8ccd4]">{renderInlineMarkdown(line, highlightDialogue)}</div>
}

function renderInlineMarkdown(text: string, highlightDialogue = false): ReactNode[] {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g)
  return parts.map((part, index) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={index} className="rounded bg-[#1b1c20] px-1 py-0.5 font-mono text-[0.9em] text-[#d7dbe2]">{part.slice(1, -1)}</code>
    }
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={index} className="font-semibold text-[#e4e7ee]">{highlightDialogueText(part.slice(2, -2), highlightDialogue, `strong-${index}`)}</strong>
    }
    return highlightDialogueText(part, highlightDialogue, `text-${index}`)
  })
}

const dialogueMarkdownComponents = {
  p: ({ children }: { children?: ReactNode }) => <p>{highlightDialogueNodes(children)}</p>,
  li: ({ children }: { children?: ReactNode }) => <li>{highlightDialogueNodes(children)}</li>,
  h1: ({ children }: { children?: ReactNode }) => <h1>{highlightDialogueNodes(children)}</h1>,
  h2: ({ children }: { children?: ReactNode }) => <h2>{highlightDialogueNodes(children)}</h2>,
  h3: ({ children }: { children?: ReactNode }) => <h3>{highlightDialogueNodes(children)}</h3>,
  h4: ({ children }: { children?: ReactNode }) => <h4>{highlightDialogueNodes(children)}</h4>,
  h5: ({ children }: { children?: ReactNode }) => <h5>{highlightDialogueNodes(children)}</h5>,
  h6: ({ children }: { children?: ReactNode }) => <h6>{highlightDialogueNodes(children)}</h6>,
  blockquote: ({ children }: { children?: ReactNode }) => <blockquote>{highlightDialogueNodes(children)}</blockquote>,
}

function highlightDialogueNodes(children: ReactNode): ReactNode {
  return Children.map(children, (child, index) => {
    if (typeof child === 'string') return highlightDialogueText(child, true, `md-${index}`)
    if (!isValidElement(child)) return child
    const props = child.props as { children?: ReactNode }
    if (props.children === undefined) return child
    return cloneElement(child, undefined, highlightDialogueNodes(props.children))
  })
}

function highlightDialogueText(text: string, enabled: boolean, keyPrefix: string): ReactNode {
  if (!enabled || !text) return text
  const nodes: ReactNode[] = []
  const pattern = /("([^"\n]+)"|“([^”\n]+)”|「([^」\n]+)」)/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  let index = 0

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) nodes.push(text.slice(lastIndex, match.index))
    nodes.push(
      <span key={`${keyPrefix}-dialogue-${index}`} className="nova-dialogue-highlight">
        {match[0]}
      </span>,
    )
    lastIndex = pattern.lastIndex
    index += 1
  }

  if (lastIndex < text.length) nodes.push(text.slice(lastIndex))
  if (nodes.length === 0) return text
  return <Fragment>{nodes}</Fragment>
}

/** 思考过程折叠块，流式思考中自动展开，结束后自动折叠。 */
function ThinkingBlock({ content, streaming }: { content: string; streaming: boolean }) {
  const [expanded, setExpanded] = useState(streaming)

  useEffect(() => {
    setExpanded(streaming)
  }, [streaming])

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <button
          type="button"
          className="flex items-center gap-1 py-1 text-xs text-[#858b96] hover:text-[#c5c9d1]"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          💭 思考过程
        </button>
        {expanded && (
          <div className="border-l border-[#303238] px-3 py-2 text-xs text-[#858b96] whitespace-pre-wrap">
            {content}
          </div>
        )}
      </div>
    </div>
  )
}
