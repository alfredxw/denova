import type { ContextAnalysis } from '../../src/lib/api'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { getCapturedModelRequest, type E2EModelRequest } from '../support/model'

const contextAnalysisProbe = '[Denova context analysis probe]'
const writingFlowMarker = 'E2E_WRITING_PROMPT_FLOW_PARITY'
const writingContextMarker = 'E2E_WRITING_PROMPT_CONTEXT_PARITY'
const gameFlowMarker = 'E2E_GAME_PROMPT_FLOW_PARITY'
const gameContextMarker = 'E2E_GAME_PROMPT_CONTEXT_PARITY'

test('keeps Agents prompts, context analysis, and real model input aligned for Writing and Game', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Agent Prompt Context E2E Book')
  const loreBase = `/api/projects/${book.projectId}/book/lore`
  const index = await (await request.get(`${loreBase}/index`)).json()
  const guide = { intro_markdown: 'E2E_LORE_INTRO', groups: [
    { id: 'core', name: 'Core', purpose: 'Global rules', body_markdown: 'E2E_LORE_ALWAYS_NOTES', default_detail: 'full' },
    { id: 'lazy', name: 'Harbor', purpose: 'Explore harbor material', body_markdown: 'E2E_LORE_DEFERRED_NOTES', default_detail: 'brief' },
  ], group_order: ['automatic:auto:character', 'custom:lazy', 'custom:core'] }
  expect((await request.put(`${loreBase}/index`, { data: { guide, base_revision: index.revision } })).ok()).toBe(true)
  for (const [name, group, content] of [['Core canon', 'core', 'E2E_LORE_ALWAYS_BODY'], ['Harbor lore', 'lazy', 'E2E_LORE_DEFERRED_BODY']]) {
    expect((await request.post(`${loreBase}/items`, { data: { id: `internal-lore-identity-${group}`, name, content, load_mode: 'auto', brief_description: 'A setting entry', index_memberships: [{ group_id: group, detail: 'inherit' }] } })).ok()).toBe(true)
  }
  expect((await request.post(`${loreBase}/items`, { data: { id: 'unassigned-lore-internal', name: 'Unassigned scout', type: 'character', importance: 'major', tags: ['E2E_HIDDEN_INDEX_TAG'], load_mode: 'auto', content: 'UNASSIGNED_BODY' } })).ok()).toBe(true)
  for (const data of [
    { id: 'shared-lore-internal', name: 'Shared navigator', type: 'character', importance: 'minor', content: 'E2E_SHARED_BODY', brief_description: 'E2E_SHARED_BRIEF', index_memberships: [{ group_id: 'lazy', detail: 'inherit' }, { group_id: 'core', detail: 'inherit' }] },
    { id: 'empty-lore-internal', name: 'Empty canon', type: 'world', content: '', brief_description: 'E2E_EMPTY_CANON_BRIEF', index_memberships: [{ group_id: 'core', detail: 'inherit' }] },
  ]) expect((await request.post(`${loreBase}/items`, { data: { load_mode: 'auto', ...data } })).ok()).toBe(true)
  const preview = await (await request.post(`${loreBase}/index/preview`, { data: guide })).json()
  expect(preview.markdown).toContain('- **Empty canon** (Worldbuilding / important / brief): E2E_EMPTY_CANON_BRIEF')
  await createStartedStory(request, 'Agent Prompt Context E2E Story')

  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: 'Agents', exact: true }).click()
  await expect(page.getByRole('heading', { name: '写作 Agent', exact: true })).toBeVisible()
  await page.getByRole('radio', { name: '当前工作区', exact: true }).click()

  const writingPrompt = await configureAgentPrompt(page, '写作 Agent', writingFlowMarker, writingContextMarker)
  const gamePrompt = await configureAgentPrompt(page, '游戏 Agent', gameFlowMarker, gameContextMarker)

  await expect.poll(async () => projectWorkspacePrompts(request, book.projectId)).toEqual({
    ide: writingPrompt,
    interactive_story: gamePrompt,
  })
  await expect(page.getByRole('status', { name: '所有更改均已保存' })).toBeVisible()

  const writingComposer = await openWritingAgent(page)
  const writingAnalysis = await openContextAnalysis(page, '/agent-chat/chat/context-analysis')
  await page.keyboard.press('Escape')
  await submitAgentChatMessage(page, writingComposer, contextAnalysisProbe)
  await expect(page.getByText('Deterministic E2E response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
  const writingRequest = await waitForCapturedRequest(request, writingContextMarker)
  expectPromptContextParity(writingAnalysis, writingRequest, writingPrompt)
  expectLoreGuide(writingRequest)

  await sidebar.getByRole('button', { name: '游戏', exact: true }).click()
  const gameComposer = page.getByPlaceholder(/你要做什么/)
  await expect(gameComposer).toBeVisible()
  const gameAnalysis = await openContextAnalysis(page, '/api/interactive/chat/context-analysis')
  await page.keyboard.press('Escape')
  await submitAgentChatMessage(page, gameComposer, contextAnalysisProbe)
  await expect(page.getByText('石门缓缓开启，暖色灯光照亮了前方的旧车站。', { exact: true })).toBeVisible()
  const gameRequest = await waitForCapturedRequest(request, gameContextMarker)
  expectPromptContextParity(gameAnalysis, gameRequest, gamePrompt)
  expectLoreGuide(gameRequest)
})

