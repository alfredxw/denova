import { createServer } from 'node:http'
import path from 'node:path'
import process from 'node:process'
import { runtimeRoot } from './e2e-paths.mjs'
import { compactionCompletion, compactionControl } from './e2e-compaction-fixture.mjs'
import { extensionImage, extensionOpening, extensionScene } from './e2e-extension-fixture.mjs'
import { responsesRequest, responsesControl, captureNativeRequest, runtimeCompletion, writeCompletionFrame, finishCompletion } from './e2e-responses-fixture.mjs'

const port = Number(process.env.DENOVA_E2E_MODEL_PORT || '18081')
const narrative = '石门缓缓开启，暖色灯光照亮了前方的旧车站。'
const gameOpeningMarker = '[Source: story opening configuration; purpose: generate the first playable turn]'
const gameOpeningNarrative = '暮色落在旧车站外，石门后的轨道传来遥远的回声。'
const agentEditMarker = 'E2E_EDIT_CHAPTER'
const delayedReplyMarker = 'E2E_DELAYED_AGENT_REPLY'
const sessionADelayMarker = 'E2E_SESSION_A_DELAY'
const sessionBDelayMarker = 'E2E_SESSION_B_DELAY'
const sessionAFollowUpMarker = 'E2E_SESSION_A_FOLLOW_UP'
const queueReloadDelayMarker = 'E2E_QUEUE_RELOAD_DELAY'
const queueReloadFollowUpMarker = 'E2E_QUEUE_RELOAD_FOLLOW_UP'
const multiAgentDisplayMarker = 'E2E_MULTI_AGENT_DISPLAY'
const multiAgentStreamGateMarker = 'E2E_MULTI_AGENT_STREAM_GATE'
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
const externalSecretPath = path.join(runtimeRoot, 'e2e-external-secret.txt')
const agentEditArguments = JSON.stringify({
  path: 'chapters/e2e-agent-chapter.md',
  edits: [{ old_string: 'Agent 修改前。', new_string: 'Agent 已通过工具完成修改。' }],
})
const branchPlanUpdate = {
  mode: 'replace_document',
  markdown: '## 当前意图\n\n围绕 [[旧车站]] 的钟楼信号展开，但保留玩家离开车站的自由。',
}
const turnSubmissionPayload = {
  state_changes: [
    { op: 'replace', actor_id: 'story', field_id: '当前详细地点', value: '旧车站入口' },
    { op: 'replace', actor_id: 'story', field_id: '当前事件', value: '石门已经开启，前方出现一座旧车站' },
  ],
  choices: ['走进旧车站', '留在门外观察'],
}
const turnSubmission = JSON.stringify(turnSubmissionPayload)
const openingTurnSubmission = JSON.stringify({ ...turnSubmissionPayload, plan_update: branchPlanUpdate })
const planningTurnSubmission = JSON.stringify({
  state_changes: [
    { op: 'replace', actor_id: 'story', field_id: '当前详细地点', value: '旧车站站台' },
    { op: 'replace', actor_id: 'story', field_id: '当前事件', value: '发现通往钟楼的维护通道' },
  ],
  choices: ['调查维护通道', '继续查看站台地图'],
  plan_update: branchPlanUpdate,
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

function latestUserMessageIncludesMarker(body, marker) {
  const messages = Array.isArray(body.messages) ? body.messages : []
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.role === 'user') return JSON.stringify(messages[index]).includes(marker)
  }
  return false
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
  const started = toolResultMessages(body, 'send')
    .flatMap(message => parseToolResult(message).results ?? [])
    .map(result => result?.ref)
    .filter(ref => ref?.agent && ref?.session && ref?.run)
  const ready = new Set(toolResultMessages(body, 'await')
    .flatMap(message => parseToolResult(message).results ?? [])
    .filter(result => result?.ready === true)
    .map(result => taskRefKey(result?.run?.ref)))
  return started.filter(ref => !ready.has(taskRefKey(ref)))
}

