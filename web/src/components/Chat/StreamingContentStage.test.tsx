import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StreamingContentStage } from './StreamingContentStage'

describe('StreamingContentStage', () => {
  it('renders one content tree when a newer streaming target is available', () => {
    const renderContent = vi.fn((content: string) => <article>{content}</article>)

    const { container } = render(
      <StreamingContentStage content="已显示" targetContent="已显示的新内容" streaming>
        {renderContent}
      </StreamingContentStage>,
    )

    expect(renderContent).toHaveBeenCalledWith('已显示的新内容')
    expect(container.querySelectorAll('article')).toHaveLength(1)
    expect(screen.getByText('已显示的新内容')).toBeInTheDocument()
  })
})