interface PromptConfiguration {
  flow_prompt: string
  system_prompt: string
}

interface ComparableModelMessage {
  role: string
  content: string
}

async function configureAgentPrompt(
  page: Page,
  agentTitle: string,
  flowMarker: string,
  contextMarker: string,
): Promise<PromptConfiguration> {
  await page.getByRole('button', { name: new RegExp(`^${agentTitle}`) }).click()
  await expect(page.getByRole('heading', { name: agentTitle, exact: true })).toBeVisible()

  await page.getByRole('button', { name: '内置流程与参考', exact: true }).click()
  return {
    flow_prompt: await appendPromptMarker(page, '流程规则', flowMarker),
    system_prompt: await appendPromptMarker(page, '用户自定义', contextMarker),
  }
}

async function appendPromptMarker(page: Page, title: string, marker: string): Promise<string> {
  const input = page.getByRole('textbox', { name: title, exact: true })
  if (await input.count() === 0) {
    await page.getByRole('button', { name: new RegExp(title) }).click()
  }
  await expect(input).toBeVisible()
  const current = (await input.inputValue()).trim()
  const next = current ? `${current}\n\n${marker}` : marker
  await input.fill(next)
  return next
}

async function projectWorkspacePrompts(
  request: APIRequestContext,
  projectId: string,
): Promise<Record<string, PromptConfiguration | undefined>> {
  const response = await request.get(`/api/projects/${encodeURIComponent(projectId)}/settings`)
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
  const snapshot = await response.json() as {
    workspace?: { agent_prompts?: Record<string, PromptConfiguration | undefined> }
  }
  return {
    ide: snapshot.workspace?.agent_prompts?.ide,
    interactive_story: snapshot.workspace?.agent_prompts?.interactive_story,
  }
}

async function openContextAnalysis(page: Page, endpoint: string): Promise<ContextAnalysis> {
  const responsePromise = page.waitForResponse((response) => (
    response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith(endpoint)
  ))
  await page.getByRole('button', { name: '输入动作' }).filter({ visible: true }).click()
  await page.getByRole('menuitem', { name: '上下文分析', exact: true }).click()
  const response = await responsePromise
  const failureDetails = response.ok() ? undefined : await response.text()
  expect(response.ok(), failureDetails).toBe(true)
  await expect(page.getByRole('dialog', { name: '上下文分析' })).toBeVisible()
  return response.json()
}

async function waitForCapturedRequest(request: APIRequestContext, marker: string): Promise<E2EModelRequest> {
  await expect.poll(async () => getCapturedModelRequest(request, marker)).not.toBeNull()
  const captured = await getCapturedModelRequest(request, marker)
  if (captured === null) {
    throw new Error(`Model request with marker ${marker} disappeared after capture`)
  }
  return captured
}

