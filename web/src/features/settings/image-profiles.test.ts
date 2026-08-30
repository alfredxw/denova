import { imageAPIProfileLabel, newImageAPIProfile } from './image-profiles'

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
