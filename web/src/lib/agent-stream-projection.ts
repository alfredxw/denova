import type { UIMessageChunk } from 'ai'
import type { AgentMessageMetadata, AgentUIMessage } from './agent-ui'

type Part = AgentUIMessage['parts'][number]
type ToolPart = Extract<Part, { type: 'dynamic-tool' }>

export function isAgentDataChunk(chunk: UIMessageChunk): chunk is Extract<UIMessageChunk, { type: `data-${string}` }> {
  return chunk.type.startsWith('data-')
}

/** Projects wire chunks into independent display items. Only unfinished items
 * and the current render batch are retained here; completed history belongs to
 * the UI window. Published messages are immutable and share unchanged bodies. */
export class AgentStreamProjection {
  private streamID: string = crypto.randomUUID()
  private metadata: AgentMessageMetadata = {}
  private active = new Map<string, AgentUIMessage>()
  private updates = new Map<string, AgentUIMessage>()
  private sequence = 0

  consume(chunk: UIMessageChunk) {
    if (isAgentDataChunk(chunk)) {
      if (!chunk.transient) this.publish(`data:${chunk.type}:${chunk.id || ++this.sequence}`, chunk as Part, false)
      return
    }
    switch (chunk.type) {
      case 'start':
        this.streamID = chunk.messageId || this.streamID
        this.metadata = (chunk.messageMetadata || {}) as AgentMessageMetadata
        return
      case 'message-metadata':
        this.metadata = { ...this.metadata, ...chunk.messageMetadata as AgentMessageMetadata }
        return
      case 'text-start':
      case 'reasoning-start':
        this.publish(chunk.id, { type: chunk.type === 'text-start' ? 'text' : 'reasoning', text: '', state: 'streaming', providerMetadata: chunk.providerMetadata }, true)
        return
      case 'text-delta':
      case 'reasoning-delta':
      case 'text-end':
      case 'reasoning-end': {
        const previous = this.require(chunk.id).parts[0]
        if (previous.type !== 'text' && previous.type !== 'reasoning') throw new Error(`Invalid text stream item ${chunk.id}`)
        const streaming = chunk.type === 'text-delta' || chunk.type === 'reasoning-delta'
        this.publish(chunk.id, {
          ...previous,
          text: previous.text + ('delta' in chunk ? chunk.delta : ''),
          state: streaming ? 'streaming' : 'done',
          providerMetadata: chunk.providerMetadata || previous.providerMetadata,
        }, streaming)
        return
      }
      case 'tool-input-start':
        this.publish(chunk.toolCallId, {
          type: 'dynamic-tool', toolCallId: chunk.toolCallId, toolName: chunk.toolName,
          state: 'input-streaming', input: undefined,
          callProviderMetadata: chunk.providerMetadata, toolMetadata: { ...chunk.toolMetadata, input_text: '' },
          ...(chunk.title ? { title: chunk.title } : {}),
        }, true)
        return
      case 'tool-input-delta': {
        const part = this.tool(chunk.toolCallId)
        const text = String(part.toolMetadata?.input_text || '') + chunk.inputTextDelta
        this.publish(chunk.toolCallId, { ...part, state: 'input-streaming', input: text, toolMetadata: { ...part.toolMetadata, input_text: text } } as ToolPart, true)
        return
      }
      case 'tool-input-available':
      case 'tool-input-error': {
        const previous = this.active.has(chunk.toolCallId) ? this.tool(chunk.toolCallId) : undefined
        this.publish(chunk.toolCallId, {
          ...previous, type: 'dynamic-tool', toolCallId: chunk.toolCallId, toolName: chunk.toolName,
          state: chunk.type === 'tool-input-error' ? 'output-error' : 'input-available', input: chunk.input,
          ...('errorText' in chunk ? { errorText: chunk.errorText } : {}),
          callProviderMetadata: chunk.providerMetadata || previous?.callProviderMetadata,
          toolMetadata: { ...previous?.toolMetadata, ...chunk.toolMetadata },
        } as ToolPart, chunk.type !== 'tool-input-error')
        return
      }
      case 'tool-output-available':
      case 'tool-output-error': {
        const part = this.tool(chunk.toolCallId)
        this.publish(chunk.toolCallId, {
          ...part,
          ...('output' in chunk ? { state: 'output-available', output: chunk.output, preliminary: chunk.preliminary } : { state: 'output-error', errorText: chunk.errorText }),
          resultProviderMetadata: chunk.providerMetadata,
          toolMetadata: { ...part.toolMetadata, ...chunk.toolMetadata },
        } as ToolPart, chunk.type === 'tool-output-available' && chunk.preliminary === true)
        return
      }
      case 'finish':
      case 'abort':
        this.finish()
        return
      case 'start-step':
      case 'finish-step':
        return
      case 'error':
        throw new Error(chunk.errorText)
      // These SDK features are not emitted by Denova's display encoder. Fail
      // visibly if that contract changes instead of silently dropping content.
      case 'custom':
      case 'tool-approval-request':
      case 'tool-approval-response':
      case 'tool-output-denied':
      case 'source-url':
      case 'source-document':
      case 'file':
      case 'reasoning-file':
        throw new Error(`Unsupported Agent display chunk: ${chunk.type}`)
      default: {
        const unhandled: never = chunk
        throw new Error(`Unknown Agent display chunk: ${String(unhandled)}`)
      }
    }
  }

  finish() {
    for (const [id, message] of this.active) {
      const part = message.parts[0]
      if (part.type === 'text' || part.type === 'reasoning') this.publish(id, { ...part, state: 'done' }, false)
    }
    this.active.clear()
  }

  /** Transfers the current batch; subsequent chunks cannot mutate it. */
  takeUpdates() {
    const updates = this.updates
    this.updates = new Map()
    return updates
  }

  private require(id: string) {
    const message = this.active.get(id)
    if (!message) throw new Error(`Agent display item was not started: ${id}`)
    return message
  }

  private tool(id: string): ToolPart {
    const part = this.require(id).parts[0]
    if (part.type !== 'dynamic-tool') throw new Error(`Invalid Agent tool item: ${id}`)
    return part
  }

  private publish(id: string, part: Part, active: boolean) {
    const previous = this.active.get(id)
    const message: AgentUIMessage = {
      id: previous?.id || `agent-stream:${this.streamID}:${id}`,
      role: 'assistant', metadata: previous?.metadata || this.metadata, parts: [part],
    }
    if (active) this.active.set(id, message)
    else this.active.delete(id)
    this.updates.set(message.id, message)
  }
}

export function applyAgentStreamUpdates(messages: AgentUIMessage[], updates: ReadonlyMap<string, AgentUIMessage>) {
  if (!updates.size) return messages
  const remaining = new Map(updates)
  const next = messages.map(message => {
    const update = remaining.get(message.id)
    if (!update) return message
    remaining.delete(message.id)
    return update
  })
  return [...next, ...remaining.values()]
}
