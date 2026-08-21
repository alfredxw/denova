import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { ImageAPIProfilesEditor } from './ImageAPIProfilesEditor'
import { imageAPIProfileID, newImageAPIProfile } from './image-profiles'
import type { ImageAPIProfileSettings } from './types'

vi.mock('./ImageProfilePingButton', () => ({ ImageProfilePingButton: () => null }))

function EditorHarness({ initialProfiles = [{
    id: 'default',
    ...newImageAPIProfile(),
  }], initialDefaultProfileID = 'default' }: {
  initialProfiles?: ImageAPIProfileSettings[]
  initialDefaultProfileID?: string
} = {}) {
  const [profiles, setProfiles] = useState<ImageAPIProfileSettings[]>(initialProfiles)
  const [defaultProfileID, setDefaultProfileID] = useState(initialDefaultProfileID)
  return (
    <>
      <output data-testid="default-profile-id">{defaultProfileID}</output>
      <output data-testid="profile-ids">{profiles.map(imageAPIProfileID).join(',')}</output>
      <ImageAPIProfilesEditor
        profiles={profiles}
        effectiveProfiles={profiles}
        defaultProfileID={defaultProfileID}
        effectiveDefaultProfileID="default"
        onDefaultProfileChange={setDefaultProfileID}
        onChange={setProfiles}
      />
    </>
  )
}

describe('ImageAPIProfilesEditor', () => {
  it('offers the supported providers and switches ComfyUI to its built-in workflow', async () => {
    const user = userEvent.setup()
    render(<EditorHarness />)

    await user.click(screen.getByRole('combobox', { name: '服务商' }))
    for (const provider of ['OpenAI', 'xAI / Grok', 'ComfyUI', '火山引擎 Seedream', 'Google Gemini', '自定义端点']) {
      expect(screen.getByRole('option', { name: provider })).toBeInTheDocument()
    }
    await user.click(screen.getByRole('option', { name: 'ComfyUI' }))

    expect(screen.getByRole('combobox', { name: '工作流' })).toHaveTextContent('内置基础工作流')
    expect(screen.getByRole('textbox', { name: 'Checkpoint' })).toBeInTheDocument()
    expect(screen.getByText('填写 Checkpoint 名称即可，不需要上传工作流。')).toBeInTheDocument()
  })

  it('shows only the resolutions and quality levels accepted by xAI', async () => {
    const user = userEvent.setup()
    render(<EditorHarness />)

    await user.click(screen.getByRole('combobox', { name: '服务商' }))
    await user.click(screen.getByRole('option', { name: 'xAI / Grok' }))

    const resolution = screen.getByRole('combobox', { name: '默认分辨率' })
    expect(resolution).toHaveTextContent('1k')
    await user.click(resolution)
    expect(screen.getByRole('option', { name: '2k' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: '4K' })).not.toBeInTheDocument()
    await user.keyboard('{Escape}')

    const quality = screen.getByRole('combobox', { name: '默认质量' })
    expect(quality).toHaveTextContent('medium')
    await user.click(quality)
    expect(screen.getByRole('option', { name: 'low' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'high' })).not.toBeInTheDocument()
  })

  it('accepts a ComfyUI API Format workflow file', async () => {
    const user = userEvent.setup()
    const { container } = render(<EditorHarness />)
    await user.click(screen.getByRole('combobox', { name: '服务商' }))
    await user.click(screen.getByRole('option', { name: 'ComfyUI' }))
    await user.click(screen.getByRole('combobox', { name: '工作流' }))
    await user.click(screen.getByRole('option', { name: '上传 API Format' }))

    const workflow = `{"6":{"class_type":"CLIPTextEncode","inputs":{"text":"old"}}}`
    const file = Object.assign(new File([workflow], 'portrait-api.json', { type: 'application/json' }), {
      text: async () => workflow,
    })
    const input = container.querySelector<HTMLInputElement>('input[type="file"]')
    expect(input).not.toBeNull()
    fireEvent.change(input!, { target: { files: [file] } })

    await waitFor(() => expect(screen.getAllByText('portrait-api.json').length).toBeGreaterThan(0))
    expect(screen.getByText('执行前会自动注入提示词和常用尺寸参数。')).toBeInTheDocument()
  })

  it('keeps the native workflow input out of the settings layout', async () => {
    const user = userEvent.setup()
    const { container } = render(<EditorHarness />)
    await user.click(screen.getByRole('combobox', { name: '服务商' }))
    await user.click(screen.getByRole('option', { name: 'ComfyUI' }))
    await user.click(screen.getByRole('combobox', { name: '工作流' }))
    await user.click(screen.getByRole('option', { name: '上传 API Format' }))

    const input = container.querySelector<HTMLInputElement>('input[type="file"]')
    expect(input).toHaveClass('hidden')
    expect(screen.getByRole('button', { name: '选择 JSON' })).toBeInTheDocument()
  })

  it('assigns stable unique IDs to newly added profiles', async () => {
    const user = userEvent.setup()
    render(<EditorHarness />)

    await user.click(screen.getByRole('button', { name: '添加图像模型' }))
    await user.click(screen.getByRole('button', { name: '添加图像模型' }))

    expect(screen.getByTestId('profile-ids')).toHaveTextContent('default,gpt-image-2,gpt-image-2-2')
  })

  it('promotes the next image model without changing its stable ID when the default is deleted', async () => {
    const user = userEvent.setup()
    render(<EditorHarness initialProfiles={[
      { id: 'default', name: 'Primary', provider: 'openai', model: 'image-a' },
      { id: 'fast-image', name: 'Fast image', provider: 'xai', model: 'image-b' },
      { id: 'quality-image', name: 'Quality image', provider: 'google', model: 'image-c' },
    ]} />)

    await user.click(screen.getAllByRole('button', { name: '删除图像模型' })[0])

    expect(screen.getByTestId('default-profile-id')).toHaveTextContent('fast-image')
    expect(screen.getByTestId('profile-ids')).toHaveTextContent('fast-image,quality-image')
  })
})
