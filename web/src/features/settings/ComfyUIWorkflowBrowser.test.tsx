import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { discoverComfyUIWorkflows, loadComfyUIWorkflow } from './api'
import { ComfyUIWorkflowBrowser } from './ComfyUIWorkflowBrowser'
import type { ComfyUIProfileSettings, ImageAPIProfileSettings } from './types'

vi.mock('./api', () => ({
  discoverComfyUIWorkflows: vi.fn(),
  loadComfyUIWorkflow: vi.fn(),
}))

const endpoint = { id: 'comfy', provider: 'comfyui', protocol: 'comfyui-workflow', base_url: 'http://127.0.0.1:8188' }
const summary = {
  name: 'Portrait', path: 'portrait.json', workflow_id: 'workflow-id', modified: 100,
  status: 'ready' as const, job_id: 'job-id', job_time: 110,
}

describe('ComfyUIWorkflowBrowser', () => {
  beforeEach(() => {
    vi.mocked(discoverComfyUIWorkflows).mockReset()
    vi.mocked(loadComfyUIWorkflow).mockReset()
    vi.mocked(discoverComfyUIWorkflows).mockResolvedValue({ workflows: [summary] })
  })

  it('persists only provider-neutral bindings from a discovered workflow', async () => {
    vi.mocked(loadComfyUIWorkflow).mockResolvedValue({
      ...summary,
      workflow: '{"workflow":true}',
      bindings: {
        prompt: { node_id: '2', input_name: 'text' },
        count: { node_id: '4', input_name: 'batch_size' },
        width: { node_id: '4', input_name: 'width' },
        height: { node_id: '4', input_name: 'height' },
      },
      candidates: {
        prompt: [{ node_id: '2', input_name: 'text', label: 'Positive Prompt' }],
        count: [{ node_id: '4', input_name: 'batch_size', label: 'Canvas' }],
        width: [{ node_id: '4', input_name: 'width', label: 'Canvas · width' }],
        height: [{ node_id: '4', input_name: 'height', label: 'Canvas · height' }],
      },
    })
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<WorkflowHarness onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: '发现工作流' }))
    await user.click(await screen.findByRole('button', { name: /Portrait/ }))

    expect(await screen.findByText('生成输入绑定')).toBeInTheDocument()
    expect(screen.getByText('提示词已绑定')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '提示词' })).toHaveTextContent('Positive Prompt')
    expect(screen.queryByText('Sampler · cfg')).not.toBeInTheDocument()
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({
      workflow: '{"workflow":true}',
      bindings: {
        prompt: { node_id: '2', input_name: 'text' },
        count: { node_id: '4', input_name: 'batch_size' },
        width: { node_id: '4', input_name: 'width' },
        height: { node_id: '4', input_name: 'height' },
      },
    }))
    expect(onChange.mock.lastCall?.[0]).not.toHaveProperty('parameters')
  })

  it('lets the user repair an ambiguous prompt without exposing static values', async () => {
    vi.mocked(loadComfyUIWorkflow).mockResolvedValue({
      ...summary,
      workflow: '{"workflow":true}',
      bindings: { count: { node_id: '4', input_name: 'batch_size' } },
      candidates: {
        prompt: [
          { node_id: '2', input_name: 'text', label: 'Positive A' },
          { node_id: '6', input_name: 'text', label: 'Positive B' },
        ],
        count: [{ node_id: '4', input_name: 'batch_size', label: 'Canvas' }],
      },
    })
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<WorkflowHarness onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: '发现工作流' }))
    await user.click(await screen.findByRole('button', { name: /Portrait/ }))
    expect(await screen.findByText('需要绑定提示词')).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: '提示词' }))
    await user.click(await screen.findByRole('option', { name: /Positive B.*6\.text/ }))

    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({
      bindings: {
        count: { node_id: '4', input_name: 'batch_size' },
        prompt: { node_id: '6', input_name: 'text' },
      },
    }))
    expect(screen.queryByText('Sampler · cfg')).not.toBeInTheDocument()
  })

  it('selects an unparsed workflow and explains how to finish API parsing', async () => {
    vi.mocked(discoverComfyUIWorkflows).mockResolvedValue({
      workflows: [{ ...summary, status: 'not_run', job_id: undefined, job_time: undefined }],
    })
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<WorkflowHarness onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: '发现工作流' }))
    await user.click(await screen.findByRole('button', { name: /Portrait/ }))

    expect(await screen.findByText('需要完成 API 解析')).toBeInTheDocument()
    expect(screen.getByText(/运行一次这个工作流，然后刷新/)).toBeInTheDocument()
    expect(onChange).toHaveBeenLastCalledWith({
      workflow_mode: 'remote',
      workflow_name: 'Portrait',
      workflow_id: 'workflow-id',
      workflow_path: 'portrait.json',
      workflow_modified: 100,
    })
    expect(loadComfyUIWorkflow).not.toHaveBeenCalled()

    vi.mocked(discoverComfyUIWorkflows).mockResolvedValue({ workflows: [summary] })
    vi.mocked(loadComfyUIWorkflow).mockResolvedValue({
      ...summary,
      workflow: '{"workflow":true}',
      bindings: { prompt: { node_id: '2', input_name: 'text' } },
      candidates: { prompt: [{ node_id: '2', input_name: 'text', label: 'Positive Prompt' }] },
    })
    await user.click(screen.getAllByRole('button', { name: '刷新工作流' })[0])

    expect(await screen.findByText('生成输入绑定')).toBeInTheDocument()
    expect(loadComfyUIWorkflow).toHaveBeenCalledWith(endpoint, expect.anything(), 'portrait.json', expect.any(AbortSignal))
  })

  it('imports API Format JSON as the fallback workflow source', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    const workflow = JSON.stringify({
      1: { class_type: 'CLIPTextEncode', inputs: { text: 'portrait' } },
    })
    render(<WorkflowHarness onChange={onChange} />)

    await user.upload(
      screen.getByLabelText('导入 API JSON'),
      new File([workflow], 'portrait-api.json', { type: 'application/json' }),
    )

    expect(await screen.findByText('portrait-api.json')).toBeInTheDocument()
    expect(screen.getByText(/已导入 ComfyUI API Format JSON/)).toBeInTheDocument()
    expect(onChange).toHaveBeenLastCalledWith({
      workflow_mode: 'api',
      workflow,
      workflow_name: 'portrait-api.json',
    })
    expect(screen.queryByText('生成输入绑定')).not.toBeInTheDocument()
  })
})

function WorkflowHarness({ onChange }: { onChange: (settings: ComfyUIProfileSettings) => void }) {
  const [comfyui, setComfyUI] = useState<ComfyUIProfileSettings>({ workflow_mode: 'remote' })
  const profile: ImageAPIProfileSettings = { id: 'portrait', endpoint_id: 'comfy', comfyui }
  return (
    <ComfyUIWorkflowBrowser
      endpoint={endpoint}
      profile={profile}
      onChange={(settings) => {
        setComfyUI(settings)
        onChange(settings)
      }}
    />
  )
}
