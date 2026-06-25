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

  it('creates a user override when editing a built-in Skill', async () => {
    const user = userEvent.setup()
    const content = '---\nname: outline\ndescription: Built-in outline\n---\n\n# Outline\n'
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({
      skills: [
        {
          name: 'outline',
          description: 'Built-in outline',
          scope: 'builtin',
          path: '/app/skills/outline/SKILL.md',
          editable: false,
          active: true,
          content,
        } as SkillDocument,
      ],
    }))
    vi.mocked(getSkillDocument).mockResolvedValue(skillDocument({
      name: 'outline',
      description: 'Built-in outline',
      scope: 'builtin',
      path: '/app/skills/outline/SKILL.md',
      editable: false,
      active: true,
      content,
    }))
    vi.mocked(saveSkillDocument).mockImplementation(async (scope, name, savedContent) => skillDocument({
      scope,
      name,
      path: `/nova/skills/${name}/SKILL.md`,
      editable: true,
      active: true,
      content: savedContent,
    }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '创建用户覆盖' }))

    await waitFor(() => {
      expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledWith('user', 'outline', content)
    })
  })

  it('renames and moves editable Skills from the config panel', async () => {
    const user = userEvent.setup()
    const doc = skillDocument({
      name: 'draft-plan',
      description: 'Planning',
      scope: 'user',
      path: '/nova/skills/draft-plan/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: draft-plan\ndescription: Planning\nagent: ide\n---\n\n# Draft Plan\n',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [doc] }))
    vi.mocked(getSkillDocument).mockResolvedValue(doc)
    vi.mocked(saveSkillDocument).mockImplementation(async (scope, name, savedContent, target) => skillDocument({
      scope: target?.scope || scope,
      name: target?.name || name,
      description: 'Beat planning',
      path: `/books/demo/.nova/skills/${target?.name || name}/SKILL.md`,
      editable: true,
      active: true,
      content: savedContent,
    }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '配置' }))
    await user.clear(screen.getByLabelText('Skill 名称'))
    await user.type(screen.getByLabelText('Skill 名称'), 'beat-plan')
    await user.click(screen.getByRole('button', { name: '工作区' }))
    await user.clear(screen.getByLabelText('触发说明'))
    await user.type(screen.getByLabelText('触发说明'), 'Beat planning')
    await user.click(screen.getByRole('button', { name: '保存配置' }))

    await waitFor(() => {
      expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledWith(
        'user',
        'draft-plan',
        expect.stringContaining('name: "beat-plan"'),
        { scope: 'workspace', name: 'beat-plan' },
      )
    })
    expect(vi.mocked(saveSkillDocument).mock.calls[0][2]).toContain('description: "Beat planning"')
  })

  it('restores built-in Skill by deleting the active override', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const override = skillDocument({
      name: 'novel-standard',
      description: 'Workspace override',
      scope: 'workspace',
      path: '/books/demo/.nova/skills/novel-standard/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: novel-standard\ndescription: Workspace override\n---\n\n# Override\n',
    })
    const builtin = skillDocument({
      name: 'novel-standard',
      description: 'Built-in standard',
      scope: 'builtin',
      path: '/app/skills/novel-standard/SKILL.md',
      editable: false,
      active: false,
      content: '---\nname: novel-standard\ndescription: Built-in standard\n---\n\n# Built-in\n',
    })
    vi.mocked(getSkills)
      .mockResolvedValueOnce(skillsSnapshot({ skills: [override, builtin] }))
      .mockResolvedValueOnce(skillsSnapshot({ skills: [{ ...builtin, active: true }] }))
    vi.mocked(getSkillDocument).mockImplementation(async (scope) => (scope === 'workspace' ? override : { ...builtin, active: true }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(vi.mocked(deleteSkillDocument)).toHaveBeenCalledWith('workspace', 'novel-standard')
    })
    confirmSpy.mockRestore()
  })
})

function skillsSnapshot(patch: Partial<SkillSnapshot> = {}): SkillSnapshot {
  return {
    scopes: [
      { scope: 'workspace', path: '/books/demo/.nova/skills', writable: true },
      { scope: 'user', path: '/nova/skills', writable: true },
      { scope: 'builtin', path: '/app/skills', writable: false },
    ],
    skills: [],
    ...patch,
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
