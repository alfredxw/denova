// Only the model is deterministic. Tests exercise the real browser, backend,
// tool router, context maintenance, and canonical journals.
const runs = new Map()

export function compactionControl(requestURL, response, writeJSON) {
  if (requestURL.pathname !== '/control/compaction-requests') return false
  writeJSON(response, 200, runs.get(requestURL.searchParams.get('marker')) ?? [])
  return true
}

export function compactionCompletion(body) {
  const serialized = JSON.stringify(body.messages ?? [])
  const marker = ['E2E_COMPACTION_WRITING', 'E2E_COMPACTION_GAME'].find(value => serialized.includes(value))
  if (!marker) return null
  const captured = runs.get(marker) ?? []
  captured.push(body)
  runs.set(marker, captured)
  const lastUser = [...(body.messages ?? [])].reverse().find(message => message.role === 'user')
  const input = JSON.stringify(lastUser?.content ?? '')
  if (input.includes('[Denova runtime context compaction request]') || body.stream !== true) {
    return { content: `${marker} checkpoint: Preserve the archive facts and continue the current task using live evidence.`, summary: true }
  }
  if (input.includes(`${marker}_SEED`)) {
    const paragraphs = marker.endsWith('GAME') ? 4000 : 4800
    return { content: `${marker} archive\n\n${'Historical archive detail about the station. '.repeat(paragraphs)}\n\n${marker} seed complete.` }
  }
  if (input.includes(`${marker}_PAD`)) return { content: `${marker} recent turn complete.` }
  if (input.includes(`${marker}_READ`)) {
    const id = `live-${marker}`
    const result = body.messages.some(message => message.role === 'tool' && message.tool_call_id === id)
    if (!result) return { tool: 'read', arguments: JSON.stringify({ path: 'compaction-evidence.txt', limit: 2000 }), id }
    return { content: `${marker} live evidence accepted.` }
  }
  return { content: `${marker} continued after reload.` }
}
