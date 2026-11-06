// Exercise the real Configuration tools and saved catalog; only the model is deterministic.
export function narrativeImportCompletion(body) {
  const messages = body.messages ?? []
  const text = message => typeof message.content === 'string' ? message.content : (message.content ?? []).map(part => part.text ?? '').join('')
  const index = messages.findLastIndex(message => message.role === 'user')
  if (index < 0 || !text(messages[index]).includes('[Narrative preset import]') || !text(messages[index]).includes('E2E_NARRATIVE_IMPORT')) return null
  const source = text(messages[index])
  const match = source.match(/\[Narrative preset import\]\n(\{[^\n]+\})/)
  if (!match) return { content: 'Import payload is missing.' }
  const payload = JSON.parse(match[1])
  if (source.includes('IGNORED_EXTENSION') || source.includes('UNSELECTED_RULE') || payload.prompts.map(prompt => prompt.identifier).join(',') !== 'voice,style,apply') {
    return { content: 'Import selection or source isolation failed.' }
  }
  const contextMarker = '[Configuration Page Context]\n'
  const contextStart = source.indexOf(contextMarker)
  if (contextStart < 0) return { content: 'Configuration page context is missing.' }
  const page = JSON.parse(source.slice(contextStart + contextMarker.length))
  if (page.origin !== 'presets' || page.resource !== 'narrative_style' || !page.resource_id || page.resource_id === 'teller'
    || page.page_context?.active_resource_kind || page.page_context?.active_resource_id) {
    return { content: 'Configuration page context did not resolve the selected tool resource.' }
  }
  for (const name of ['config_read', 'config_apply']) {
    const resources = body.tools?.find(tool => tool.function?.name === name)?.function?.parameters?.properties?.resource?.enum
    if (!resources?.includes(page.resource) || resources.includes('teller')) {
      return { content: 'Configuration tool schema does not expose canonical resource names.' }
    }
  }
  const history = messages.slice(index + 1)
  const result = id => history.find(message => message.role === 'tool' && message.tool_call_id === id)
  const call = (tool, args, id) => ({ tool, arguments: JSON.stringify(args), id })
  if (!result('import-reference')) return call('read', { path: 'skill://configuration/references/narrative-import.md' }, 'import-reference')
  if (!text(result('import-reference')).includes('originally_enabled')) return { content: 'Import Skill reference is unavailable.' }
  if (!result('style-reference')) return call('read', { path: 'skill://configuration/references/narrative-style.md' }, 'style-reference')
  if (!result('style-describe')) return call('config_read', { operation: 'describe', resource: page.resource }, 'style-describe')
  if (!result('style-current')) return call('config_read', { operation: 'get', resource: page.resource, ids: [page.resource_id] }, 'style-current')
  if (JSON.parse(text(result('style-current'))).items?.[0]?.id !== page.resource_id) return { content: 'Selected preset read failed.' }
  if (!result('style-create')) return call('config_apply', {
    operation: 'create', resource: page.resource, scope: 'user',
    value: {
      ...payload.target, description: `Imported from ${payload.source_filename}`,
      slots: [
        { id: 'dialogue', name: '对白', target: 'system', enabled: true, content: 'E2E_IMPORTED_DIALOGUE: At least 40% of the prose must be direct dialogue.' },
        { id: 'viewpoint', name: '白描', target: 'turn_context', enabled: true, content: 'E2E_IMPORTED_VIEWPOINT: Convey emotion through concrete gestures; do not reveal unexpressed thoughts.' },
      ],
    },
  }, 'style-create')
  if (!result('style-verify')) return call('config_read', { operation: 'get', resource: page.resource, ids: [payload.target.id] }, 'style-verify')
  const saved = JSON.parse(text(result('style-verify'))).items?.[0]
  return { content: saved?.slots?.length === 2 ? 'E2E narrative import verified.' : 'Import verification failed.' }
}
