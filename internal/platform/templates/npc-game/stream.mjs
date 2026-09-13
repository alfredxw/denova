// Consume the scoped run stream. Reconnecting starts from a fresh snapshot,
// replacing partial text, so replayed deltas never duplicate a reply.
export async function followRun(client, runId, onText) {
  const response = await client.request(`/agents/runs/${runId}/events`, { responseType: 'stream', headers: { Accept: 'text/event-stream' } });
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '', text = '', kind = '', data = [];
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) throw new Error('Run stream disconnected before completion');
      buffer += decoder.decode(chunk.value, { stream: true });
      let end;
      while ((end = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, end).replace(/\r$/, ''); buffer = buffer.slice(end + 1);
        if (line.startsWith('event:')) kind = line.slice(6).trim();
        else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
        else if (!line && data.length) {
          const payload = JSON.parse(data.join('\n')); data = [];
          switch (kind) {
            case 'snapshot':
              text = payload.snapshot.text ?? ''; onText(text);
              if (!['accepted', 'running', 'waiting'].includes(payload.snapshot.status)) return payload.snapshot;
              break;
            case 'delta': text += payload.delta; onText(text); break;
            case 'result': onText(payload.text ?? text); return payload;
            case 'state': case 'interaction': break;
            case 'error': throw Object.assign(new Error('Run stream failed'), payload);
            default: throw new Error(`Unsupported run event: ${kind}`);
          }
          kind = '';
        }
      }
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}
