import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import { exchange } from './api'

type ResourceFiles = {
  files: { path: string; bytes: number }[]
  content?: string
  path?: string
  truncated: boolean
  binary: boolean
}

// Read content fields from validated payloads; other files remain available as source.
function readableContent(result: ResourceFiles): string | undefined {
  if (!result.path || result.binary || result.truncated) return
  if (/\.(md|txt)$/i.test(result.path)) return result.content?.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '')
  if (!result.path.endsWith('.json')) return
  try {
    const value = JSON.parse(result.content || '')
    if (typeof value?.content === 'string') return value.content
    if (typeof value?.prompt === 'string') return value.prompt
    if (Array.isArray(value?.slots)) return value.slots.map((slot: { name?: string; content?: string }) =>
      typeof slot.content === 'string' ? `${slot.name ? `### ${slot.name}\n\n` : ''}${slot.content}` : '',
    ).filter(Boolean).join('\n\n') || undefined
  } catch { /* A truncated or non-JSON file is still readable as source. */ }
}

export function ResourcePreview({ previewID, candidateID, resourceID }: {
  previewID: string
  candidateID: string
  resourceID: string
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState('')
  const [result, setResult] = useState<ResourceFiles>()
  const [error, setError] = useState('')
  useEffect(() => {
    let alive = true
    setResult(undefined)
    setError('')
    const query = new URLSearchParams({ candidate_id: candidateID, resource_id: resourceID, path: selected })
    void exchange<ResourceFiles>(`/previews/${previewID}/files?${query}`).then((value) => {
      if (!alive) return
      if (!selected && value.files.length) {
        setSelected(value.files.find((file) => file.path === 'SKILL.md')?.path || value.files[0].path)
      } else setResult(value)
    }).catch((cause) => {
      console.error('[market] failed to read resource preview', cause)
      if (alive) setError(t('market.errors.operationFailed'))
    })
    return () => { alive = false }
  }, [previewID, candidateID, resourceID, selected, t])
  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!result) return <p role="status" className="text-sm text-muted-foreground">{t('market.contents.loading')}</p>
  const content = readableContent(result)
  const source = <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 text-xs [overflow-wrap:anywhere]">{result.content}</pre>
  return <div className="min-w-0 space-y-3">
    {result.files.length > 1 && <div className="flex max-h-40 flex-col overflow-auto rounded-md border">
      {result.files.map((file) => <Button key={file.path} variant={selected === file.path ? 'secondary' : 'ghost'} className="h-auto justify-start whitespace-normal p-2 text-left text-xs [overflow-wrap:anywhere]" onClick={() => setSelected(file.path)}>{file.path}</Button>)}
    </div>}
    {result.binary ? <p className="text-sm text-muted-foreground">{t('market.import.binaryFile')}</p> : content ? <>
      <ThemedMarkdownRenderer content={content} className="text-sm leading-7 [overflow-wrap:anywhere]" components={{ img: ({ alt }) => <span>{alt}</span> }} />
      <details className="text-xs text-muted-foreground"><summary className="cursor-pointer py-2">{t('market.contents.source')}</summary>{source}</details>
    </> : source}
    {result.truncated && <p className="text-xs text-muted-foreground">{t('market.import.truncated')}</p>}
  </div>
}
