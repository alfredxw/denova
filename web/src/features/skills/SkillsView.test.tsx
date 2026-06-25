import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSkill, deleteSkillDocument, getSkillDocument, getSkills, saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillSnapshot } from '@/lib/api'
import { SkillsView } from './SkillsView'

vi.mock('@/components/Chat/ConfigManagerChat', () => ({
  ConfigManagerChat: () => <div data-testid="config-manager-chat" />,
}))

vi.mock('@/lib/api', () => ({
  createSkill: vi.fn(),
  deleteSkillDocument: vi.fn(),
  getSkillDocument: vi.fn(),
  getSkills: vi.fn(),
  saveSkillDocument: vi.fn(),
}))

describe('SkillsView', () => {
  beforeEach(() => {
    vi.mocked(createSkill).mockReset()
    vi.mocked(deleteSkillDocument).mockReset()
    vi.mocked(getSkillDocument).mockReset()
    vi.mocked(getSkills).mockReset()
    vi.mocked(saveSkillDocument).mockReset()
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot())
    vi.mocked(createSkill).mockImplementation(async (scope, name, description, agents = []) => skillDocument({
      scope,
      name,
      description,
      agent: agents.join(','),
    }))
  })

  it('creates new Skills in user scope by default', async () => {
    const user = userEvent.setup()
    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: /新建 Skill/ }))
    await user.type(screen.getByLabelText('Skill 名称'), 'draft-plan')
    await user.type(screen.getByLabelText('触发说明'), '规划章节草稿')
    await user.click(screen.getByRole('button', { name: '创建 SKILL.md' }))

    await waitFor(() => {
      expect(vi.mocked(createSkill)).toHaveBeenCalledWith('user', 'draft-plan', '规划章节草稿', ['ide'])
    })
  })
})

function skillsSnapshot(): SkillSnapshot {
  return {
    scopes: [
      { scope: 'workspace', path: '/books/demo/.nova/skills', writable: true },
      { scope: 'user', path: '/nova/skills', writable: true },
      { scope: 'builtin', path: '/app/skills', writable: false },
    ],
    skills: [],
  }
}

function skillDocument(patch: Partial<SkillDocument>): SkillDocument {
  return {
    name: 'draft-plan',
    description: '',
    scope: 'user',
    path: '/nova/skills/draft-plan/SKILL.md',
    editable: true,
    active: true,
    content: '---\nname: draft-plan\ndescription: Planning\n---\n',
    ...patch,
  }
}
