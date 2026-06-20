import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSkill, getSkillDocument, getSkills, saveSkillDocument } from '@/lib/api'
import type { SkillDocument, SkillSnapshot } from '@/lib/api'
import { SkillsView } from './SkillsView'

vi.mock('@/lib/api', () => ({
  createSkill: vi.fn(),
  getSkillDocument: vi.fn(),
  getSkills: vi.fn(),
  saveSkillDocument: vi.fn(),
}))

const baseSnapshot: SkillSnapshot = {
  scopes: [
    { scope: 'builtin', path: '/nova/builtin/skills', writable: false },
    { scope: 'user', path: '/nova/user/skills', writable: true },
    { scope: 'workspace', path: '/books/demo/.nova/skills', writable: true },
  ],
  skills: [
    {
      name: 'outline',
      description: 'Build outlines.',
      scope: 'workspace',
      path: '/books/demo/.nova/skills/outline/SKILL.md',
      editable: true,
      active: true,
      agent: 'ide',
    },
  ],
}

const outlineDoc: SkillDocument = {
  ...baseSnapshot.skills[0],
  content: `---
name: outline
description: Build outlines.
agent: ide
---

# outline
`,
}

describe('SkillsView', () => {
  beforeEach(() => {
    vi.mocked(getSkills).mockReset()
    vi.mocked(getSkillDocument).mockReset()
    vi.mocked(createSkill).mockReset()
    vi.mocked(saveSkillDocument).mockReset()
    vi.mocked(getSkills).mockResolvedValue(baseSnapshot)
    vi.mocked(getSkillDocument).mockResolvedValue(outlineDoc)
    vi.mocked(saveSkillDocument).mockImplementation(async (_scope, _name, content) => ({ ...outlineDoc, content }))
  })

  it('opens a guided create panel from the Skills page', async () => {
    const user = userEvent.setup()
    render(<SkillsView workspace="/books/demo" />)

    await screen.findByText('/outline')
    expect(screen.getByRole('button', { name: '配置 Agent' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '新建 Skill' }))

    expect(screen.getByText('基础信息')).toBeInTheDocument()
    expect(screen.getByLabelText('Skill 名称')).toBeInTheDocument()
    expect(screen.getByLabelText('触发说明')).toBeInTheDocument()
    expect(screen.getByText('/skill-name')).toBeInTheDocument()
    expect(screen.getByText('/books/demo/.nova/skills/skill-name/SKILL.md')).toBeInTheDocument()
  })

  it('creates a skill with scope, description, and selected agents, then opens it', async () => {
    const user = userEvent.setup()
    const createdDoc: SkillDocument = {
      name: 'beats',
      description: 'Draft chapter beats.',
      scope: 'workspace',
      path: '/books/demo/.nova/skills/beats/SKILL.md',
      editable: true,
      active: true,
      agent: 'ide,config_manager',
      content: `---
name: beats
description: Draft chapter beats.
agent: ide,config_manager
---

# beats
`,
    }
    const updatedSnapshot: SkillSnapshot = {
      ...baseSnapshot,
      skills: [...baseSnapshot.skills, { ...createdDoc }],
    }
    vi.mocked(createSkill).mockResolvedValue(createdDoc)
    vi.mocked(getSkills)
      .mockResolvedValueOnce(baseSnapshot)
      .mockResolvedValueOnce(updatedSnapshot)
    vi.mocked(getSkillDocument).mockImplementation(async (_scope, name) => (name === 'beats' ? createdDoc : outlineDoc))

    render(<SkillsView workspace="/books/demo" />)
    await screen.findByText('/outline')
    await user.click(screen.getByRole('button', { name: '新建 Skill' }))
    await user.type(screen.getByLabelText('Skill 名称'), 'beats')
    await user.type(screen.getByLabelText('触发说明'), 'Draft chapter beats.')
    await user.click(screen.getByLabelText(/配置管理 Agent/))
    await user.click(screen.getByRole('button', { name: '创建 SKILL.md' }))

    await waitFor(() => {
      expect(createSkill).toHaveBeenCalledWith('workspace', 'beats', 'Draft chapter beats.', ['ide', 'config_manager'])
    })
    await waitFor(() => {
      expect(getSkillDocument).toHaveBeenCalledWith('workspace', 'beats')
    })
    expect((await screen.findAllByText('/beats')).length).toBeGreaterThan(0)
  })

  it('shows invalid names without calling create', async () => {
    const user = userEvent.setup()
    render(<SkillsView workspace="/books/demo" />)

    await screen.findByText('/outline')
    await user.click(screen.getByRole('button', { name: '新建 Skill' }))
    await user.type(screen.getByLabelText('Skill 名称'), 'bad name')

    expect(screen.getByText('Skill 名称只能包含字母、数字、下划线或连字符，并且必须以字母或数字开头。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '创建 SKILL.md' })).toBeDisabled()
    expect(createSkill).not.toHaveBeenCalled()
  })

  it('disables creation when no writable scope exists', async () => {
    const user = userEvent.setup()
    vi.mocked(getSkills).mockResolvedValue({
      scopes: [{ scope: 'builtin', path: '/nova/builtin/skills', writable: false }],
      skills: [],
    })

    render(<SkillsView workspace="/books/demo" />)
    await user.click(await screen.findByRole('button', { name: '新建 Skill' }))

    expect(screen.getByText('当前没有可写的 Skill 目录。请检查工作区或用户级 Nova 目录配置后再创建。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '创建 SKILL.md' })).not.toBeInTheDocument()
  })
})
