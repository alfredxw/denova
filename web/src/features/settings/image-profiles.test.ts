import { hasConfiguredImageModel, imageAPIProfileLabel, newImageAPIProfile } from './image-profiles'

describe('image model availability', () => {
  it('does not treat built-in defaults without credentials as configured', () => {
    expect(hasConfiguredImageModel()).toBe(false)
    expect(hasConfiguredImageModel({})).toBe(false)
    expect(hasConfiguredImageModel({ image_api_endpoints: [{ id: 'default', api_key: '  ' }] })).toBe(false)
    expect(hasConfiguredImageModel({ image_api_endpoints: [{ id: 'default', api_key: 'key' }] })).toBe(true)
  })

  it('accepts a keyless custom model and rejects an incomplete connection or model', () => {
    const settings = {
      default_image_api_profile_id: 'local',
      image_api_endpoints: [{ id: 'local', provider: 'custom', base_url: 'http://localhost:8000/v1' }],
      image_api_profiles: [{ id: 'local', endpoint_id: 'local', model: 'image' }],
    }
    expect(hasConfiguredImageModel(settings)).toBe(true)
    expect(hasConfiguredImageModel({ ...settings, image_api_endpoints: [] })).toBe(false)
    expect(hasConfiguredImageModel({ ...settings, image_api_profiles: [{ id: 'local', endpoint_id: 'local' }] })).toBe(false)
  })

  it('requires a usable workflow for ComfyUI, without requiring an API key or model ID', () => {
    const settings = {
      default_image_api_profile_id: 'workflow',
      image_api_endpoints: [{ id: 'comfy', provider: 'comfyui' }],
      image_api_profiles: [{ id: 'workflow', endpoint_id: 'comfy', comfyui: { workflow: '{}' } }],
    }
    expect(hasConfiguredImageModel(settings)).toBe(true)
    expect(hasConfiguredImageModel({ ...settings, image_api_profiles: [{ id: 'workflow', endpoint_id: 'comfy' }] })).toBe(false)
  })
})

describe('ComfyUI image profiles', () => {
  it('defaults to discovering a saved user workflow', () => {
    expect(newImageAPIProfile('comfyui', 'comfy-endpoint')).toMatchObject({
      endpoint_id: 'comfy-endpoint',
      comfyui: { workflow_mode: 'remote' },
    })
  })

  it('uses the selected workflow name instead of a legacy model value', () => {
    expect(imageAPIProfileLabel({
      model: 'legacy-checkpoint.safetensors',
      comfyui: { workflow_mode: 'remote', workflow_name: 'Portrait' },
    })).toBe('Portrait')
  })
})
