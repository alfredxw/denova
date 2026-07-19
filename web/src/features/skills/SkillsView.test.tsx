import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSkill, deleteSkillDocument, getSkillDocument, getSkillFileDocument, getSkills, installSkillRemote, installSkillZip, previewSkillRemoteInstall, previewSkillZipInstall, saveSkillDocument, saveSkillFileDocument } from '@/lib/api'
import { APIError } from '@/lib/api-client'
import type { SkillDocument, SkillFileDocument, SkillSnapshot } from '@/lib/api'
import { SkillsView } from './SkillsView'

vi.mock('@/components/Chat/ConfigManagerChat', () => ({
  ConfigManagerChat: () => <div data-testid="config-manager-chat" />,
}))

vi.mock('@/lib/api', () => ({
  createSkill: vi.fn(),
  deleteSkillDocument: vi.fn(),
  getSkillDocument: vi.fn(),
  getSkillFileDocument: vi.fn(),
  getSkills: vi.fn(),
  installSkillRemote: vi.fn(),
  installSkillZip: vi.fn(),
  previewSkillRemoteInstall: vi.fn(),
  previewSkillZipInstall: vi.fn(),
  saveSkillDocument: vi.fn(),
  saveSkillFileDocument: vi.fn(),
}))

describe('SkillsView', () => {
  beforeEach(() => {
    vi.mocked(createSkill).mockReset()
    vi.mocked(deleteSkillDocument).mockReset()
    vi.mocked(getSkillDocument).mockReset()
    vi.mocked(getSkillFileDocument).mockReset()
    vi.mocked(getSkills).mockReset()
    vi.mocked(installSkillRemote).mockReset()
    vi.mocked(installSkillZip).mockReset()
    vi.mocked(previewSkillRemoteInstall).mockReset()
    vi.mocked(previewSkillZipInstall).mockReset()
    vi.mocked(saveSkillDocument).mockReset()
    vi.mocked(saveSkillFileDocument).mockReset()
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot())
    vi.mocked(createSkill).mockImplementation(async (scope, name, description, agents = []) => skillDocument({
      scope,
      name,
      description,
      agent: agents.join(','),
    }))
  })

  it('opens the Config Agent in a resizable desktop pane', async () => {
    const user = userEvent.setup()
    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '配置 Agent' }))

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '调整右侧面板宽度' })).toBeVisible()
  })

  it('creates new Skills in user scope by default', async () => {
    const user = userEvent.setup()
    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '新建' }))
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
      expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledWith('builtin', 'outline', content, { scope: 'user', name: 'outline' }, 'skill-r1')
    })
  })

  it('transparently rebases a built-in Skill override when its revision changed', async () => {
    const user = userEvent.setup()
    const initialContent = '---\nname: outline\ndescription: Built-in outline\n---\n\nOriginal body.\n'
    const externalContent = '---\nname: outline\ndescription: Built-in outline\n---\n\nUpdated built-in body.\n'
    const initial = skillDocument({
      name: 'outline',
      description: 'Built-in outline',
      scope: 'builtin',
      path: '/app/skills/outline/SKILL.md',
      editable: false,
      active: true,
      content: initialContent,
      revision: 'skill-r1',
    })
    const external = { ...initial, content: externalContent, revision: 'skill-r2' }
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [initial] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(external)
    vi.mocked(saveSkillDocument)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockImplementationOnce(async (_scope, name, savedContent, target) => skillDocument({
        ...external,
        scope: target?.scope || 'user',
        name,
        path: `/nova/skills/${name}/SKILL.md`,
        editable: true,
        content: savedContent,
        revision: 'skill-r3',
      }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '创建用户覆盖' }))

    await waitFor(() => expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledTimes(2))
    expect(vi.mocked(saveSkillDocument)).toHaveBeenNthCalledWith(
      2,
      'builtin',
      'outline',
      externalContent,
      { scope: 'user', name: 'outline' },
      'skill-r2',
    )
    expect(screen.queryByText('revision conflict')).not.toBeInTheDocument()
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
    await user.click(screen.getByRole('radio', { name: '工作区' }))
    await user.clear(screen.getByLabelText('触发说明'))
    await user.type(screen.getByLabelText('触发说明'), 'Beat planning')
    expect(screen.queryByRole('button', { name: '保存配置' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '应用重命名/迁移' }))

    await waitFor(() => {
      expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledWith(
        'user',
        'draft-plan',
        expect.stringContaining('name: "beat-plan"'),
        { scope: 'workspace', name: 'beat-plan' },
        'skill-r1',
      )
    })
    expect(vi.mocked(saveSkillDocument).mock.calls[0][2]).toContain('description: "Beat planning"')
  })

  it('transparently rebases and retries a Skill rename conflict', async () => {
    const user = userEvent.setup()
    const doc = skillDocument({
      name: 'draft-plan',
      description: 'Planning',
      scope: 'user',
      path: '/nova/skills/draft-plan/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: draft-plan\ndescription: Planning\n---\n\nOld body.\n',
      revision: 'skill-r1',
    })
    const external = {
      ...doc,
      content: '---\nname: draft-plan\ndescription: Planning\n---\n\nExternal body.\n',
      revision: 'skill-r2',
    }
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [doc] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(doc)
      .mockResolvedValueOnce(external)
    vi.mocked(saveSkillDocument)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockImplementationOnce(async (_scope, _name, content, target) => skillDocument({
        ...external,
        scope: target?.scope || 'user',
        name: target?.name || 'draft-plan',
        content,
        revision: 'skill-r3',
      }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '配置' }))
    await user.clear(screen.getByLabelText('Skill 名称'))
    await user.type(screen.getByLabelText('Skill 名称'), 'beat-plan')
    await user.click(screen.getByRole('button', { name: '应用重命名/迁移' }))

    await waitFor(() => expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledTimes(2))
    const retry = vi.mocked(saveSkillDocument).mock.calls[1]
    expect(retry?.slice(0, 2)).toEqual(['user', 'draft-plan'])
    expect(retry?.[2]).toContain('name: "beat-plan"')
    expect(retry?.[2]).toContain('External body.')
    expect(retry?.slice(3)).toEqual([{ scope: 'user', name: 'beat-plan' }, 'skill-r2'])
    expect(screen.queryByText('revision conflict')).not.toBeInTheDocument()
  })

  it('autosaves non-identity Skill config changes without a Save Config button', async () => {
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
    vi.mocked(saveSkillDocument).mockImplementation(async (scope, name, savedContent) => skillDocument({
      ...doc,
      scope,
      name,
      description: 'Beat planning',
      content: savedContent,
    }))

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '配置' }))
    const description = screen.getByLabelText('触发说明')
    await user.clear(description)
    await user.type(description, 'Beat planning')
    expect(screen.queryByRole('button', { name: '保存配置' })).not.toBeInTheDocument()
    fireEvent.keyDown(description, { key: 's', ctrlKey: true })

    await waitFor(() => {
      expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledWith(
        'user',
        'draft-plan',
        expect.stringContaining('description: "Beat planning"'),
        undefined,
        'skill-r1',
      )
    })
  })

  it('rebases Skill metadata autosave over an external content update', async () => {
    const initialContent = '---\nname: draft-plan\ndescription: Planning\nagent: ide\n---\n\nBody old.\n'
    const externalContent = '---\nname: draft-plan\ndescription: Planning\nagent: ide\n---\n\nBody updated externally.\n'
    const initial = skillDocument({ agent: 'ide', content: initialContent, revision: 'skill-r1' })
    const external = skillDocument({ agent: 'ide', content: externalContent, revision: 'skill-r2' })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [initial] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(external)
    vi.mocked(saveSkillDocument)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockImplementationOnce(async (scope, name, content) => skillDocument({
        ...external,
        scope,
        name,
        content,
        description: 'Beat planning',
        revision: 'skill-r3',
      }))

    render(<SkillsView workspace="/books/demo" />)

    await userEvent.click(await screen.findByRole('button', { name: '配置' }))
    const description = screen.getByLabelText('触发说明')
    await userEvent.clear(description)
    await userEvent.type(description, 'Beat planning')
    fireEvent.keyDown(description, { key: 's', ctrlKey: true })

    await waitFor(() => expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledTimes(2))
    const retry = vi.mocked(saveSkillDocument).mock.calls[1]
    expect(retry?.slice(0, 2)).toEqual(['user', 'draft-plan'])
    expect(retry?.[2]).toContain('description: "Beat planning"')
    expect(retry?.[2]).toContain('agent: "ide"')
    expect(retry?.[2]).toContain('Body updated externally.')
    expect(retry?.slice(3)).toEqual([undefined, 'skill-r2'])
    expect(screen.getByRole('status')).toHaveAccessibleName('所有更改均已保存')
  })

  it('reloads the selected document when refreshing an unchanged skill list', async () => {
    const user = userEvent.setup()
    const initial = skillDocument({
      description: 'Initial',
      content: '---\nname: draft-plan\ndescription: Initial\n---\n\n# Initial\n',
      revision: 'skill-r1',
    })
    const refreshed = skillDocument({
      description: 'External update',
      content: '---\nname: draft-plan\ndescription: External update\n---\n\n# Refreshed externally\n',
      revision: 'skill-r2',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [initial] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(refreshed)

    render(<SkillsView workspace="/books/demo" />)

    expect(await screen.findByRole('heading', { name: 'Initial' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '刷新' }))

    expect(await screen.findByRole('heading', { name: 'Refreshed externally' })).toBeInTheDocument()
    expect(vi.mocked(getSkillDocument)).toHaveBeenCalledTimes(2)
  })

  it('automatically loads an external Skill update when the editor is clean', async () => {
    const initial = skillDocument({
      description: 'Initial',
      content: '---\nname: draft-plan\ndescription: Initial\n---\n\n# Initial\n',
      revision: 'skill-r1',
    })
    const external = skillDocument({
      description: 'External update',
      content: '---\nname: draft-plan\ndescription: External update\n---\n\n# Updated elsewhere\n',
      revision: 'skill-r2',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [initial] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(external)

    render(<SkillsView workspace="/books/demo" />)

    expect(await screen.findByRole('heading', { name: 'Initial' })).toBeInTheDocument()
    window.dispatchEvent(new CustomEvent('nova:skills-updated', { detail: { source: 'another-editor' } }))

    expect(await screen.findByRole('heading', { name: 'Updated elsewhere' })).toBeInTheDocument()
    expect(vi.mocked(saveSkillDocument)).not.toHaveBeenCalled()
  })

  it('automatically rebases and retries a dirty Skill after a revision conflict', async () => {
    const initialContent = '---\nname: draft-plan\ndescription: Initial\n---\n\nExternal section: old\n'
    const localContent = '---\nname: draft-plan\ndescription: Local edit\n---\n\nExternal section: old\n'
    const externalContent = '---\nname: draft-plan\ndescription: Initial\n---\n\nExternal section: updated elsewhere\n'
    const mergedContent = '---\nname: draft-plan\ndescription: Local edit\n---\n\nExternal section: updated elsewhere\n'
    const initial = skillDocument({ content: initialContent, revision: 'skill-r1' })
    const external = skillDocument({ content: externalContent, revision: 'skill-r2' })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [initial] }))
    vi.mocked(getSkillDocument)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(external)
    vi.mocked(saveSkillDocument)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockImplementationOnce(async (scope, name, content) => skillDocument({
        ...external,
        scope,
        name,
        content,
        revision: 'skill-r3',
      }))

    const { container } = render(<SkillsView workspace="/books/demo" />)

    await userEvent.click(await screen.findByRole('button', { name: 'Raw' }))
    const editor = container.querySelector('textarea') as HTMLTextAreaElement
    fireEvent.change(editor, { target: { value: localContent } })
    fireEvent.keyDown(editor, { key: 's', ctrlKey: true })

    await waitFor(() => expect(vi.mocked(saveSkillDocument)).toHaveBeenCalledTimes(2))
    expect(vi.mocked(saveSkillDocument)).toHaveBeenNthCalledWith(1, 'user', 'draft-plan', localContent, undefined, 'skill-r1')
    expect(vi.mocked(saveSkillDocument)).toHaveBeenNthCalledWith(2, 'user', 'draft-plan', mergedContent, undefined, 'skill-r2')
    await waitFor(() => expect(editor.value).toBe(mergedContent))
    expect(screen.getByRole('status')).toHaveAccessibleName('所有更改均已保存')
  })

  it('opens and saves supporting files inside a Skill directory', async () => {
    const user = userEvent.setup()
    const doc = skillDocument({
      name: 'draft-plan',
      description: 'Planning',
      scope: 'user',
      path: '/nova/skills/draft-plan/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: draft-plan\ndescription: Planning\n---\n\n# Draft Plan\n',
      files: [
        { path: 'SKILL.md', size: 64, entry: true, editable: true },
        { path: 'references/style.md', size: 8, entry: false, editable: true },
      ],
    })
    const refDoc = skillFileDocument({
      skill: doc,
      file: { path: 'references/style.md', size: 8, entry: false, editable: true },
      content: '# Style\n',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [doc] }))
    vi.mocked(getSkillDocument).mockResolvedValue(doc)
    vi.mocked(getSkillFileDocument).mockResolvedValue(refDoc)
    vi.mocked(saveSkillFileDocument).mockResolvedValue({
      ...refDoc,
      content: '# Updated\n',
      file: { ...refDoc.file, size: 10 },
    })

    const { container } = render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '目录文件' }))
    await user.click(await screen.findByRole('button', { name: /style\.md/ }))
    await waitFor(() => {
      expect(vi.mocked(getSkillFileDocument)).toHaveBeenCalledWith('user', 'draft-plan', 'references/style.md')
    })
    await user.click(screen.getByRole('button', { name: 'Raw' }))
    const editor = container.querySelector('textarea') as HTMLTextAreaElement
    await waitFor(() => {
      expect(editor.value).toContain('# Style')
    })
    await user.clear(editor)
    await user.type(editor, '# Updated\n')
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'SKILL.md' }))

    await waitFor(() => {
      expect(vi.mocked(saveSkillFileDocument)).toHaveBeenCalledWith('user', 'draft-plan', 'references/style.md', '# Updated\n', 'file-r1')
    })
    expect(vi.mocked(saveSkillDocument)).not.toHaveBeenCalled()
  })

  it('renders Skill markdown by default and switches to raw editing', async () => {
    const user = userEvent.setup()
    const doc = skillDocument({
      name: 'draft-plan',
      description: 'Planning',
      scope: 'user',
      path: '/nova/skills/draft-plan/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: draft-plan\ndescription: Planning\n---\n\n# Draft Plan\n\n- Keep the outline lean\n',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [doc] }))
    vi.mocked(getSkillDocument).mockResolvedValue(doc)

    const { container } = render(<SkillsView workspace="/books/demo" />)

    expect(await screen.findByRole('heading', { name: 'Draft Plan' })).toBeInTheDocument()
    expect(screen.queryByText(/name: draft-plan/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'SKILL.md' })).not.toBeInTheDocument()
    expect(container.querySelector('textarea')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Raw' }))

    const editor = container.querySelector('textarea') as HTMLTextAreaElement
    expect(editor.value).toContain('# Draft Plan')
  })

  it('scans Remote URL sources and installs only selected Skills', async () => {
    const user = userEvent.setup()
    vi.mocked(previewSkillRemoteInstall).mockResolvedValue({
      candidates: [
        { id: 'id-one', name: 'one', description: 'One skill', source_path: 'skills/one', conflict: false },
        { id: 'id-two', name: 'two', description: 'Two skill', source_path: 'skills/two', conflict: false },
      ],
    })
    vi.mocked(installSkillRemote).mockResolvedValue({
      installed: [skillDocument({ name: 'one', description: 'One skill', scope: 'user' })],
    })

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '导入' }))
    await user.type(screen.getByLabelText('远程 URL'), 'owner/repo')
    await user.click(screen.getByRole('button', { name: '扫描' }))

    await waitFor(() => {
      expect(vi.mocked(previewSkillRemoteInstall)).toHaveBeenCalledWith({
        url: 'owner/repo',
        ref: '',
        subdir: '',
        scope: 'user',
      })
    })
    await user.click(screen.getByText('/one').closest('label')!)
    await user.click(screen.getByRole('button', { name: '安装 1 个' }))

    await waitFor(() => {
      expect(vi.mocked(installSkillRemote)).toHaveBeenCalledWith({
        url: 'owner/repo',
        ref: '',
        subdir: '',
        scope: 'user',
        candidateIds: ['id-one'],
      })
    })
  })

  it('scans ZIP uploads and auto-selects a single installable Skill', async () => {
    const user = userEvent.setup()
    const file = new File(['zip'], 'skill.zip', { type: 'application/zip' })
    vi.mocked(previewSkillZipInstall).mockResolvedValue({
      candidates: [
        { id: 'zip-one', name: 'zip-one', description: 'Zip skill', source_path: 'skills/zip-one', conflict: false },
      ],
    })
    vi.mocked(installSkillZip).mockResolvedValue({
      installed: [skillDocument({ name: 'zip-one', description: 'Zip skill', scope: 'user' })],
    })

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '导入' }))
    await user.click(screen.getByRole('radio', { name: 'ZIP' }))
    await user.upload(screen.getByLabelText('ZIP 文件'), file)
    await user.click(screen.getByRole('button', { name: '扫描' }))

    await waitFor(() => {
      expect(vi.mocked(previewSkillZipInstall)).toHaveBeenCalledWith(file, 'user')
    })
    await user.click(screen.getByRole('button', { name: '安装 1 个' }))

    await waitFor(() => {
      expect(vi.mocked(installSkillZip)).toHaveBeenCalledWith(file, 'user', ['zip-one'])
    })
  })

  it('deletes an editable Skill after confirming in the dialog', async () => {
    const user = userEvent.setup()
    const doc = skillDocument({
      name: 'draft-plan',
      description: 'Planning',
      scope: 'user',
      path: '/nova/skills/draft-plan/SKILL.md',
      editable: true,
      active: true,
      content: '---\nname: draft-plan\ndescription: Planning\n---\n\n# Draft Plan\n',
    })
    vi.mocked(getSkills).mockResolvedValue(skillsSnapshot({ skills: [doc] }))
    vi.mocked(getSkillDocument).mockResolvedValue(doc)

    render(<SkillsView workspace="/books/demo" />)

    await user.click(await screen.findByRole('button', { name: '删除' }))
    const dialog = await screen.findByRole('alertdialog', { name: '删除' })
    expect(within(dialog).getByText(/draft-plan/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(vi.mocked(deleteSkillDocument)).toHaveBeenCalledWith('user', 'draft-plan')
    })
  })

  it('restores built-in Skill by deleting the active override', async () => {
    const user = userEvent.setup()
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
    const dialog = await screen.findByRole('alertdialog', { name: '恢复内置' })
    await user.click(within(dialog).getByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(vi.mocked(deleteSkillDocument)).toHaveBeenCalledWith('workspace', 'novel-standard')
    })
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
    revision: 'skill-r1',
    files: [{ path: 'SKILL.md', size: 48, entry: true, editable: true }],
    ...patch,
  }
}

function skillFileDocument(patch: Partial<SkillFileDocument>): SkillFileDocument {
  const baseSkill = skillDocument({})
  return {
    skill: baseSkill,
    file: { path: 'references/style.md', size: 0, entry: false, editable: true },
    content: '',
    revision: 'file-r1',
    ...patch,
  }
}
