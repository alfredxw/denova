import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ProjectMarkdownPreview, resolveMarkdownProjectPath } from './ProjectMarkdownPreview'

describe('ProjectMarkdownPreview', () => {
  it('resolves project-relative images and opens relative files inside Files', () => {
    const onOpenFile = vi.fn()
    render(
      <ProjectMarkdownPreview
        projectId="project one"
        path="docs/README.md"
        content={'![Cover](../art/cover%20image.png)\n\n[Guide](./guide.md)'}
        onOpenFile={onOpenFile}
      />,
    )

    expect(screen.getByRole('img', { name: 'Cover' })).toHaveAttribute(
      'src',
      '/api/projects/project%20one/files/asset?path=art%2Fcover+image.png',
    )
    fireEvent.click(screen.getByRole('link', { name: 'Guide' }))
    expect(onOpenFile).toHaveBeenCalledWith('docs/guide.md')
  })

  it('does not turn external, fragment, or escaping links into project paths', () => {
    expect(resolveMarkdownProjectPath('docs/README.md', 'https://example.com/a.md')).toBeNull()
    expect(resolveMarkdownProjectPath('docs/README.md', '#section')).toBeNull()
    expect(resolveMarkdownProjectPath('README.md', '../outside.md')).toBeNull()
    expect(resolveMarkdownProjectPath('docs/README.md', '/root.md')).toBe('root.md')
  })
})
