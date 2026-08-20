import type { ReactNode } from 'react'
import {
  DetailBlock,
  DetailPre,
  DetailStack,
  EmptyValue,
  ExternalLink,
  fieldMeta,
  formatMaybeJSON,
  formatValue,
  inlinePreview,
  MetaLine,
  numericMeta,
  parseRecord,
  recordArray,
  recordValue,
  stringArray,
  stringValue,
  ToolResourceLink,
  type ToolDetailAdapter,
  type ToolDetailRenderProps,
} from './shared'
import { WorkspacePathText } from './path-text'

export const generalToolDetailAdapters: Record<string, ToolDetailAdapter> = {
  web_search: outputAdapter(renderWebSearchInput, renderWebSearchOutput),
  web_fetch: outputAdapter(renderWebFetchInput, renderWebFetchOutput),
  browser: outputAdapter(renderBrowserInput, renderBrowserOutput),
  skill: outputAdapter(renderSkillInput, renderSkillOutput),
  task: outputAdapter(renderTaskInput, renderTaskOutput),
  script: inputAdapter(renderScriptInput, renderScriptOutput),
  config_read: outputAdapter(renderConfigReadInput, renderConfigReadOutput),
  config_apply: inputAdapter(renderConfigApplyInput, renderConfigApplyOutput),
}

function outputAdapter(renderInput: ToolDetailAdapter['renderInput'], renderOutput: ToolDetailAdapter['renderOutput']): ToolDetailAdapter {
  return { layout: 'output', renderInput, renderOutput }
}

function inputAdapter(renderInput: ToolDetailAdapter['renderInput'], renderOutput: ToolDetailAdapter['renderOutput']): ToolDetailAdapter {
  return { layout: 'input', renderInput, renderOutput }
}

function renderWebSearchInput({ input }: ToolDetailRenderProps) {
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
      <span className="min-w-0 break-words text-[var(--nova-text)]">{stringValue(input.query)}</span>
      <MetaLine items={[fieldMeta('time_range', input.time_range), numericMeta('max_results', input.max_results)]} />
    </div>
  )
}

function renderWebSearchOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response || response.schema !== 'web_search.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const results = recordArray(response.results)
  if (results.length === 0) return <OutcomeMessage response={response} t={t} />
  return (
    <DetailStack>
      {results.map((item, index) => {
        const url = stringValue(item.url)
        return (
          <DetailBlock key={`${url}-${index}`}>
            <div className="font-sans text-xs font-medium leading-4"><ExternalLink href={url}>{stringValue(item.title) || url}</ExternalLink></div>
            <MetaLine items={[stringValue(item.provider) || sourceLabel(url), stringValue(item.published_at)]} />
            {stringValue(item.summary) ? <div className="font-sans text-[11px] leading-4 text-[var(--nova-text-muted)]">{stringValue(item.summary)}</div> : null}
          </DetailBlock>
        )
      })}
      <Warnings value={response.warnings} />
    </DetailStack>
  )
}

function renderWebFetchInput({ input }: ToolDetailRenderProps) {
  const url = stringValue(input.url)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]"><ExternalLink href={url}>{url}</ExternalLink></DetailPre>
      <MetaLine items={[numericMeta('start_index', input.start_index), numericMeta('max_chars', input.max_chars)]} />
    </DetailStack>
  )
}

function renderWebFetchOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response || response.schema !== 'web_fetch.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const finalURL = stringValue(response.final_url) || stringValue(response.url)
  const content = stringValue(response.content) || stringValue(response.excerpt)
  const attempts = recordArray(response.attempts)
  return (
    <DetailStack>
      {(stringValue(response.title) || finalURL) ? (
        <DetailBlock>
          <div className="font-sans text-xs font-medium leading-4"><ExternalLink href={finalURL}>{stringValue(response.title) || finalURL}</ExternalLink></div>
          <MetaLine items={[
            stringValue(response.byline),
            stringValue(response.fetch_method),
            stringValue(response.content_type),
            response.truncated === true ? t('chat.tool.detail.truncated') : '',
          ]} />
        </DetailBlock>
      ) : null}
      {content ? <DetailPre className="text-[var(--nova-text-muted)]">{content}</DetailPre> : null}
      {content ? null : attempts.map((attempt, index) => (
        <DetailBlock key={`${stringValue(attempt.method)}-${index}`} title={stringValue(attempt.method)} tone="danger">
          <div>{[stringValue(attempt.outcome), attempt.http_status ? `HTTP ${attempt.http_status}` : '', stringValue(attempt.message)].filter(Boolean).join(' · ')}</div>
        </DetailBlock>
      ))}
      {!content && attempts.length === 0 ? <OutcomeMessage response={response} t={t} /> : null}
      {!content && attempts.length > 0 ? <MetaLine items={[stringValue(response.status), stringValue(response.suggested_action)]} /> : null}
      {stringValue(response.warning) ? <div className="text-[var(--nova-warning)]">{stringValue(response.warning)}</div> : null}
    </DetailStack>
  )
}

