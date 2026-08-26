import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { discoverModels } from './api'
import { ModelDiscoveryPicker } from './ModelDiscoveryPicker'

vi.mock('./api', () => ({
  discoverModels: vi.fn(),
}))

describe('ModelDiscoveryPicker', () => {
  it('adds multiple discovered models while disabling existing ones', async () => {
    vi.mocked(discoverModels).mockResolvedValue({
      provider: 'openai',
      protocol: 'openai-chat-completions',
      base_url: 'https://models.example/v1',
      models: [
        { id: 'model-a', display_name: 'Model A' },
        { id: 'model-b', display_name: 'Model B' },
        { id: 'model-existing', display_name: 'Existing Model' },
      ],
    })
    const onAdd = vi.fn()
    const user = userEvent.setup()
    render(
      <ModelDiscoveryPicker
        endpoint={{ id: 'shared', provider: 'openai', protocol: 'openai-chat-completions', base_url: 'https://models.example/v1' }}
        existingModels={['model-existing']}
        onAdd={onAdd}
      />,
    )

    await user.click(screen.getByRole('button', { name: '选择模型' }))
    const list = await screen.findByRole('listbox', { name: '可用模型' })
    const existing = within(list).getByRole('option', { name: /Existing Model/ })
    expect(existing).toBeDisabled()

    await user.click(within(list).getByRole('option', { name: /Model A/ }))
    await user.click(within(list).getByRole('option', { name: /Model B/ }))
    await user.click(screen.getByRole('button', { name: '添加选中的 2 个模型' }))

    expect(onAdd).toHaveBeenCalledWith([
      { id: 'model-a', display_name: 'Model A' },
      { id: 'model-b', display_name: 'Model B' },
    ])
  })
})
