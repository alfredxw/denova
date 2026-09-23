import { randomUUID } from 'node:crypto'

// Share deterministic model scenarios across Native and the installed Codex
// executable. Only the model wire format is adapted; product APIs and runtime
// control, tools, persistence, and compaction all execute their real code.
const streams = new WeakMap()
const requests = []

export function responsesRequest(body, response) {
  requests.push(body)
  streams.set(response, { id: `resp_${randomUUID()}`, output: [], text: null, started: false, inputTokens: 10 })
  const messages = body.instructions ? [{ role: 'system', content: body.instructions }] : []
  for (const item of body.input ?? []) {
    if (item.type === 'function_call') {
      messages.push({ role: 'assistant', tool_calls: [{ id: item.call_id, type: 'function', function: { name: item.name, arguments: item.arguments } }] })
    } else if (item.type === 'function_call_output') {
      messages.push({ role: 'tool', tool_call_id: item.call_id, content: typeof item.output === 'string' ? item.output : JSON.stringify(item.output) })
    } else if (item.role) {
      messages.push({ role: item.role, content: Array.isArray(item.content) ? item.content.map(part => (
        part.type === 'input_image' ? { type: 'image_url', image_url: { url: part.image_url } } : { type: 'text', text: part.text ?? '' }
      )) : item.content })
    }
  }
  return { ...body, messages, tools: (body.tools ?? []).map(tool => ({ type: tool.type, function: tool })), stream: true }
}

export function responsesControl(url, response, writeJSON) {
  if (url.pathname !== '/control/runtime-requests') return false
  const marker = url.searchParams.get('marker') ?? ''
  writeJSON(response, 200, requests.filter(body => JSON.stringify(body).includes(marker)))
  return true
}

export function captureNativeRequest(body) {
  if (JSON.stringify(body.messages).includes('E2E_RUNTIME_')) requests.push(body)
}

export function runtimeCompletion(body) {
  const serialized = JSON.stringify(body.messages ?? [])
  const marker = serialized.match(/E2E_RUNTIME_(?:AUTO|PLAN|SWITCH|GOAL)_(?:WRITING|GAME)/)?.[0]
  if (!marker) return null
  const lastUser = [...body.messages].reverse().find(message => message.role === 'user')
  const input = JSON.stringify(lastUser?.content)
  if (input.includes('CONTEXT CHECKPOINT COMPACTION')) return { content: `${marker} checkpoint: read evidence verified.`, summary: true }
  if (input.includes('[Goal evaluation request]')) {
    const completed = serialized.includes(`${marker} final proof.`)
    return { content: JSON.stringify({ verdict: completed ? 'complete' : 'continue', reason: completed ? 'Both requested proofs are present.' : 'The final proof is still required.', next_instruction: completed ? '' : `${marker} FINISH_GOAL` }), summary: true }
  }
  if (marker.includes('_GOAL_')) return { content: `${marker} ${input.includes('FINISH_GOAL') ? 'final' : 'first'} proof.` }
  if (marker.includes('_SWITCH_')) {
    const step = input.match(/STEP_(\d)/)?.[1] ?? '1'
    const previous = Number(step) - 1
    if (previous > 0 && !serialized.includes(`${marker} reply ${previous}.`)) return { content: 'Prior runtime history is missing.' }
    return { content: `${marker} reply ${step}.` }
  }
  if (input.includes('CONTINUE_AFTER_COMPACT')) return { content: `${marker} continued.` }
  if (marker.includes('_PLAN_')) {
    if (!body.messages.some(message => message.tool_call_id === 'call-native-plan')) return {
      tool: 'update_plan', id: 'call-native-plan', arguments: JSON.stringify({ plan: [{ step: 'Verify the station evidence', status: 'completed' }] }),
    }
    return { content: `${marker} planned.` }
  }
  if (!serialized.includes('read evidence verified') && !body.messages.some(message => message.tool_call_id === 'call-auto-compact-read')) return {
    tool: 'read', id: 'call-auto-compact-read', arguments: JSON.stringify({ path: 'runtime-evidence.txt' }),
  }
  return { content: `${marker} completed after compaction.` }
}

function emit(response, event) {
  response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`)
}

export function writeCompletionFrame(response, frame) {
  const stream = streams.get(response)
  if (!stream) {
    response.write(`data: ${JSON.stringify(frame)}\n\n`)
    return
  }
  if (!stream.started) {
    stream.started = true
    emit(response, { type: 'response.created', response: { id: stream.id, object: 'response', status: 'in_progress', output: [] } })
  }
  const delta = frame.choices?.[0]?.delta
  if (delta?.content) {
    if (!stream.text) {
      stream.text = { type: 'message', id: `msg_${randomUUID()}`, role: 'assistant', status: 'in_progress', content: [{ type: 'output_text', text: '', annotations: [] }] }
      stream.output.push(stream.text)
      emit(response, { type: 'response.output_item.added', output_index: stream.output.length - 1, item: stream.text })
    }
    stream.text.content[0].text += delta.content
    emit(response, { type: 'response.output_text.delta', output_index: stream.output.indexOf(stream.text), content_index: 0, item_id: stream.text.id, delta: delta.content })
  }
  for (const call of delta?.tool_calls ?? []) {
    // Report a full context only at this deterministic tool boundary. The
    // installed runtime, not the test backend, decides to compact and emits
    // the actual protocol event consumed by Denova.
    if (call.id === 'call-auto-compact-read') stream.inputTokens = 90000
    const item = { type: 'function_call', id: `fc_${randomUUID()}`, call_id: call.id, name: call.function.name, arguments: call.function.arguments, status: 'completed' }
    stream.output.push(item)
    emit(response, { type: 'response.output_item.added', output_index: stream.output.length - 1, item })
  }
}

export function finishCompletion(response) {
  const stream = streams.get(response)
  if (!stream) {
    response.end('data: [DONE]\n\n')
    return
  }
  stream.output.forEach((item, index) => {
    item.status = 'completed'
    emit(response, { type: 'response.output_item.done', output_index: index, item })
  })
  emit(response, { type: 'response.completed', response: { id: stream.id, object: 'response', status: 'completed', output: stream.output, usage: { input_tokens: stream.inputTokens, output_tokens: 10, total_tokens: stream.inputTokens + 10 } } })
  response.end()
}