function renderBrowserInput({ input }: ToolDetailRenderProps) {
  const action = stringValue(input.action)
  const command = stringValue(input.command)
  const primary = [action, stringValue(input.tab), command].filter(Boolean).join(' · ')
  const target = stringValue(input.url) || stringValue(input.selector) || stringValue(input.text) || stringValue(input.expression)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{primary}</DetailPre>
      {target ? <DetailPre>{target}</DetailPre> : null}
      <MetaLine items={[
        fieldMeta('key', input.key),
        stringArray(input.values).length ? `values=${stringArray(input.values).join(', ')}` : '',
        input.full_page === true ? 'full_page=true' : '',
        numericMeta('timeout', input.timeout_seconds, 's'),
        input.all === true ? 'all=true' : '',
      ]} />
    </DetailStack>
  )
}

function renderBrowserOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response || response.schema !== 'browser.result.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const observation = recordValue(response.observation)
  const elements = recordArray(observation.elements)
  const screenshot = recordValue(response.screenshot)
  const tabs = stringArray(response.tabs)
  const value = response.value
  return (
    <DetailStack>
      {stringValue(observation.url) ? (
        <DetailBlock>
          <div className="font-sans text-xs font-medium"><ExternalLink href={stringValue(observation.url)}>{stringValue(observation.title) || stringValue(observation.url)}</ExternalLink></div>
          <MetaLine items={[observation.truncated === true ? t('chat.tool.detail.truncated') : '']} />
        </DetailBlock>
      ) : null}
      {stringValue(observation.text) ? <DetailPre>{stringValue(observation.text)}</DetailPre> : null}
      {elements.length ? (
        <DetailStack className="space-y-1">
          {elements.map((element, index) => (
            <div key={`${stringValue(element.ref)}-${index}`} className="grid min-w-0 grid-cols-[max-content_minmax(0,1fr)] gap-x-2">
              <span className="text-[var(--nova-text-faint)]">{stringValue(element.ref)}</span>
              <span className="min-w-0 break-words">{[stringValue(element.role), stringValue(element.name), stringValue(element.selector)].filter(Boolean).join(' · ')}</span>
            </div>
          ))}
        </DetailStack>
      ) : null}
      {value !== undefined ? <DetailPre>{formatValue(value)}</DetailPre> : null}
      {tabs.length ? <DetailPre>{tabs.join('\n')}</DetailPre> : null}
      {stringValue(screenshot.path) ? (
        <DetailBlock title={t('chat.tool.detail.screenshot')}>
          <DetailPre><WorkspacePathText>{stringValue(screenshot.path)}</WorkspacePathText></DetailPre>
          <MetaLine items={[stringValue(screenshot.mime_type), screenshot.bytes ? `${screenshot.bytes} B` : '']} />
        </DetailBlock>
      ) : null}
      {!stringValue(observation.text) && !elements.length && value === undefined && !tabs.length && !stringValue(screenshot.path)
        ? <span>{stringValue(response.status) || t('chat.tool.noReturn')}</span>
        : null}
    </DetailStack>
  )
}

function renderSkillInput({ input, t }: ToolDetailRenderProps) {
  const directName = stringValue(input.name)
  if (directName) return <ConfigLink resource="skill" id={directName}>{directName}</ConfigLink>
  const action = stringValue(input.action)
  const refs = recordArray(input.refs)
  if (action === 'list') {
    return <DetailPre className="text-[var(--nova-text)]">{stringValue(input.query) || t('chat.tool.detail.allSkills')}</DetailPre>
  }
  return (
    <DetailStack className="space-y-1">
      {refs.map((ref, index) => <SkillRefLink key={`${stringValue(ref.id)}-${index}`} refValue={ref} />)}
      <MetaLine items={[action, numericMeta('limit', input.limit)]} />
    </DetailStack>
  )
}

function renderSkillOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response) return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const skills = recordArray(response.skills)
  if (skills.length) {
    return (
      <DetailStack className="space-y-1.5">
        {skills.map((skill, index) => (
          <DetailBlock key={`${stringValue(skill.name)}-${index}`}>
            <ConfigLink resource="skill" id={stringValue(recordValue(skill.ref).id) || stringValue(skill.name)} scope={stringValue(recordValue(skill.ref).source)}>{stringValue(skill.name)}</ConfigLink>
            {stringValue(skill.description) ? <div className="font-sans text-[11px] leading-4 text-[var(--nova-text-muted)]">{stringValue(skill.description)}</div> : null}
          </DetailBlock>
        ))}
      </DetailStack>
    )
  }
  const results = recordArray(response.results)
  if (!results.length) return <EmptyValue t={t} />
  return (
    <DetailStack>
      {results.map((item, index) => {
        const content = recordValue(item.content)
        const ref = recordValue(item.ref)
        const name = stringValue(content.name) || stringValue(ref.id)
        return (
          <DetailBlock key={`${name}-${index}`} tone={stringValue(item.error) ? 'danger' : 'normal'}>
            <div><ConfigLink resource="skill" id={stringValue(ref.id) || name} scope={stringValue(ref.source)}>{name}</ConfigLink></div>
            {stringValue(item.error) ? <DetailPre>{stringValue(item.error)}</DetailPre> : <DetailPre>{stringValue(content.instructions)}</DetailPre>}
          </DetailBlock>
        )
      })}
    </DetailStack>
  )
}

function SkillRefLink({ refValue }: { refValue: Record<string, unknown> }) {
  const id = stringValue(refValue.id)
  const source = stringValue(refValue.source)
  return <ConfigLink resource="skill" id={id} scope={source}>{[source, id].filter(Boolean).join(' · ')}</ConfigLink>
}

function renderTaskInput({ input, t }: ToolDetailRenderProps) {
  const action = stringValue(input.action)
  if (!['start', 'observe', 'steer', 'respond', 'abort'].includes(action)) {
    return <DetailPre>{formatMaybeJSON(JSON.stringify(input))}</DetailPre>
  }
  if (action === 'start') {
    return <TaskStarts values={recordArray(input.starts)} t={t} />
  }
  const refs = recordArray(input.refs)
  return (
    <DetailStack>
      <DetailPre className="text-[var(--nova-text)]">{action}</DetailPre>
      {stringValue(input.input) ? <DetailPre>{stringValue(input.input)}</DetailPre> : null}
      {stringValue(input.reason) ? <DetailPre>{stringValue(input.reason)}</DetailPre> : null}
      {refs.map((ref, index) => <TaskRefLine key={`${stringValue(ref.run)}-${index}`} value={ref} />)}
      {recordArray(input.responses).map((response, index) => (
        <DetailBlock key={`${stringValue(response.interaction_id)}-${index}`} title={stringValue(response.interaction_id)}>
          <TaskRefLine value={recordValue(response.ref)} />
          <DetailPre>{formatValue(response.response)}</DetailPre>
        </DetailBlock>
      ))}
      <MetaLine items={[fieldMeta('cursor', input.cursor)]} />
    </DetailStack>
  )
}

function TaskStarts({ values, t }: { values: Record<string, unknown>[]; t: ToolDetailRenderProps['t'] }) {
  if (!values.length) return <span>{t('chat.tool.detail.taskStart')}</span>
  return (
    <DetailStack>
      {values.map((item, index) => (
        <DetailBlock key={`${stringValue(item.agent)}-${index}`} title={stringValue(item.agent)}>
          <DetailPre className="text-[var(--nova-text)]">{stringValue(item.prompt)}</DetailPre>
          <MetaLine items={[item.detached === true ? t('chat.tool.detail.detached') : '', fieldMeta('idempotency_key', item.idempotency_key)]} />
        </DetailBlock>
      ))}
    </DetailStack>
  )
}

function TaskRefLine({ value }: { value: Record<string, unknown> }) {
  return <MetaLine items={[stringValue(value.agent), fieldMeta('session', value.session), fieldMeta('run', value.run)]} />
}

function renderTaskOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response) return <DetailBlock title={t('chat.subagent.result')}><DetailPre>{result}</DetailPre></DetailBlock>
  const results = recordArray(response.results)
  if (!results.length) return <EmptyValue t={t} />
  return (
    <DetailStack>
      {results.map((item, index) => {
        const task = recordValue(item.task)
        const observation = recordValue(item.observation)
        const observedTask = recordValue(observation.task)
        const visibleTask = Object.keys(task).length ? task : observedTask
        const error = stringValue(item.error)
        const output = stringValue(observation.output) || stringValue(visibleTask.output)
        return (
          <DetailBlock key={`${item.index ?? index}`} title={`#${item.index ?? index}`} tone={error ? 'danger' : 'normal'}>
            {Object.keys(visibleTask).length ? (
              <>
                <TaskRefLine value={recordValue(visibleTask.ref)} />
                <MetaLine items={[stringValue(visibleTask.status), observation.incomplete === true ? t('chat.tool.detail.incomplete') : '', fieldMeta('cursor', observation.cursor)]} />
              </>
            ) : null}
            {output ? <DetailPre>{output}</DetailPre> : null}
            {recordArray(observation.events).length ? <DetailPre>{formatValue(observation.events)}</DetailPre> : null}
            {recordArray(observation.interactions).length ? <DetailPre>{formatValue(observation.interactions)}</DetailPre> : null}
            {error ? <DetailPre>{error}</DetailPre> : null}
          </DetailBlock>
        )
      })}
    </DetailStack>
  )
}

function renderScriptInput({ input, t }: ToolDetailRenderProps) {
  return (
    <DetailStack>
      <DetailBlock title={t('chat.tool.detail.source')}><DetailPre className="text-[var(--nova-text)]">{stringValue(input.source)}</DetailPre></DetailBlock>
      {input.input === undefined ? null : <DetailBlock title={t('chat.tool.detail.scriptInput')}><DetailPre>{formatValue(input.input)}</DetailPre></DetailBlock>}
    </DetailStack>
  )
}

function renderScriptOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response) return result ? <DetailPre>{result}</DetailPre> : <EmptyValue t={t} />
  const diagnostics = recordArray(response.diagnostics)
  const logs = stringArray(response.logs)
  return (
    <DetailStack>
      {response.error !== undefined ? <DetailBlock title={t('chat.tool.detail.error')} tone="danger"><DetailPre>{formatValue(response.error)}</DetailPre></DetailBlock> : null}
      {diagnostics.length ? <DetailBlock title={t('chat.tool.detail.diagnostics')} tone="danger"><DetailPre>{formatValue(diagnostics)}</DetailPre></DetailBlock> : null}
      {logs.length ? <DetailBlock title={t('chat.tool.detail.logs')}><DetailPre>{logs.join('\n')}</DetailPre></DetailBlock> : null}
      {response.error === undefined && diagnostics.length === 0 && logs.length === 0 ? <DetailPre>{formatValue(response)}</DetailPre> : null}
    </DetailStack>
  )
}

function renderConfigReadInput({ input }: ToolDetailRenderProps) {
  const ids = stringArray(input.ids)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{[stringValue(input.operation) || 'list', stringValue(input.resource)].filter(Boolean).join(' · ')}</DetailPre>
      {ids.length ? <DetailPre>{ids.join('\n')}</DetailPre> : null}
      {stringValue(input.query) ? <DetailPre>{stringValue(input.query)}</DetailPre> : null}
      <MetaLine items={[fieldMeta('scope', input.scope), numericMeta('limit', input.limit), fieldMeta('cursor', input.cursor)]} />
    </DetailStack>
  )
}

