import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
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

    expect(screen.getByText('SKILL.md')).toBeInTheDocument()
    expect(screen.getByText('guide.md')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '新建文件' })).not.toBeInTheDocument()

    await user.click(screen.getByText('guide.md'))
    expect(onSelectFile).toHaveBeenCalledWith('references/guide.md')
  })
})
