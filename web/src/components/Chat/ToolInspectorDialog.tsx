import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { DialogClose, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { Badge } from '@/components/ui/badge'
import type { InspectableToolMessage } from './ToolInspector'
import { ContextCopyButton } from './ContextCopyButton'
import { ToolStatusIcon } from './message-tool-status'
import { toolDisplayName } from './tool-display-name'
import { hasSpecializedToolDetail, ToolCallDetailBody } from './message-tool-detail'
import { stripToolResultMetadata } from './message-tool'
import { decodeToolResultEnvelope } from '@/lib/tool-result-envelope'
import { DetailPre } from './tool-detail/shared'
import { ChapterIllustrationBlock, InteractiveImageBlock } from './message-media'
import { TodoListBlock } from './message-todo'
import { toolCallRenderer } from '@/lib/tool-presentation'
import { ToolRawData } from './ToolRawData'
import { formatToolJSON, inspectToolMessage } from './tool-inspection'
import { focusDialogContentOnOpen } from './dialog-focus'

export default function ToolInspectorDialog({ message, projectId }: { message: InspectableToolMessage; projectId: string }) {
  const { t } = useTranslation()
  const inspected = inspectToolMessage(message)
  const { name, callId, input, output, interaction, truncated } = inspected
  const resultBody = stripToolResultMetadata(output || '')
  const severity = message.status === 'error' ? 'error' : decodeToolResultEnvelope(resultBody)?.severity || 'success'
  const status = message.status || (interaction?.status === 'pending' ? 'running' : interaction?.status === 'cancelled' ? 'cancelled' : interaction ? 'success' : 'running')
  const outputEmpty = status === 'running' ? t('chat.tool.inspector.awaitingOutput') : t('chat.tool.inspector.noOutput')
  const title = toolDisplayName(name, t)
  let readable
  if (interaction) {
    readable = (
      <div className="flex flex-col gap-4">
        {interaction.questions.map((question) => (
          <section key={question.id} className="flex flex-col gap-2">
            <h3 className="font-medium">{question.question}</h3>
            {question.options?.map((option) => <p className="m-0 text-muted-foreground" key={option.id}>{option.label}{option.description ? ` · ${option.description}` : ''}</p>)}
          </section>
        ))}
        {interaction.approval && <DetailPre>{interaction.approval.command || interaction.approval.details}</DetailPre>}
        {interaction.answers?.length ? <DetailPre>{formatToolJSON(JSON.stringify(interaction.answers)).text}</DetailPre> : null}
      </div>
    )
  } else if (message.role !== 'ask' && message.illustration) {
    readable = <ChapterIllustrationBlock message={message} projectId={projectId} />
  } else if (message.role !== 'ask' && (message.interactive_image || message.interactive_images?.length)) {
    readable = <InteractiveImageBlock message={message} projectId={projectId} />
  } else if (message.role === 'tool_call' && !message.streaming && toolCallRenderer(message) === 'todo') {
    readable = <TodoListBlock message={message} />
  } else if (!message.streaming && hasSpecializedToolDetail(name)) {
    readable = <ToolCallDetailBody name={name} rawArgs={input || ''} result={resultBody} resultSeverity={severity} layout="inspector" />
  } else {
    readable = (
      <div className="flex flex-col gap-5">
        <section className="flex flex-col gap-2"><h3 className="font-medium">{t('chat.tool.inspector.input')}</h3><DetailPre>{formatToolJSON(input || '').text || t('chat.tool.inspector.noInput')}</DetailPre></section>
        <section className="flex flex-col gap-2"><h3 className="font-medium">{t('chat.tool.inspector.output')}</h3><DetailPre>{formatToolJSON(output || '').text || outputEmpty}</DetailPre></section>
      </div>
    )
  }

  return (
    <DialogContent
      showCloseButton={false}
      tabIndex={-1}
      onOpenAutoFocus={focusDialogContentOnOpen}
      className="flex h-[86dvh] max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-6xl flex-col gap-0 overflow-hidden p-0 sm:w-[90vw]"
      data-nova-tool-inspector
    >
      <DialogHeader className="shrink-0 border-b px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <ToolStatusIcon status={status} warning={severity === 'warning'} />
          <DialogTitle className="min-w-0 flex-1 truncate" title={name}>{title}</DialogTitle>
          <Badge variant="secondary">{t(`chat.tool.inspector.status.${status}`)}</Badge>
          <DialogClose asChild><TooltipIconButton size="icon-sm" label={t('common.close')}><X /></TooltipIconButton></DialogClose>
        </div>
        <DialogDescription className="flex min-w-0 items-center gap-2 text-xs">
          <span className="min-w-0 flex-1 truncate" title={callId}>{name}{callId ? ` · ${callId}` : ''}</span>
          {callId && <ContextCopyButton content={callId} label={t('chat.tool.inspector.copyId')} copiedLabel={t('chat.contextAnalysis.copied')} failedLabel={t('chat.contextAnalysis.copyFailed')} />}
        </DialogDescription>
      </DialogHeader>
      <Tabs defaultValue="detail" className="min-h-0 flex-1 gap-0">
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2">
          <TabsList>
            <TabsTrigger value="detail">{t('chat.tool.inspector.detail')}</TabsTrigger>
            <TabsTrigger value="raw">{t('chat.tool.inspector.raw')}</TabsTrigger>
          </TabsList>
          {truncated && <Badge variant="outline">{t('chat.tool.inspector.truncated')}</Badge>}
        </div>
        <TabsContent value="detail" className="min-h-0 overflow-y-auto p-4" data-nova-tool-inspector-readable>{readable}</TabsContent>
        <TabsContent value="raw" className="min-h-0 overflow-hidden data-[state=active]:flex data-[state=active]:flex-col">
          <p className="m-0 shrink-0 px-4 py-2 text-xs leading-5 text-muted-foreground">{t(interaction ? 'chat.tool.inspector.interactionSource' : 'chat.tool.inspector.source')}</p>
          <div className="flex min-h-0 flex-1 flex-col gap-3 px-4 pb-4">
            {interaction ? (
              <ToolRawData title={t('chat.tool.inspector.interaction')} source={JSON.stringify(interaction, null, 2)} emptyLabel={t('chat.tool.inspector.noInput')} />
            ) : (
              <>
                <ToolRawData title={t('chat.tool.inspector.input')} source={input} preview={inspected.inputPreview} emptyLabel={t('chat.tool.inspector.noInput')} />
                <ToolRawData title={t('chat.tool.inspector.output')} source={output} emptyLabel={outputEmpty} />
              </>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </DialogContent>
  )
}
