export function fileTreeHost(label?: string): HTMLElement {
  const hosts = [...document.querySelectorAll<HTMLElement>('file-tree-container')]
  const host = label ? hosts.find((candidate) => candidate.getAttribute('aria-label') === label) : hosts[0]
  if (!host) throw new Error(`File tree host${label ? ` "${label}"` : ''} is not rendered`)
  return host
}

export function fileTreeShadow(label?: string): ShadowRoot {
  const shadow = fileTreeHost(label).shadowRoot
  if (!shadow) throw new Error('File tree shadow root is not rendered')
  return shadow
}

export function fileTreeRow(path: string, label?: string): HTMLElement {
  const row = [...fileTreeShadow(label).querySelectorAll<HTMLElement>('[data-item-path]')]
    .find((candidate) => candidate.dataset.itemPath === path
      && candidate.dataset.fileTreeStickyRow !== 'true'
      && candidate.dataset.itemParked !== 'true')
  if (!row) throw new Error(`File tree row "${path}" is not rendered`)
  return row
}
