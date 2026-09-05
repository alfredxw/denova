import { useMemo, useRef, useState } from 'react'
import { ChevronDown, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { editor } from 'monaco-editor'
import { DenovaMonacoEditor } from '@/components/monaco/DenovaMonaco'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { ContextCopyButton } from './ContextCopyButton'
import { formatToolJSON } from './tool-inspection'

/** Formatting is a view projection; copy always uses the untouched source. */
export function ToolRawData({ title, source, emptyLabel, preview = false }: { title: string; source?: string; emptyLabel: string; preview?: boolean }) {
  const { t } = useTranslation()
  const [formatMode, setFormatMode] = useState('json')
  const [expanded, setExpanded] = useState(true)
  const formatted = useMemo(() => formatToolJSON(source || ''), [source])
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const value = formatMode === 'json' ? formatted.text : source || ''
  return (
    <Collapsible open={expanded} onOpenChange={setExpanded} className={cn('flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border', expanded ? 'flex-1' : 'shrink-0')}>
      <div className="flex min-h-11 shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5">
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm" className="mr-auto min-w-0 flex-1 justify-start" aria-label={t(expanded ? 'chat.tool.inspector.collapseSection' : 'chat.tool.inspector.expandSection', { section: title })}>
            <ChevronDown className={cn('transition-transform', !expanded && '-rotate-90')} />
            {title}
          </Button>
        </CollapsibleTrigger>
        {preview && <span className="text-xs text-muted-foreground">{t('chat.tool.inspector.inputPreview')}</span>}
        {formatted.language === 'json' && (
          <ToggleGroup type="single" size="sm" value={formatMode} onValueChange={(value) => value && setFormatMode(value)} aria-label={t('chat.tool.inspector.format')}>
            <ToggleGroupItem value="json">JSON</ToggleGroupItem>
            <ToggleGroupItem value="text">{t('chat.tool.inspector.original')}</ToggleGroupItem>
          </ToggleGroup>
        )}
        <TooltipIconButton label={t('chat.tool.inspector.find')} disabled={!source || !expanded} onClick={() => void editorRef.current?.getAction('actions.find')?.run()}>
          <Search />
        </TooltipIconButton>
        <ContextCopyButton content={source || ''} label={t('chat.tool.inspector.copyOriginal')} copiedLabel={t('chat.contextAnalysis.copied')} failedLabel={t('chat.contextAnalysis.copyFailed')} />
      </div>
      <CollapsibleContent className="min-h-0 flex-1" data-nova-tool-raw={title}>
        {source ? (
          <DenovaMonacoEditor
            value={value}
            language={formatted.language}
            loading={<div className="p-3 text-sm text-muted-foreground">{t('common.loading')}</div>}
            onMount={(instance) => { editorRef.current = instance }}
            options={{
              readOnly: true, domReadOnly: true, automaticLayout: true,
              minimap: { enabled: false }, scrollBeyondLastLine: false,
              wordWrap: 'on', fontSize: 12, lineNumbersMinChars: 3,
              folding: true, stickyScroll: { enabled: false },
              renderLineHighlight: 'none', ariaLabel: title,
            }}
          />
        ) : <p className="m-0 p-3 text-sm text-muted-foreground">{emptyLabel}</p>}
      </CollapsibleContent>
    </Collapsible>
  )
}
