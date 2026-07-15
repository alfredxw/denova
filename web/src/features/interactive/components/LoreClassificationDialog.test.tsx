import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { applyLoreClassification, previewLoreClassification, type LoreItem } from '@/lib/api'
import { LoreClassificationDialog } from './LoreClassificationDialog'

vi.mock('@/lib/api', () => ({
  previewLoreClassification: vi.fn(),
  applyLoreClassification: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const classifiedItem: LoreItem = {
  id: 'shen',
  enabled: true,
  type: 'character',
  type_source: 'manual',
  name: '人物详情：沈凝',
  importance: 'important',
  load_mode: 'auto',
  tags: [],
  brief_description: '公开比试见证者',
  keywords: [],
  content: '## 沈凝',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:01Z',
}

describe('LoreClassificationDialog', () => {
  beforeEach(() => {
    vi.mocked(previewLoreClassification).mockReset()
    vi.mocked(applyLoreClassification).mockReset()
    vi.mocked(previewLoreClassification).mockResolvedValue({
      revision: 'rev-1',
      mode: 'semantic',
      counts: { character: 1 },
      items: [{
        id: 'shen',
        name: '人物详情：沈凝',
        current_type: 'other',
        current_type_source: 'heuristic',
        suggested_type: 'character',
        confidence: 'high',
        suggestion_source: 'heuristic',
      }],
    })
    vi.mocked(applyLoreClassification).mockResolvedValue({ revision: 'rev-2', items: [classifiedItem], updated: [classifiedItem] })
  })

  it('previews suggestions and applies only the selected type changes', async () => {
    const onApplied = vi.fn()
    render(<LoreClassificationDialog open onOpenChange={vi.fn()} onApplied={onApplied} />)

    expect(await screen.findByText('人物详情：沈凝')).toBeInTheDocument()
    expect(screen.getByText('将更新 1 项')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '应用所选分类' }))

    await waitFor(() => expect(applyLoreClassification).toHaveBeenCalledWith({
      revision: 'rev-1',
      changes: [{ id: 'shen', type: 'character' }],
    }))
    expect(onApplied).toHaveBeenCalledWith([classifiedItem])
  })
})
