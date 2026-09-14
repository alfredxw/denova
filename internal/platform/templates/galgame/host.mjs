// Thin HTTP conveniences, not a game SDK. Durable commands always keep their ID.
export function host(client) {
  const request = client.request.bind(client);
  const json = (path, method, body) => request(path, { method, body: JSON.stringify(body) });
  const address = (key, turn, branchId) => ({ key, ...(turn ? { turnId: turn.id, sourceRevision: turn.revision, branchId } : {}) });
  return {
    snapshot: () => request('/story'),
    history: (branchId, beforeCursor) => request(`/story/history?${new URLSearchParams({ branchId, beforeCursor, limit: '50' })}`),
    command: command => json('/story/commands', 'POST', command),
    record: (key, turn, branchId) => request(`/story/records?${new URLSearchParams(address(key, turn, branchId))}`),
    save: (key, record, value, turn, branchId) => json('/story/records', 'PUT', { ...address(key, turn, branchId), expectedRevision: record.revision, schemaVersion: 1, value }),
    followStory: (operationId, onEvent) => followStory(client, operationId, onEvent),
    library: (query, offset = 0) => request(`/library/items?${new URLSearchParams({ query, offset: String(offset), limit: '30' })}`),
    item: id => request(`/library/items/${encodeURIComponent(id)}`),
    image: pending => json('/images/generations', 'POST', { commandId: pending.commandId, modelSlot: 'illustrator', prompt: pending.prompt, aspectRatio: pending.kind === 'portrait' ? '2:3' : '16:9' }),
    imageResult: id => request(`/images/generations/${encodeURIComponent(id)}`),
    cancelImage: id => json(`/images/generations/${encodeURIComponent(id)}/cancel`, 'POST', {}),
    async asset(asset) {
      const response = await request(`/assets/content?${new URLSearchParams(asset)}`, { responseType: 'stream' });
      return URL.createObjectURL(await response.blob());
    },
  };
}

async function followStory(client, operationId, onEvent) {
  const response = await client.request(`/story/events?${new URLSearchParams({ operationId })}`, { responseType: 'stream', headers: { Accept: 'text/event-stream' } });
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '', data = [];
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) throw new Error('Story stream disconnected before settlement');
      buffer += decoder.decode(chunk.value, { stream: true });
      let end;
      while ((end = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, end).replace(/\r$/, ''); buffer = buffer.slice(end + 1);
        if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
        else if (!line && data.length) {
          const payload = JSON.parse(data.join('\n')); data = [];
          if (payload.kind === 'settled') return;
          if (!['delta', 'reset'].includes(payload.kind)) throw new Error('Unsupported Story stream event');
          onEvent(payload);
        }
      }
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}

export const nextPoll = () => new Promise(resolve => setTimeout(resolve, 1500));
