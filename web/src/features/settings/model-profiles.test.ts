import { expect, it } from 'vitest'
import { modelEndpointsWithDefault, modelProfilesWithDefault } from './model-profiles'
import { hasUsableLanguageModel } from '@/features/onboarding/model-status'

it('keeps unconfigured settings empty and requires model setup', () => {
  for (const settings of [undefined, {}, { model_endpoints: [], model_profiles: [] }]) {
    expect(modelEndpointsWithDefault(settings)).toEqual([])
    expect(modelProfilesWithDefault(settings)).toEqual([])
    expect(hasUsableLanguageModel(settings)).toBe(false)
  }
})

it('uses user connections and models without inserting a default preset', () => {
  const settings = {
    model_endpoints: [{ id: 'mine', api_key: 'test-key', base_url: 'https://models.example/v1' }],
    model_profiles: [{ id: 'my-model', endpoint_id: 'mine', model: 'my-model' }],
  }
  expect(modelEndpointsWithDefault(settings)).toEqual(settings.model_endpoints)
  expect(modelProfilesWithDefault(settings)).toEqual(settings.model_profiles)
  expect(hasUsableLanguageModel(settings)).toBe(true)
})

it('preserves explicitly configured legacy models', () => {
  const settings = { openai_api_key: 'test-key', openai_base_url: 'https://api.deepseek.com', openai_model: 'my-model' }
  expect(modelEndpointsWithDefault(settings)).toMatchObject([{ id: 'default', api_key: 'test-key' }])
  expect(modelProfilesWithDefault(settings)).toMatchObject([{ id: 'default', model: 'my-model' }])
  expect(hasUsableLanguageModel(settings)).toBe(true)
})