function renderConfigReadOutput({ input, result, t }: ToolDetailRenderProps) {
  let value: unknown
  try {
    value = JSON.parse(result)
  } catch {
    return <DetailPre>{result}</DetailPre>
  }
  if (Array.isArray(value)) {
    const descriptors = value.map(item => recordValue(item))
    return (
      <DetailStack>
        {descriptors.map((descriptor, index) => (
          <DetailBlock key={`${stringValue(descriptor.name)}-${index}`}>
            <ConfigLink resource={stringValue(descriptor.name)}>{stringValue(descriptor.name)}</ConfigLink>
            {stringValue(descriptor.description) ? <div className="font-sans text-[11px] leading-4">{stringValue(descriptor.description)}</div> : null}
            <MetaLine items={[stringArray(descriptor.scopes).join(', '), stringArray(descriptor.operations).join(', '), stringValue(descriptor.revision_field)]} />
          </DetailBlock>
        ))}
      </DetailStack>
    )
  }
  const response = recordValue(value)
  const resource = stringValue(response.resource) || stringValue(input.resource)
  const scope = stringValue(response.scope) || stringValue(input.scope)
  const items = Array.isArray(response.items) ? response.items : []
  return (
    <DetailStack>
      {items.map((item, index) => {
        const record = recordValue(item)
        const id = resourceItemID(record)
        return (
          <DetailBlock key={`${id}-${index}`}>
            {id ? <div><ConfigLink resource={resource} id={id} scope={scope}>{id}</ConfigLink></div> : null}
            <DetailPre>{formatValue(item)}</DetailPre>
          </DetailBlock>
        )
      })}
      {stringArray(response.missing_ids).length ? <DetailBlock title={t('chat.tool.detail.missing')} tone="danger"><DetailPre>{stringArray(response.missing_ids).join('\n')}</DetailPre></DetailBlock> : null}
      {recordArray(response.failures).map((failure, index) => (
        <DetailBlock key={`${stringValue(failure.id)}-${index}`} title={stringValue(failure.id)} tone="danger"><DetailPre>{stringValue(failure.message)}</DetailPre></DetailBlock>
      ))}
      {response.truncated === true ? <MetaLine items={[t('chat.tool.detail.partial'), fieldMeta('next_cursor', response.next_cursor), `${response.returned ?? response.processed ?? items.length}/${response.total ?? '?'}`]} /> : null}
      {!items.length && !stringArray(response.missing_ids).length && !recordArray(response.failures).length ? <EmptyValue t={t} /> : null}
    </DetailStack>
  )
}

function renderConfigApplyInput({ input, t }: ToolDetailRenderProps) {
  const resource = stringValue(input.resource)
  const id = stringValue(input.id)
  const operation = stringValue(input.operation)
  return (
    <DetailStack>
      {input.value === undefined ? <span className="text-[var(--nova-text-faint)]">{t('chat.tool.detail.noValue')}</span> : <DetailPre className="text-[var(--nova-text)]">{formatValue(input.value)}</DetailPre>}
      <div className="border-t border-[var(--nova-border)] pt-1.5">
        <MetaLine items={[
          operation,
          resource,
          id,
          fieldMeta('scope', input.scope),
          fieldMeta('revision', input.revision),
        ]} />
      </div>
    </DetailStack>
  )
}

function renderConfigApplyOutput({ result }: ToolDetailRenderProps) {
  const receipt = parseRecord(result)
  if (!receipt || receipt.schema !== 'config.mutation_receipt.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const resource = stringValue(receipt.resource)
  const id = stringValue(receipt.id)
  return (
    <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5">
      <span>{stringValue(receipt.status)}</span>
      <span>{stringValue(receipt.operation)}</span>
      {id ? <ConfigLink resource={resource} id={id}>{id}</ConfigLink> : null}
      {stringValue(receipt.revision) ? <span className="text-[var(--nova-text-faint)]">revision={inlinePreview(stringValue(receipt.revision))}</span> : null}
    </div>
  )
}

function ConfigLink({ resource, id, scope, children }: { resource: string; id?: string; scope?: string; children: ReactNode }) {
  if (!resource) return <span>{children}</span>
  return <ToolResourceLink target={{ kind: 'config_resource', resource, id, scope }}>{children}</ToolResourceLink>
}

function OutcomeMessage({ response, t }: { response: Record<string, unknown>; t: ToolDetailRenderProps['t'] }) {
  const message = stringValue(response.message) || stringValue(response.suggested_action)
  return (
    <DetailStack className="space-y-1">
      <span>{message || t('chat.tool.noReturn')}</span>
      <Warnings value={response.warnings} />
    </DetailStack>
  )
}

function Warnings({ value }: { value: unknown }) {
  const warnings = stringArray(value)
  return warnings.length ? <DetailPre className="text-[var(--nova-warning)]">{warnings.join('\n')}</DetailPre> : null
}

function sourceLabel(url: string) {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}

function resourceItemID(value: Record<string, unknown>) {
  return stringValue(value.id) || stringValue(value.name) || stringValue(value.key)
}
