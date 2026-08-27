import { createServer } from 'node:http'
import process from 'node:process'

const port = Number(process.env.DENOVA_E2E_MODEL_PORT || '18081')
const narrative = '石门缓缓开启，暖色灯光照亮了前方的旧车站。'
const turnSubmission = JSON.stringify({
  state_changes: [
    { op: 'replace', actor_id: 'story', field_id: '当前详细地点', value: '旧车站入口' },
    { op: 'replace', actor_id: 'story', field_id: '当前事件', value: '石门已经开启，前方出现一座旧车站' },
  ],
  choices: ['走进旧车站', '留在门外观察'],
})

function chatCompletionFrames() {
  return [
    {
      id: 'denova-e2e-response',
      object: 'chat.completion.chunk',
      created: 1,
      model: 'denova-e2e',
      choices: [{ index: 0, delta: { role: 'assistant', content: narrative }, finish_reason: '' }],
    },
    {
      id: 'denova-e2e-response',
      object: 'chat.completion.chunk',
      created: 1,
      model: 'denova-e2e',
      choices: [{
        index: 0,
        delta: {
          tool_calls: [{
            index: 0,
            id: 'call-submit-interactive-turn',
            type: 'function',
            function: { name: 'submit_interactive_turn', arguments: turnSubmission },
          }],
        },
        finish_reason: 'tool_calls',
      }],
    },
    {
      id: 'denova-e2e-response',
      object: 'chat.completion.chunk',
      created: 1,
      model: 'denova-e2e',
      choices: [],
      usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 },
    },
  ]
}

const server = createServer(async (request, response) => {
  if (request.method === 'GET' && request.url === '/health') {
    response.writeHead(200, { 'Content-Type': 'application/json' })
    response.end('{"status":"ok"}')
    return
  }
  if (request.method !== 'POST' || request.url !== '/v1/chat/completions') {
    response.writeHead(404, { 'Content-Type': 'application/json' })
    response.end('{"error":"not found"}')
    return
  }

  for await (const _chunk of request) {
    // Drain the request before replying so connection reuse stays deterministic.
  }
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  })
  for (const frame of chatCompletionFrames()) response.write(`data: ${JSON.stringify(frame)}\n\n`)
  response.end('data: [DONE]\n\n')
})

server.listen(port, '127.0.0.1', () => {
  console.log(`[e2e-model] listening on http://127.0.0.1:${port}`)
})

function close() {
  server.close(() => process.exit(0))
}
process.once('SIGINT', close)
process.once('SIGTERM', close)
