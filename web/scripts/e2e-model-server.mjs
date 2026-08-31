import { createServer } from 'node:http'
import path from 'node:path'
import process from 'node:process'

const port = Number(process.env.DENOVA_E2E_MODEL_PORT || '18081')
const narrative = '石门缓缓开启，暖色灯光照亮了前方的旧车站。'
const agentEditMarker = 'E2E_EDIT_CHAPTER'
const delayedReplyMarker = 'E2E_DELAYED_AGENT_REPLY'
const sessionADelayMarker = 'E2E_SESSION_A_DELAY'
const sessionBDelayMarker = 'E2E_SESSION_B_DELAY'
const sessionAFollowUpMarker = 'E2E_SESSION_A_FOLLOW_UP'
const queueReloadDelayMarker = 'E2E_QUEUE_RELOAD_DELAY'
const queueReloadFollowUpMarker = 'E2E_QUEUE_RELOAD_FOLLOW_UP'
const multiAgentDisplayMarker = 'E2E_MULTI_AGENT_DISPLAY'
const multiAgentChildren = [
  { marker: 'E2E_MULTI_AGENT_ALPHA', label: 'Alpha', frameDelay: 18 },
  { marker: 'E2E_MULTI_AGENT_BETA', label: 'Beta', frameDelay: 12 },
  { marker: 'E2E_MULTI_AGENT_GAMMA', label: 'Gamma', frameDelay: 6 },
]
const writingAttachmentMarker = 'E2E_WRITING_IMAGE_ATTACHMENT'
const gameAttachmentMarker = 'E2E_GAME_IMAGE_ATTACHMENT'
const gameRegenerationMarker = 'E2E_GAME_REGENERATE_FAILURE'
const gameFollowUpDelayMarker = 'E2E_GAME_FOLLOW_UP_DELAY'
const gameFollowUpMarker = 'E2E_GAME_FOLLOW_UP_STEER'
const gameBranchPlanMarker = 'E2E_GAME_BRANCH_PLAN'
const generalProjectAlphaMarker = 'E2E_GENERAL_PROJECT_ALPHA_WRITE'
const generalProjectBetaMarker = 'E2E_GENERAL_PROJECT_BETA_WRITE'
const externalReadAskMarker = 'E2E_EXTERNAL_READ_ASK'
const externalReadWriteMarker = 'E2E_EXTERNAL_READ_WRITE'
const externalReadFullMarker = 'E2E_EXTERNAL_READ_FULL_ACCESS'
const legacyWritingContinuationMarker = 'E2E_V033_WRITING_CONTINUE'
const legacyWritingHistoryMarker = '第一章保留了 v0.3.3 的正文。'
const writingPromptContextMarker = 'E2E_WRITING_PROMPT_CONTEXT_PARITY'
const gamePromptContextMarker = 'E2E_GAME_PROMPT_CONTEXT_PARITY'
const capturedRequestMarkers = [writingPromptContextMarker, gamePromptContextMarker]
const writingAttachmentName = 'writing-e2e.png'
const gameAttachmentName = 'game-e2e.png'
const gameAttachmentNarrative = '图像中的蓝色信标亮起，旧车站的侧门随之打开。'
const originalRegenerationNarrative = '第一次生成的钟声从旧车站深处传来。'
const regeneratedNarrative = '重试后，月台广播给出了全新的撤离路线。'
const gameFollowUpNarrative = '你立即改变方向，沿着新发现的脚印进入旧车站。'
const gameBranchPlanNarrative = '你在站台地图上发现一条通往钟楼的维护通道。'
const externalSecret = 'DENOVA_E2E_EXTERNAL_SECRET'
const externalSecretPath = path.resolve('test-results', 'runtime', 'e2e-external-secret.txt')
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
const planningTurnSubmission = JSON.stringify({
  state_changes: [
    { op: 'replace', actor_id: 'story', field_id: '当前详细地点', value: '旧车站站台' },
    { op: 'replace', actor_id: 'story', field_id: '当前事件', value: '发现通往钟楼的维护通道' },
  ],
  choices: ['调查维护通道', '继续查看站台地图'],
  plan_update: '## 当前意图\n\n围绕 [[旧车站]] 的钟楼信号展开，但保留玩家离开车站的自由。',
})

