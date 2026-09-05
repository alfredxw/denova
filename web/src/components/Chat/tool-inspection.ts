import { applyEdits, format, visit } from 'jsonc-parser'
import type { InspectableToolMessage } from './ToolInspector'
import { decodeToolResultEnvelope } from '@/lib/tool-result-envelope'

/** Whitespace-only formatting preserves large integers, duplicate keys and escapes. */
export function formatToolJSON(source: string): { text: string; language: 'json' | 'plaintext' } {
  let invalid = !source.trim()
  visit(source, { onError: () => { invalid = true } }, { disallowComments: true, allowTrailingComma: false })
  if (invalid) return { text: source, language: 'plaintext' }
  return {
    text: applyEdits(source, format(source, undefined, { tabSize: 2, insertSpaces: true, eol: '\n' })),
    language: 'json',
  }
}

/** Inspection shows recorded display data, never reconstructs a tool call from an Ask. */
export function inspectToolMessage(message: InspectableToolMessage) {
  const interaction = 'ask' in message ? message.ask : undefined
  const name = message.role === 'ask'
    ? interaction?.approval?.tool_name || 'ask'
    : message.name || (message.role === 'tool_call' ? message.content?.split('\n')[0] : '') || 'unknown_tool'
  const input = message.role === 'tool_call'
    ? message.args ?? message.content?.split('\n').slice(1).join('\n')
    : undefined
  const output = message.role === 'ask'
    ? undefined
    : message.result ?? (message.role === 'tool_result' ? message.content : undefined)
  const envelope = decodeToolResultEnvelope(output || '')
  return {
    name,
    callId: interaction?.tool_call_id || message.id,
    input,
    output,
    inputPreview: message.role === 'tool_call' && message.args_preview === true,
    interaction,
    truncated: (message.role !== 'ask' && message.result_truncated === true)
      || envelope?.truncated === true || (output || '').endsWith('\n[tool result truncated]'),
  }
}
