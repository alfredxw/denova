import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { fileTreeRow } from '@/test/file-tree'
import { SkillFileTree } from './SkillFileTree'

describe('SkillFileTree', () => {
  it('browses original Skill file names without project operation controls', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    render(
      <SkillFileTree
        nodes={[
          { name: 'SKILL.md', type: 'file' },
          { name: 'references', type: 'dir', children: [{ name: 'guide.md', type: 'file' }] },
        ]}
        selectedFile="SKILL.md"
        defaultExpandedPaths={['references']}
        onSelectFile={onSelectFile}
      />,
    )

    expect(fileTreeRow('SKILL.md')).toBeInTheDocument()
    expect(fileTreeRow('references/guide.md')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '新建文件' })).not.toBeInTheDocument()

    await user.click(fileTreeRow('references/guide.md'))
    expect(onSelectFile).toHaveBeenCalledWith('references/guide.md')
  })
})