const delayedResponses = new Map([
  [delayedReplyMarker, 'Recovered response completed exactly once.'],
  [sessionADelayMarker, 'Session A initial response completed.'],
  [sessionBDelayMarker, 'Session B response completed independently.'],
  [queueReloadDelayMarker, 'Reloaded queue initial response completed.'],
])
const delayedWaiters = new Map()
const requestCounts = new Map()
const capturedRequests = new Map()
let gameRegenerationAllowed = false
let gameRegenerationFailureRequests = 0

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

function chatCompletionFrames(content = narrative, submission = turnSubmission) {
  return [
    completionFrame({ role: 'assistant', content }),
    ...toolCompletionFrames('submit_interactive_turn', submission, 'call-submit-interactive-turn'),
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

function requestToolResults(body) {
  return JSON.stringify((body.messages ?? []).filter((message) => message?.role === 'tool'))
}

function toolResultMessages(body, toolName) {
  const toolNamesByCallID = new Map()
  for (const message of body.messages ?? []) {
    if (message?.role !== 'assistant' || !Array.isArray(message.tool_calls)) continue
    for (const call of message.tool_calls) {
      if (call?.id && call?.function?.name) toolNamesByCallID.set(call.id, call.function.name)
    }
  }
  return (body.messages ?? []).filter((message) => (
    message?.role === 'tool' && toolNamesByCallID.get(message.tool_call_id) === toolName
  ))
}

function parseToolResult(message) {
  try {
    return JSON.parse(message?.content ?? '')
  } catch {
    return {}
  }
}

function taskRefKey(ref) {
  return `${ref?.agent ?? ''}\u001f${ref?.session ?? ''}\u001f${ref?.run ?? ''}`
}

function remainingMultiAgentTaskRefs(body) {
  const started = toolResultMessages(body, 'task')
    .flatMap(message => parseToolResult(message).results ?? [])
    .map(result => result?.task?.ref)
    .filter(ref => ref?.agent && ref?.session && ref?.run)
  const ready = new Set(toolResultMessages(body, 'task_wait')
    .flatMap(message => parseToolResult(message).results ?? [])
    .filter(result => result?.ready === true)
    .map(result => taskRefKey(result?.task?.ref)))
  return started.filter(ref => !ready.has(taskRefKey(ref)))
}

function requestIncludesImageAttachment(body, name) {
  const messages = JSON.stringify(body.messages ?? [])
  return messages.includes(name) && messages.includes('data:image/png;base64,')
}

function recordRequest(marker) {
  requestCounts.set(marker, (requestCounts.get(marker) ?? 0) + 1)
}

function waitForDelayedRelease(marker) {
  return new Promise((resolve) => {
    const waiters = delayedWaiters.get(marker) ?? new Set()
    waiters.add(resolve)
    delayedWaiters.set(marker, waiters)
  })
}

function releaseDelayedRequests(marker = '') {
  const markers = marker ? [marker] : [...delayedWaiters.keys()]
  let released = 0
  for (const current of markers) {
    const waiters = delayedWaiters.get(current)
    if (!waiters) continue
    for (const resolve of waiters) {
      resolve()
      released += 1
    }
    delayedWaiters.delete(current)
  }
  return released
}

function delayedStatus() {
  return Object.fromEntries([...delayedWaiters].map(([marker, waiters]) => [marker, waiters.size]))
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

async function writeGatedMultiAgentCompletion(response, child) {
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  })
  response.write(`data: ${JSON.stringify(completionFrame({ role: 'assistant' }))}\n\n`)
  await delay(child.frameDelay)
  response.write(`data: ${JSON.stringify(completionFrame({ reasoning_content: `${child.label} reasoning one. ` }))}\n\n`)
  await delay(child.frameDelay)
  response.write(`data: ${JSON.stringify(completionFrame({ reasoning_content: `${child.label} reasoning two.` }))}\n\n`)
  await delay(child.frameDelay)
  response.write(`data: ${JSON.stringify(completionFrame({ content: `${child.label} stream started. ` }))}\n\n`)
  await waitForDelayedRelease(child.marker)
  response.write(`data: ${JSON.stringify(completionFrame({ content: `${child.label} stream completed.` }, 'stop'))}\n\n`)
  response.write(`data: ${JSON.stringify(usageFrame())}\n\n`)
  response.end('data: [DONE]\n\n')
}

function delay(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds))
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