function expectPromptContextParity(
  analysis: ContextAnalysis,
  modelRequest: E2EModelRequest,
  configured: PromptConfiguration,
): void {
  const messages = modelRequest.messages.map(comparableCapturedMessage)
  const systemPrompt = messages
    .filter((message) => message.role === 'system')
    .map((message) => message.content)
    .join('\n\n')
  const contextMessages = messages.filter((message) => message.role !== 'system')

  expect(systemPrompt).toBe(analysis.system_prompt)
  expect(normalizeRuntimeCapture(contextMessages)).toEqual(
    normalizeRuntimeCapture(
      analysis.context_messages.map((message) => ({ role: message.role, content: message.content })),
    ),
  )
  expect(analysis.system_prompt_parts.find((part) => part.id === 'builtin_base')?.content)
    .toBe(configured.flow_prompt)
  expect(analysis.system_prompt_parts.find((part) => part.id === 'agent_custom_rules')?.content)
    .toBe(`\n\n## Agent Custom Rules\n\n${configured.system_prompt}`)
}

function comparableCapturedMessage(message: E2EModelRequest['messages'][number]): ComparableModelMessage {
  const toolCalls = message.tool_calls || []
  if (toolCalls.length === 0) {
    expect(typeof message.content).toBe('string')
    return { role: message.role, content: message.content as string }
  }

  const renderedCalls = toolCalls.map((call, index) => {
    const name = call.function?.name?.trim() || 'unknown_tool'
    const id = call.id?.trim()
    const suffix = id ? ` (id: ${id})` : ''
    return `${index + 1}. ${name}${suffix}\narguments:\n${call.function?.arguments?.trim() || ''}`
  }).join('\n')
  const renderedToolCalls = `[工具调用]\n${renderedCalls}`
  const content = typeof message.content === 'string' ? message.content : ''
  return {
    role: message.role,
    content: content.trim() ? `${content.replace(/\n+$/, '')}\n\n${renderedToolCalls}` : renderedToolCalls,
  }
}

function normalizeRuntimeCapture(messages: ComparableModelMessage[]): ComparableModelMessage[] {
  return messages.map((message) => ({
    ...message,
    content: message.content.replace(
      /^- Captured at: [^\r\n]+$/m,
      '- Captured at: <turn timestamp>',
    ),
  }))
}

function expectLoreGuide(request: E2EModelRequest) {
  const content = request.messages.map(message => typeof message.content === 'string' ? message.content : '').join('\n')
  for (const marker of ['# Lore Index', 'E2E_LORE_INTRO', 'E2E_LORE_ALWAYS_NOTES', 'E2E_LORE_ALWAYS_BODY', 'E2E_LORE_DEFERRED_NOTES', 'Harbor lore', 'Explore harbor material', 'group_names', 'Unassigned scout', 'Character · On demand']) expect(content).toContain(marker)
  for (const marker of [
    '- Unassigned scout (Character / major / name)',
    '- **Harbor lore** (Worldbuilding / important / brief): A setting entry',
    '### Core canon (Worldbuilding / important / full)',
    '### Shared navigator (Character / minor / full)',
    '- Shared navigator (included above)',
    '- **Empty canon** (Worldbuilding / important / brief): E2E_EMPTY_CANON_BRIEF',
    'Entry metadata is category / importance / detail.',
  ]) expect(content).toContain(marker)
  for (const marker of ['E2E_LORE_DEFERRED_BODY', 'internal-lore-identity-core', 'internal-lore-identity-lazy', 'unassigned-lore-internal', 'shared-lore-internal', 'empty-lore-internal', 'UNASSIGNED_BODY', 'E2E_SHARED_BRIEF', 'E2E_HIDDEN_INDEX_TAG', '"index_memberships"']) expect(content).not.toContain(marker)
  expect(content.split('E2E_LORE_ALWAYS_BODY')).toHaveLength(2)
  expect(content.split('E2E_SHARED_BODY')).toHaveLength(2)
  expect(content.indexOf('### Shared navigator')).toBeLessThan(content.indexOf('## Core'))
  expect(content.indexOf('## Character · On demand')).toBeLessThan(content.indexOf('## Harbor'))
  expect(content.indexOf('## Harbor')).toBeLessThan(content.indexOf('## Core'))
}
