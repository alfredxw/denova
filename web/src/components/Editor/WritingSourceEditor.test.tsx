import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { ProjectFileDocument } from '@/lib/api-client/project-files'
import { WritingSourceEditor } from './WritingSourceEditor'

vi.mock('@/features/files/ProjectSourceEditor', () => ({
  ProjectTextEditor: ({ document, value, onChange }: {
    document: ProjectFileDocument
    value: string
    onChange: (value: string) => void
  }) => (
    <textarea
      aria-label={`source:${document.path}`}
      defaultValue={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}))

describe('WritingSourceEditor', () => {
  it('autosaves the exact source text through the shared revision-aware lane', async () => {
    const onSave = vi.fn().mockResolvedValue({ revision: 'r2' })
    const document: ProjectFileDocument = {
      project_id: 'project-source',
      path: 'data/events.jsonl',
      content: '{"event":1}\nnot-json',
      revision: 'r1',
      kind: 'text',
      mime_type: 'application/x-ndjson',
      size: 20,
      editable: true,
    }

    render(
      <WritingSourceEditor
        projectId="project-source"
        document={document}
        onSave={onSave}
        autoSaveDelayMs={1}
      />,
    )

    fireEvent.change(screen.getByLabelText('source:data/events.jsonl'), {
      target: { value: '{"event":2}\nnot-json\n' },
    })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(
      'data/events.jsonl',
      '{"event":2}\nnot-json\n',
      'r1',
    ))
  })
})
