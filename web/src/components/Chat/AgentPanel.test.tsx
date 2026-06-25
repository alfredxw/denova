import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentPanel } from './AgentPanel'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({
    effective: { ide_story_teller_id: 'classic' },
    workspace: {},
  }),
  updateWorkspaceSettings: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

describe('AgentPanel', () => {
  it('创作 Agent 顶部切换器不再展示 Review tab', () => {
    render(
      <AgentPanel
        workspace="/workspace"
        selectedFile={null}
        tellers={[{ id: 'classic', name: '默认叙事', style_rules: [] } as any]}
        messages={[]}
        sessions={[{ id: 'session-1', title: '当前会话', active: true, message_count: 0, created_at: '', updated_at: '' }]}
        activeSessionId="session-1"
        isStreaming={false}
        activityContent=""
        references={[]}
        loreReferences={[]}
        loreReferenceLabels={{}}
        loreSuggestions={[]}
        styleScenes={[]}
        textSelections={[]}
        fileSuggestions={[]}
        onCreateSession={vi.fn()}
        onSwitchSession={vi.fn()}
        onRenameSession={vi.fn()}
        onDeleteSession={vi.fn()}
        onSend={vi.fn()}
        onAnalyzeContext={vi.fn().mockResolvedValue({} as any)}
        onStop={vi.fn()}
        onReferenceRemove={vi.fn()}
        onLoreReferenceAdd={vi.fn()}
        onLoreReferenceRemove={vi.fn()}
        onStyleSceneAdd={vi.fn()}
        onStyleSceneRemove={vi.fn()}
        onTextSelectionRemove={vi.fn()}
        onClose={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: '对话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '会话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行追踪' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review' })).not.toBeInTheDocument()
  })
})
