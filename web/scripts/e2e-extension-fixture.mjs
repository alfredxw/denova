// Deterministic providers for the extension composition journey. No paid API.
export const extensionImage = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aV1kAAAAASUVORK5CYII=';

// The extension preview uses the real default adapt-template Story contract.
// Its fixture must finalize that contract and fill the returned required fields
// before submitting prose, exactly as a real model is instructed to do.
export function extensionOpening(initializationResults, submissionResults) {
  if (submissionResults.length) {
    throw new Error('Extension opening submission was rejected: ' + JSON.stringify(submissionResults.at(-1)));
  }
  if (!initializationResults.length) {
    return {
      tool: 'initialize_story_state_schema',
      input: {
        summary: 'Retain the default templates for the train station opening.',
        items: [{
          item_id: 'extension-opening-review',
          requirements: [{
            source: { kind: 'opening', id: 'opening-draft' },
            requirement: 'Track the station event as the traveler enters.',
            value_policy: 'schema_only', expected_type: 'string', decision: 'covered',
            template_id: 'story_context', field_id: '当前事件',
          }],
          adaptation: { template_ops: [] },
        }],
        finalize: true,
      },
    };
  }
  const receipt = initializationResults.at(-1);
  if (!receipt.finalized || !Array.isArray(receipt.initialization_guide?.required_state_changes)) {
    throw new Error('Extension opening schema initialization was rejected: ' + JSON.stringify(receipt));
  }
  const values = {
    姓名: 'Alex', 基本身份: 'A traveler arriving by train', 外貌与装扮: 'A blue coat and a small suitcase',
    性格与背景: 'A curious traveler visiting the old station', 当前处境: 'Standing outside the station entrance',
    当前时间: 'Early evening', 当前详细地点: 'The old train station entrance',
    当前事件: 'The stone door opens and reveals the station platform',
    可承接钩子: 'A station bell rings beyond the open door', 世界局势: 'The station is quiet as evening approaches',
  };
  const stateChanges = receipt.initialization_guide.required_state_changes.map(field => {
    // Default preset fields without defaults are textual. Fail explicitly if a
    // changed preset needs another type, instead of fabricating invalid values
    // and keeping the model fixture in an unbounded repair loop.
    if (field.type !== 'string' || typeof values[field.field_id] !== 'string') {
      throw new Error('Extension opening fixture needs a value for ' + JSON.stringify(field));
    }
    return { op: 'replace', actor_id: field.actor_id, field_id: field.field_id, value: values[field.field_id] };
  });
  return {
    content: 'Alex arrives at the old train station in a blue coat, carrying a small suitcase. The stone door opens as the evening bell rings.',
    submission: {
      state_changes: stateChanges,
      choices: ['Enter the station', 'Read the timetable', 'Listen to the bell', 'Inspect the door', 'Look along the platform'],
      plan_update: { mode: 'replace_document', markdown: '## Current direction\n\nExplore the old station and the ringing bell while leaving the traveler free to depart.' },
    },
  };
}

export function extensionScene(body) {
  if (!JSON.stringify(body.messages).includes('You arrange a committed story turn for a visual novel.')) return null;
  for (const message of [...body.messages].reverse()) {
    if (message.role !== 'user') continue;
    const text = typeof message.content === 'string' ? message.content : (message.content ?? []).filter(part => part.type === 'text').map(part => part.text).join('\n');
    const start = text.indexOf('{'), end = text.lastIndexOf('}');
    if (start < 0 || end < start) continue;
    try {
      const input = JSON.parse(text.slice(start, end + 1));
      if (typeof input.narrative !== 'string') continue;
      return JSON.stringify({ title: 'The quiet platform', background: 'A quiet train station at dusk, watercolor sky.', beats: [{ speaker: 'narrator', text: input.narrative, expression: 'neutral', cg: 'A sealed letter on a wooden station bench.' }] });
    } catch { /* Other user context fragments are not presentation requests. */ }
  }
  throw new Error('Visual novel presenter request did not contain the committed narrative');
}
