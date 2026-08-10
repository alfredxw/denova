import { CheckCircle2, Circle, CircleDot, ListTodo } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ToolCallChatMessage } from '@/lib/api'
import { stripToolResultMetadata } from './message-tool'

interface TodoItem {
  step: string
  status: 'pending' | 'in_progress' | 'completed' | string
}

/** Tolerates partial streamed arguments, then trusts the structured final result. */
export function TodoListBlock({ message }: { message: ToolCallChatMessage }) {
  const { t } = useTranslation()
  const args = message.args || ''
  const status = message.status || 'running'
  const resultPlan = status === 'success' ? parseTodoPlanResult(stripToolResultMetadata(message.result || '')) : null
  const todos = resultPlan ?? parseTodoPlanFromArgs(args)
  const total = todos.length
  const completed = todos.filter(t => t.status === 'completed').length
  const inProgress = todos.find(t => t.status === 'in_progress')
  const headline = inProgress?.step || (status === 'success' ? t('chat.todo.updated') : t('chat.todo.updating'))

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex min-h-10 min-w-0 items-center gap-2 px-3 py-2">
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            <ListTodo className="h-3.5 w-3.5" />
          </span>
          <span className="shrink-0 font-medium text-[var(--nova-text)]">{t('chat.todo.list')}</span>
          {total > 0 && (
            <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-faint)]">
              {completed}/{total}
            </span>
          )}
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{headline}</span>
        </div>
        {todos.length > 0 && (
          <ul className="grid gap-1 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
            {todos.map((todo, index) => (
              <TodoListItem key={index} todo={todo} />
            ))}
          </ul>
        )}
        {todos.length === 0 && (
          <div className="border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 text-[var(--nova-text-faint)]">
            {status === 'running' ? t('chat.todo.parsing') : t('chat.todo.empty')}
          </div>
        )}
      </div>
    </div>
  )
}

function TodoListItem({ todo }: { todo: TodoItem }) {
  const text = todo.step
  if (todo.status === 'completed') {
    return (
      <li className="flex items-start gap-2 rounded-md px-2 py-1.5 leading-5">
        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-green)]" />
        <span className="text-[var(--nova-text-faint)] line-through">{text}</span>
      </li>
    )
  }
  if (todo.status === 'in_progress') {
    return (
      <li className="flex items-start gap-2 rounded-md border border-[var(--nova-border)] bg-[var(--nova-hover)] px-2 py-1.5 leading-5">
        <CircleDot className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-pulse text-[var(--nova-text)]" />
        <span className="text-[var(--nova-text)]">{text}</span>
      </li>
    )
  }
  return (
    <li className="flex items-start gap-2 rounded-md px-2 py-1.5 leading-5">
      <Circle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
      <span className="text-[var(--nova-text-muted)]">{text}</span>
    </li>
  )
}

function parseTodoPlanResult(result: string): TodoItem[] | null {
  if (!result) return null
  try {
    const data = JSON.parse(result) as { schema?: string; plan?: TodoItem[] }
    if (data.schema === 'todo.plan.v1' && Array.isArray(data.plan)) return data.plan
  } catch {
    // Unstructured or incomplete output is not an authoritative success result.
  }
  return null
}

/** Extracts completed Todo entries from potentially partial streamed JSON. */
function parseTodoPlanFromArgs(args: string): TodoItem[] {
  if (!args) return []
  const trimmed = args.trim()
  if (!trimmed) return []
  // Prefer the complete payload when it is already available.
  try {
    const data = JSON.parse(trimmed) as { plan?: TodoItem[] }
    if (Array.isArray(data?.plan)) return data.plan
  } catch {
    // Partial or truncated arguments are expected during streaming.
  }
  // Fall back to completed objects already present in the plan array.
  const arrayMatch = trimmed.match(/"plan"\s*:\s*\[([\s\S]*)$/)
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
          // A partial object does not invalidate earlier completed entries.
        }
        start = -1
      }
    }
  }
  return items
}
