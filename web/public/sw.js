// Cleanup shim for historical Denova builds that registered /sw.js.
// Current builds do not register a service worker; keeping this file lets
// browsers update the old registration and remove it without a Hertz 404 log.
self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.registration.unregister())
})
