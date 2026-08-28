import { createServer } from 'node:http'
import process from 'node:process'

const port = Number(process.env.DENOVA_E2E_MODEL_PORT || '18081')
const narrative = '石门缓缓开启，暖色灯光照亮了前方的旧车站。'
const agentEditMarker = 'E2E_EDIT_CHAPTER'
const delayedReplyMarker = 'E2E_DELAYED_AGENT_REPLY'
const agentEditArguments = JSON.stringify({
  path: 'chapters/e2e-agent-chapter.md',
  edits: [{ old_string: 'Agent 修改前。', new_string: 'Agent 已通过工具完成修改。' }],
})
const turnSubmission = JSON.stringify({
  state_changes: [
    { op: 'replace', actor_id: 'story', field_id: '当前详细地点', value: '旧车站入口' },
    { op: 'replace', actor_id: 'story', field_id: '当前事件', value: '石门已经开启，前方出现一座旧车站' },
  ],
  choices: ['走进旧车站', '留在门外观察'],
})

const delayedWaiters = new Set()

function completionFrame(delta, finishReason = '') {
  return {
    id: 'denova-e2e-response',
    object: 'chat.completion.chunk',
    created: 1,
    model: 'denova-e2e',
    choices: [{ index: 0, delta, finish_reason: finishReason }],
  }
}

function usageFrame() {
  return {
    id: 'denova-e2e-response',
    object: 'chat.completion.chunk',
    created: 1,
    model: 'denova-e2e',
    choices: [],
    usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 },
  }
}

function textCompletionFrames(content) {
  return [completionFrame({ role: 'assistant', content }, 'stop'), usageFrame()]
}

function toolCompletionFrames(name, argumentsJSON, id) {
  return [
    completionFrame({
      role: 'assistant',
      tool_calls: [{
        index: 0,
        id,
        type: 'function',
        function: { name, arguments: argumentsJSON },
      }],
    }, 'tool_calls'),
    usageFrame(),
  ]
}

function chatCompletionFrames() {
  return [
    completionFrame({ role: 'assistant', content: narrative }),
    ...toolCompletionFrames('submit_interactive_turn', turnSubmission, 'call-submit-interactive-turn'),
  ]
}

async function readJSONBody(request) {
  const chunks = []
  for await (const chunk of request) chunks.push(chunk)
  const body = Buffer.concat(chunks).toString('utf8')
  return body ? JSON.parse(body) : {}
}

function requestIncludesTool(body, toolName) {
  return Array.isArray(body.tools) && body.tools.some((tool) => tool?.function?.name === toolName)
}

function requestIncludesMarker(body, marker) {
  return JSON.stringify(body.messages ?? []).includes(marker)
}

function requestHasToolResult(body) {
  return Array.isArray(body.messages) && body.messages.some((message) => message?.role === 'tool')
}

function waitForDelayedRelease() {
  return new Promise((resolve) => delayedWaiters.add(resolve))
}

function releaseDelayedRequests() {
  for (const resolve of delayedWaiters) resolve()
  delayedWaiters.clear()
}

function writeJSON(response, status, body) {
  response.writeHead(status, { 'Content-Type': 'application/json' })
  response.end(JSON.stringify(body))
}

function writeChatCompletion(response, frames) {
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  })
  for (const frame of frames) response.write(`data: ${JSON.stringify(frame)}\n\n`)
  response.end('data: [DONE]\n\n')
}

function writeGeneratedCompletion(response, content) {
  writeJSON(response, 200, {
    id: 'denova-e2e-response',
    object: 'chat.completion',
    created: 1,
    model: 'denova-e2e',
    choices: [{ index: 0, message: { role: 'assistant', content }, finish_reason: 'stop' }],
    usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 },
  })
}

const server = createServer(async (request, response) => {
  if (request.method === 'GET' && request.url === '/health') {
    writeJSON(response, 200, { status: 'ok' })
    return
  }
  if (request.method === 'GET' && request.url === '/control/status') {
    writeJSON(response, 200, { delayed_waiting: delayedWaiters.size })
    return
  }
  if (request.method === 'POST' && request.url === '/control/release') {
    releaseDelayedRequests()
    writeJSON(response, 200, { released: true })
    return
  }
  if (request.method !== 'POST' || request.url !== '/v1/chat/completions') {
    writeJSON(response, 404, { error: 'not found' })
    return
  }

  let body
  try {
    body = await readJSONBody(request)
  } catch (error) {
    writeJSON(response, 400, { error: `invalid request body: ${error.message}` })
    return
  }

  if (body.stream !== true) {
    writeGeneratedCompletion(response, '保存核心章节内容')
    return
  }

  if (requestIncludesTool(body, 'submit_interactive_turn')) {
    writeChatCompletion(response, chatCompletionFrames())
    return
  }
  if (requestIncludesMarker(body, agentEditMarker)) {
    const frames = requestHasToolResult(body)
      ? textCompletionFrames('The requested chapter update is complete.')
      : toolCompletionFrames('edit', agentEditArguments, 'call-edit-e2e-chapter')
    writeChatCompletion(response, frames)
    return
  }
  if (requestIncludesMarker(body, delayedReplyMarker)) {
    await waitForDelayedRelease()
    writeChatCompletion(response, textCompletionFrames('Recovered response completed exactly once.'))
    return
  }
  writeChatCompletion(response, textCompletionFrames('Deterministic E2E response completed.'))
})

server.listen(port, '127.0.0.1', () => {
  console.log(`[e2e-model] listening on http://127.0.0.1:${port}`)
})

function close() {
  server.close(() => process.exit(0))
}
process.once('SIGINT', close)
process.once('SIGTERM', close)