function writeModelError(response, message) {
  writeJSON(response, 500, {
    error: { message, type: 'server_error', code: 'denova_e2e_model_failure' },
  })
}

const server = createServer(async (request, response) => {
  const requestURL = new URL(request.url || '/', 'http://127.0.0.1')
  if (request.method === 'GET' && request.url === '/health') {
    writeJSON(response, 200, { status: 'ok' })
    return
  }
  if (request.method === 'GET' && request.url === '/control/status') {
    const delayedWaitingByMarker = delayedStatus()
    writeJSON(response, 200, {
      delayed_waiting: Object.values(delayedWaitingByMarker).reduce((total, count) => total + count, 0),
      delayed_waiting_by_marker: delayedWaitingByMarker,
      request_counts: Object.fromEntries(requestCounts),
      game_regeneration_allowed: gameRegenerationAllowed,
      game_regeneration_failure_requests: gameRegenerationFailureRequests,
      external_secret_path: externalSecretPath,
    })
    return
  }
  if (request.method === 'GET' && requestURL.pathname === '/control/captured-request') {
    const marker = requestURL.searchParams.get('marker') || ''
    const captured = capturedRequests.get(marker)
    if (!captured) {
      writeJSON(response, 404, { captured: false })
      return
    }
    writeJSON(response, 200, captured)
    return
  }
  if (request.method === 'POST' && request.url === '/control/release') {
    const body = await readJSONBody(request)
    const released = releaseDelayedRequests(typeof body.marker === 'string' ? body.marker : '')
    writeJSON(response, 200, { released })
    return
  }
  if (request.method === 'POST' && request.url === '/control/allow-game-regeneration') {
    gameRegenerationAllowed = true
    writeJSON(response, 200, { allowed: true })
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

  for (const marker of capturedRequestMarkers) {
    if (!capturedRequests.has(marker) && requestIncludesMarker(body, marker)) {
      capturedRequests.set(marker, body)
    }
  }

  if (body.stream !== true) {
    writeGeneratedCompletion(response, '保存核心章节内容')
    return
  }

  if (requestIncludesMarker(body, gameBranchPlanMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameBranchPlanMarker)
    writeChatCompletion(response, chatCompletionFrames(gameBranchPlanNarrative, planningTurnSubmission))
    return
  }
  if (requestIncludesMarker(body, gameAttachmentMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameAttachmentMarker)
    const content = requestIncludesImageAttachment(body, gameAttachmentName)
      ? gameAttachmentNarrative
      : 'Game image attachment was not delivered to the model.'
    writeChatCompletion(response, chatCompletionFrames(content))
    return
  }
  if (requestIncludesMarker(body, gameRegenerationMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameRegenerationMarker)
    if ((requestCounts.get(gameRegenerationMarker) ?? 0) === 1) {
      writeChatCompletion(response, chatCompletionFrames(originalRegenerationNarrative))
      return
    }
    if (!gameRegenerationAllowed) {
      gameRegenerationFailureRequests += 1
      writeModelError(response, 'Deterministic Game regeneration failure.')
      return
    }
    writeChatCompletion(response, chatCompletionFrames(regeneratedNarrative))
    return
  }
  if (requestIncludesMarker(body, gameFollowUpMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameFollowUpMarker)
    writeChatCompletion(response, chatCompletionFrames(gameFollowUpNarrative))
    return
  }
  if (requestIncludesMarker(body, gameFollowUpDelayMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameFollowUpDelayMarker)
    await waitForDelayedRelease(gameFollowUpDelayMarker)
    writeChatCompletion(response, chatCompletionFrames())
    return
  }
  if (requestIncludesTool(body, 'submit_interactive_turn')) {
    writeChatCompletion(response, chatCompletionFrames())
    return
  }
  const multiAgentChild = multiAgentChildren.find(child => requestIncludesMarker(body, child.marker))
  if (multiAgentChild && !requestIncludesTool(body, 'task')) {
    recordRequest(multiAgentChild.marker)
    await writeGatedMultiAgentCompletion(response, multiAgentChild)
    return
  }
  if (requestIncludesMarker(body, multiAgentDisplayMarker) && requestIncludesTool(body, 'task')) {
    const taskResults = toolResultMessages(body, 'task')
    if (taskResults.length === 0) {
      const starts = multiAgentChildren.map(child => ({
        agent: 'general-purpose',
        prompt: `Return only the deterministic ${child.label} stream. ${child.marker}`,
      }))
      writeChatCompletion(response, toolCompletionFrames(
        'task',
        JSON.stringify({ action: 'start', starts }),
        'call-e2e-multi-agent-start',
      ))
      return
    }
    const remaining = remainingMultiAgentTaskRefs(body)
    if (remaining.length > 0) {
      const waitIndex = toolResultMessages(body, 'task_wait').length + 1
      writeChatCompletion(response, toolCompletionFrames(
        'task_wait',
        JSON.stringify({ targets: remaining.map(ref => ({ ref })) }),
        `call-e2e-multi-agent-wait-${waitIndex}`,
      ))
      return
    }
    writeChatCompletion(response, textCompletionFrames('All three delegated results completed.'))
    return
  }
  if (requestIncludesMarker(body, sessionAFollowUpMarker)) {
    recordRequest(sessionAFollowUpMarker)
    writeChatCompletion(response, textCompletionFrames('Session A follow-up reached only Session A.'))
    return
  }
  if (requestIncludesMarker(body, queueReloadFollowUpMarker)) {
    recordRequest(queueReloadFollowUpMarker)
    writeChatCompletion(response, textCompletionFrames('Reloaded queued follow-up completed exactly once.'))
    return
  }
  for (const [marker, content] of delayedResponses) {
    if (!requestIncludesMarker(body, marker)) continue
    recordRequest(marker)
    await waitForDelayedRelease(marker)
    writeChatCompletion(response, textCompletionFrames(content))
    return
  }
  for (const [marker, content] of [
    [generalProjectAlphaMarker, 'alpha-project-only'],
    [generalProjectBetaMarker, 'beta-project-only'],
  ]) {
    if (!requestIncludesMarker(body, marker)) continue
    recordRequest(marker)
    const frames = requestHasToolResult(body)
      ? textCompletionFrames(`General Project write completed: ${content}.`)
      : toolCompletionFrames('write', JSON.stringify({ path: 'e2e-project-proof.txt', content }), `call-${content}`)
    writeChatCompletion(response, frames)
    return
  }
  if (requestIncludesMarker(body, agentEditMarker)) {
    const frames = requestHasToolResult(body)
      ? textCompletionFrames('The requested chapter update is complete.')
      : toolCompletionFrames('edit', agentEditArguments, 'call-edit-e2e-chapter')
    writeChatCompletion(response, frames)
    return
  }
  for (const [marker, label] of [
    [externalReadAskMarker, 'Ask'],
    [externalReadWriteMarker, 'Write'],
    [externalReadFullMarker, 'Full access'],
  ]) {
    if (!requestIncludesMarker(body, marker)) continue
    recordRequest(marker)
    if (!requestHasToolResult(body)) {
      writeChatCompletion(response, toolCompletionFrames(
        'read',
        JSON.stringify({ path: externalSecretPath }),
        `call-external-read-${label.toLowerCase().replaceAll(' ', '-')}`,
      ))
      return
    }
    const toolResults = requestToolResults(body)
    const content = toolResults.includes(externalSecret)
      ? `External read completed in ${label} mode.`
      : `External read was denied in ${label} mode.`
    writeChatCompletion(response, textCompletionFrames(content))
    return
  }
  if (requestIncludesMarker(body, writingAttachmentMarker)) {
    recordRequest(writingAttachmentMarker)
    const content = requestIncludesImageAttachment(body, writingAttachmentName)
      ? 'Writing image attachment reached the model.'
      : 'Writing image attachment was not delivered to the model.'
    writeChatCompletion(response, textCompletionFrames(content))
    return
  }
  if (requestIncludesMarker(body, legacyWritingContinuationMarker)) {
    const content = requestIncludesMarker(body, legacyWritingHistoryMarker)
      ? 'v0.3.3 写作历史续聊成功。'
      : 'v0.3.3 写作历史未传入模型。'
    writeChatCompletion(response, textCompletionFrames(content))
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