function requestIncludesImageAttachment(body, name) {
  const messages = JSON.stringify(body.messages ?? [])
  const images = (body.messages ?? []).flatMap(message => Array.isArray(message.content) ? message.content : [])
    .filter(part => part.type === 'image_url' && part.image_url?.url?.startsWith('data:image/png;base64,'))
  const expected = messages.includes('E2E_TWO_IMAGES') ? 2 : 1
  return messages.includes(name) && images.length === expected
    && new Set(images.map(part => part.image_url.url)).size === expected
    && (expected === 1 || JSON.stringify(body).length > 4 * 1024 * 1024)
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
  for (const frame of frames) writeCompletionFrame(response, frame)
  finishCompletion(response)
}

async function writeGatedMultiAgentCompletion(response, child) {
  // Keep every child silent until the parent has entered await. Without
  // this handshake a fast mock response can finish its initial frames before
  // the wait subscription exists, so the test exercises timing rather than
  // the live interleaving contract.
  await waitForDelayedRelease(multiAgentStreamGateMarker)
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
  if (request.method === 'GET' && responsesControl(requestURL, response, writeJSON)) return
  if (request.method === 'GET' && compactionControl(requestURL, response, writeJSON)) return
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
  if (request.method === 'POST' && request.url === '/v1/images/generations') {
    const body = await readJSONBody(request)
    recordRequest('E2E_EXTENSION_IMAGE')
    writeJSON(response, 200, { data: [{ b64_json: extensionImage, revised_prompt: body.prompt }] })
    return
  }
  if (request.method !== 'POST' || !['/v1/chat/completions', '/v1/responses'].includes(request.url)) {
    writeJSON(response, 404, { error: 'not found' })
    return
  }

  let body
  try {
    body = await readJSONBody(request)
    if (request.url === '/v1/responses') body = responsesRequest(body, response)
    else captureNativeRequest(body)
  } catch (error) {
    writeJSON(response, 400, { error: `invalid request body: ${error.message}` })
    return
  }

  for (const marker of capturedRequestMarkers) {
    if (!capturedRequests.has(marker) && requestIncludesMarker(body, marker)) {
      capturedRequests.set(marker, body)
    }
  }

  const scene = extensionScene(body)
  if (scene) {
    recordRequest('E2E_EXTENSION_PRESENTER')
    if (body.stream === true) writeChatCompletion(response, textCompletionFrames(scene))
    else writeGeneratedCompletion(response, scene)
    return
  }
  const compaction = runtimeCompletion(body) ?? compactionCompletion(body)
  if (compaction) {
    if (body.stream !== true) writeGeneratedCompletion(response, compaction.content)
    else if (compaction.tool) writeChatCompletion(response, toolCompletionFrames(compaction.tool, compaction.arguments, compaction.id))
    else if (!compaction.summary && requestIncludesTool(body, 'submit_interactive_turn')) writeChatCompletion(response, chatCompletionFrames(compaction.content))
    else writeChatCompletion(response, textCompletionFrames(compaction.content))
    return
  }

  if (body.stream !== true) {
    writeGeneratedCompletion(response, '保存核心章节内容')
    return
  }

  if (requestIncludesMarker(body, 'E2E_IMAGE_TRANSPORT_LIMIT')) {
    response.writeHead(413, { 'Content-Type': 'application/json' })
    response.end(JSON.stringify({ error: { type: 'request_too_large', message: 'image request exceeds gateway transfer limit' } }))
    return
  }
  if (requestIncludesMarker(body, 'E2E_TOOL_IMAGE_READ')) {
    const callID = 'call-read-image-e2e'
    const hasResult = (body.messages ?? []).some(message => message.role === 'tool' && message.tool_call_id === callID)
    if (!hasResult) {
      writeChatCompletion(response, toolCompletionFrames('read', JSON.stringify({ path: 'e2e-tool-image.png' }), callID))
      return
    }
    const hasImage = (body.messages ?? []).some(message => message.role === 'user' && Array.isArray(message.content)
      && message.content.some(part => part.type === 'text' && part.text.includes(`Image from tool call "${callID}"`))
      && message.content.some(part => part.type === 'image_url' && part.image_url?.url?.startsWith('data:image/png;base64,')))
    const game = requestIncludesTool(body, 'submit_interactive_turn')
    const content = hasImage
      ? (game ? '工具读取的图片已呈现，旧车站地图上的路线清晰可见。' : 'Tool image reached the model.')
      : 'Tool image was not delivered to the model.'
    writeChatCompletion(response, game ? chatCompletionFrames(content) : textCompletionFrames(content))
    return
  }
  if (requestIncludesMarker(body, 'E2E_COMPOSER_PAUSE')) {
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache', Connection: 'keep-alive' })
    writeCompletionFrame(response, completionFrame({ role: 'assistant', content: '正在检查门后的脚印，接下来会继续核对沿途留下的线索。'.repeat(8) }))
    await waitForDelayedRelease('E2E_COMPOSER_PAUSE')
    writeCompletionFrame(response, completionFrame({ content: '检查完成。' }, 'stop'))
    finishCompletion(response)
    return
  }
  if (body.input && body.messages.at(-1)?.tool_call_id === 'call-submit-interactive-turn') {
    writeChatCompletion(response, textCompletionFrames('Turn completed.'))
    return
  }
  if (requestIncludesMarker(body, 'E2E_GAME_TURN_ORDER')) {
    const afterEarlySubmission = body.messages.at(-1)?.tool_call_id === 'call-early-submit'
    await waitForDelayedRelease(afterEarlySubmission ? 'E2E_GAME_TURN_ORDER_AFTER_TOOL' : 'E2E_GAME_TURN_ORDER_START')
    writeChatCompletion(response, afterEarlySubmission
      ? chatCompletionFrames('The gate opens after the guard leaves.')
      : toolCompletionFrames('submit_interactive_turn', turnSubmission, 'call-early-submit'))
    return
  }
  if (requestIncludesMarker(body, 'E2E_EXTENSION_OPENING') && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest('E2E_EXTENSION_OPENING')
    const opening = extensionOpening(
      toolResultMessages(body, 'initialize_story_state_schema').map(parseToolResult),
      toolResultMessages(body, 'submit_interactive_turn').map(parseToolResult),
    )
    if (opening.tool) writeChatCompletion(response, toolCompletionFrames(opening.tool, JSON.stringify(opening.input), 'call-extension-opening-schema'))
    else writeChatCompletion(response, chatCompletionFrames(opening.content, JSON.stringify(opening.submission)))
    return
  }

  if (requestIncludesMarker(body, gameBranchPlanMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest(gameBranchPlanMarker)
    writeChatCompletion(response, chatCompletionFrames(gameBranchPlanNarrative, planningTurnSubmission))
    return
  }
  if (latestUserMessageIncludesMarker(body, gameOpeningMarker) && requestIncludesTool(body, 'submit_interactive_turn')) {
    writeChatCompletion(response, chatCompletionFrames(gameOpeningNarrative, openingTurnSubmission))
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
      // End the installed runtime's retry loop deterministically. Provider
      // retry timing is independent of the product's regeneration contract.
      if (body.input) writeJSON(response, 400, { error: { message: 'Deterministic Game regeneration failure.', type: 'invalid_request_error' } })
      else writeModelError(response, 'Deterministic Game regeneration failure.')
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
    const callID = 'call-game-follow-up-read'
    if (body.input && body.messages.some(message => message.tool_call_id === callID)) {
      writeChatCompletion(response, chatCompletionFrames())
      return
    }
    recordRequest(gameFollowUpDelayMarker)
    await waitForDelayedRelease(gameFollowUpDelayMarker)
    // Native steer interrupts generation; Codex accepts it for the following
    // model step. A real host read supplies that boundary before submission.
    writeChatCompletion(response, body.input
      ? toolCompletionFrames('read', JSON.stringify({ path: 'CREATOR.md' }), callID)
      : chatCompletionFrames())
    return
  }
  if (requestIncludesMarker(body, 'E2E_PLUGIN_CHAIN')) {
    const tool = body.tools?.find(item => item.function?.name?.startsWith('plugin_'))?.function?.name
    if (!tool) {
      writeChatCompletion(response, textCompletionFrames('No plugin tools are enabled.'))
      return
    }
    const latestUser = body.messages.findLastIndex(message => message.role === 'user')
    const results = toolResultMessages({ messages: body.messages.slice(latestUser) }, tool)
    if (results.length === 0) {
      writeChatCompletion(response, toolCompletionFrames(tool, JSON.stringify({ text: 'A🌷中' }), 'call-plugin-chain'))
      return
    }
    const output = JSON.stringify(results)
    const value = JSON.parse(results.at(-1).content.split('\n\nStructured result:\n').at(-1)).value
    const content = typeof value === 'number' ? `Plugin result adopted: ${value}.` : `Unexpected plugin result: ${output}`
    if (requestIncludesMarker(body, 'E2E_PLUGIN_WRITE') && toolResultMessages(body, 'write').length === 0) {
      writeChatCompletion(response, toolCompletionFrames('write', JSON.stringify({ path: 'chapters/plugin-result.md', content: `# Plugin result\n\n${content}` }), 'call-plugin-chapter'))
      return
    }
    writeChatCompletion(response, requestIncludesTool(body, 'submit_interactive_turn') ? chatCompletionFrames(content) : textCompletionFrames(content))
    return
  }
  if (latestUserMessageIncludesMarker(body, 'E2E_GALGAME_STREAM') && requestIncludesTool(body, 'submit_interactive_turn')) {
    recordRequest('E2E_GALGAME_STREAM')
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
    response.write(`data: ${JSON.stringify(completionFrame({ role: 'assistant', content: '[lin|smile] The lamp' }))}\n\n`)
    await waitForDelayedRelease('E2E_GALGAME_STREAM')
    for (const frame of [completionFrame({ content: ' is still warm.\n[xu|neutral] Shall we read the letter?' }), ...toolCompletionFrames('submit_interactive_turn', turnSubmission, 'call-live-turn')]) response.write(`data: ${JSON.stringify(frame)}\n\n`)
    response.end('data: [DONE]\n\n')
    return
  }
  if (requestIncludesTool(body, 'submit_interactive_turn')) {
    writeChatCompletion(response, chatCompletionFrames())
    return
  }
  const multiAgentChild = multiAgentChildren.find(child => requestIncludesMarker(body, child.marker))
  if (multiAgentChild && !requestIncludesTool(body, 'send')) {
    recordRequest(multiAgentChild.marker)
    await writeGatedMultiAgentCompletion(response, multiAgentChild)
    return
  }
  if (requestIncludesMarker(body, multiAgentDisplayMarker) && requestIncludesTool(body, 'send')) {
    const taskResults = toolResultMessages(body, 'send')
    if (taskResults.length === 0) {
      const starts = multiAgentChildren.map(child => ({
        agent: 'general-purpose',
        action: 'delegate',
        message: `Return only the deterministic ${child.label} stream. ${child.marker}`,
      }))
      writeChatCompletion(response, toolCompletionFrames(
        'send',
        JSON.stringify({ items: starts }),
        'call-e2e-multi-agent-start',
      ))
      return
    }
    const remaining = remainingMultiAgentTaskRefs(body)
    if (remaining.length > 0) {
      const waitIndex = toolResultMessages(body, 'await').length + 1
      writeChatCompletion(response, toolCompletionFrames(
        'await',
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
    const completedReads = toolResultMessages(body, 'read').length
    const requiredReads = marker === externalReadAskMarker ? 3 : 1
    if (completedReads < requiredReads) {
      writeChatCompletion(response, toolCompletionFrames(
        'read',
        JSON.stringify({ path: externalSecretPath }),
        `call-external-read-${label.toLowerCase().replaceAll(' ', '-')}-${completedReads + 1}`,
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
