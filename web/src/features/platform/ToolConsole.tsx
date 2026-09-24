import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { consumer, management, platformError, type Manifest, type RuntimeSnapshot } from './api'

/** Tool invocation belongs to extension testing; Agent conversations stay in the workbench. */
export function ToolConsole({ runtime, manifest, onStop, onFeedback }: {
  runtime: RuntimeSnapshot
  manifest: Manifest
  onStop: () => void
  onFeedback?: (feedback: string) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const tools = manifest.contributes?.tools ?? []
  const [selection, setSelection] = useState(tools[0]?.id ?? '')
  const [input, setInput] = useState('{}')
  const [output, setOutput] = useState('')
  const [busy, setBusy] = useState(false)
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    try { await action() } catch (error) {
      console.error('[extensions] tool test failed', error)
      const result = `Tool test failed: ${error instanceof Error ? error.message : String(error)}`
      setOutput(result)
      onFeedback?.(result)
      toast.error(platformError(error))
    }
    finally { setBusy(false) }
  }
  return (
    <div className="flex flex-col gap-3 rounded-lg border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t('platform.toolConsole')}</h3>
        <Button variant="outline" disabled={busy} onClick={() => void run(async () => {
          await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
          onStop()
        })}>{t('platform.stop')}</Button>
      </div>
      {selection && <>
        <Field>
          <FieldLabel htmlFor={id + '-tool'}>{t('platform.tool')}</FieldLabel>
          <Select value={selection} onValueChange={setSelection} disabled={busy}>
            <SelectTrigger id={id + '-tool'} className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>{tools.map(tool => <SelectItem key={tool.id} value={tool.id}>{tool.id}</SelectItem>)}</SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={id + '-input'}>{t('platform.toolInput')}</FieldLabel>
          <Textarea id={id + '-input'} value={input} onChange={event => setInput(event.target.value)} disabled={busy} />
        </Field>
        <Button disabled={busy} onClick={() => void run(async () => {
          const result = await consumer(runtime, `/tools/${manifest.id}/${selection}/invoke`, 'POST', { input: JSON.parse(input) })
          setOutput(JSON.stringify(result, null, 2))
          onFeedback?.(`Tool ${manifest.id}/${selection}\nInput: ${input}\nResult: ${JSON.stringify(result)}`)
        })}>{t('platform.send')}</Button>
        <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words text-sm" aria-live="polite">{output}</pre>
      </>}
    </div>
  )
}
