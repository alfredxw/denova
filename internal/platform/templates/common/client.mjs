// Optional convenience wrapper. All platform operations are ordinary HTTP.
export function connect() {
  const parentOrigin = new URL(document.referrer).origin;
  const nonce = crypto.randomUUID();
  return new Promise((resolve) => {
    function receive(event) {
      if (event.source !== parent || event.origin !== parentOrigin ||
          event.data?.type !== 'denova:bootstrap' || event.data.nonce !== nonce) return;
      window.removeEventListener('message', receive);
      const { connection, context } = event.data;
      window.addEventListener('message', (message) => {
        if (message.source !== parent || message.origin !== parentOrigin || message.data?.type !== 'denova:appearance') return;
        context.locale = message.data.locale === 'zh-CN' ? 'zh-CN' : 'en-US';
        context.theme = message.data.theme === 'light' ? 'light' : 'dark';
        window.dispatchEvent(new Event('denova:appearance'));
      });
      resolve({ context, async request(path, options = {}) {
        const { responseType, ...fetchOptions } = options;
        const response = await fetch(connection.baseUrl + path, {
          ...fetchOptions, headers: { 'Content-Type': 'application/json',
            ...options.headers, Authorization: `Bearer ${connection.token}` },
        });
        if (!response.ok) {
          const error = await response.json();
          throw Object.assign(new Error(error.diagnostic), error);
        }
        return responseType === 'stream' ? response : response.status === 204 ? undefined : response.json();
      }});
    }
    window.addEventListener('message', receive);
    parent.postMessage({ type: 'denova:ready', nonce }, parentOrigin);
  });
}
