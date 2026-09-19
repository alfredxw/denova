import { expect, it } from 'vitest'
import { runtimeProfileOptions } from './api-profiles'
import type { ModelCatalog, Settings } from '@/features/settings/types'

it('offers only profiles whose effective protocol and routing are supported by the CLI', () => {
  const catalog: ModelCatalog = { protocols: [], providers: [
    { id: 'openai', name: 'OpenAI', default_protocol: 'openai-responses', endpoints: {} },
    { id: 'anthropic', name: 'Anthropic', default_protocol: 'anthropic-messages', endpoints: {} },
  ] }
  const effective: Settings = {
    model_endpoints: [
      { id: 'responses', provider: 'openai' },
      { id: 'messages', provider: 'anthropic' },
      { id: 'chat', provider: 'openai', protocol: 'openai-chat-completions' },
      { id: 'override', provider: 'openai', protocol: 'anthropic-messages' },
      { id: 'options', provider: 'openai', protocol_options: { extra: true } },
      { id: 'mapping', provider: 'anthropic', session_key_mapping: { location: 'header', name: 'X-Session' } },
    ],
    model_profiles: ['responses', 'messages', 'chat', 'override', 'options', 'mapping', 'missing'].map(id => ({ id, endpoint_id: id, model: 'model', name: ` ${id} ` })),
  }
  expect(runtimeProfileOptions('codex', { effective }, catalog)).toEqual([{ id: 'profile:responses', label: 'responses', modelLabel: 'responses' }])
  expect(runtimeProfileOptions('claude', { effective }, catalog).map(item => item.id)).toEqual(['profile:messages', 'profile:override'])
  expect(runtimeProfileOptions('native', { effective }, catalog)).toEqual([])
})
