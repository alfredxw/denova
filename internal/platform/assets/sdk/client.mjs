// Optional convenience wrapper. All platform operations are ordinary HTTP.
export function connect({ prepareExit } = {}) {
  const parentOrigin = new URL(document.referrer || location.ancestorOrigins?.[0]).origin;
  const nonce = crypto.randomUUID();
  return new Promise((resolve) => {
    function receive(event) {
      if (event.source !== parent || event.origin !== parentOrigin ||
          event.data?.type !== 'denova:bootstrap' || event.data.nonce !== nonce) return;
      window.removeEventListener('message', receive);
      const { connection, context } = event.data;
      window.addEventListener('message', (message) => {
        if (message.source !== parent || message.origin !== parentOrigin) return;
        if (message.data?.type === 'denova:prepare-exit' && typeof message.data.requestId === 'string' && message.data.requestId.length <= 128 && prepareExit) {
          Promise.resolve().then(prepareExit).then(ok => {
            parent.postMessage({ type: 'denova:exit-ready', requestId: message.data.requestId, ok: ok !== false }, parentOrigin);
          }).catch(() => parent.postMessage({ type: 'denova:exit-ready', requestId: message.data.requestId, ok: false }, parentOrigin));
          return;
        }
        if (message.data?.type === 'denova:refresh') { window.dispatchEvent(new Event('denova:refresh')); return; }
        if (message.data?.type === 'denova:page-action' && typeof message.data.id === 'string') { window.dispatchEvent(new CustomEvent('denova:page-action', { detail: { id: message.data.id } })); return; }
        if (message.data?.type === 'denova:visibility') {
          context.visible = message.data.visible === true;
          window.dispatchEvent(new Event('denova:visibility'));
          return;
        }
        if (message.data?.type !== 'denova:appearance') return;
        context.locale = message.data.locale === 'zh-CN' ? 'zh-CN' : 'en-US';
        context.theme = message.data.theme === 'light' ? 'light' : 'dark';
        window.dispatchEvent(new Event('denova:appearance'));
      });
      resolve({ context, setContext(value) {
        if (new TextEncoder().encode(JSON.stringify(value)).byteLength > 32768) throw new Error('Page context exceeds 32 KiB');
        parent.postMessage({ type: 'denova:page-context', context: value }, parentOrigin);
      }, setPresentation(presentation) {
        if (new TextEncoder().encode(JSON.stringify(presentation)).byteLength > 8192) throw new Error('Page presentation exceeds 8 KiB');
        parent.postMessage({ type: 'denova:page-presentation', presentation }, parentOrigin);
      }, openTab(tab) {
        if (tab !== 'lore') throw new Error('Unsupported host tab');
        parent.postMessage({ type: 'denova:open-tab', tab }, parentOrigin);
      }, setState({ busy = false, dirty = false } = {}) {
        parent.postMessage({ type: 'denova:state', busy: !!busy, dirty: !!dirty }, parentOrigin);
      }, exit() {
        parent.postMessage({ type: 'denova:exit' }, parentOrigin);
      }, openInstance(instanceId) {
        parent.postMessage({ type: 'denova:open-instance', instanceId }, parentOrigin);
      }, async request(path, options = {}) {
        const { responseType, ...fetchOptions } = options;
        const response = await fetch(connection.baseUrl + path, {
          ...fetchOptions, headers: { 'Content-Type': 'application/json',
            ...options.headers, Authorization: `Bearer ${connection.token}`,
            ...(connection.consumerId ? { 'X-Denova-Consumer': connection.consumerId } : {}) },
        });
        if (!response.ok) {
          const error = await response.json();
          throw Object.assign(new Error(error.diagnostic), error);
        }
        return responseType === 'stream' ? response : response.status === 204 ? undefined : response.json();
      }});
    }
    window.addEventListener('message', receive);
    parent.postMessage({ type: 'denova:ready', nonce, exitGuard: !!prepareExit }, parentOrigin);
  });
}
