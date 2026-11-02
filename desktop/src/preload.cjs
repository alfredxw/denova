const { contextBridge, ipcRenderer } = require('electron')

// Sandboxed preload exposes only named operations, never IPC or Node APIs.
const backendOrigin = process.argv.find(value => value.startsWith('--denova-backend='))?.split('=')[1]
contextBridge.exposeInMainWorld('denovaDesktop', {
  platform: process.platform,
  backendOrigin,
  chooseDirectory: initialPath => ipcRenderer.invoke('desktop:directory', initialPath),
  changeDataDirectory: () => ipcRenderer.invoke('desktop:change-data-directory'),
  setAppearance: appearance => ipcRenderer.invoke('desktop:appearance', appearance),
  updates: {
    status: () => ipcRenderer.invoke('desktop:update-status'),
    check: () => ipcRenderer.invoke('desktop:update-check'),
    download: () => ipcRenderer.invoke('desktop:update-download'),
    apply: when => ipcRenderer.invoke('desktop:update-apply', when),
  },
})
