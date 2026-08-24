export function fileTreeMenuPlatformPresentation() {
  const platform = typeof navigator === 'undefined' ? '' : navigator.platform
  if (/Mac|iPhone|iPad/.test(platform)) {
    return { copy: '⌘C', copyPath: '⌘⌥C', copyRelativePath: '⌘⌥⇧C', delete: '⌘⌫', revealKey: 'sidebar.revealInFinder' as const }
  }
  if (/Win/.test(platform)) {
    return { copy: 'Ctrl+C', copyPath: 'Ctrl+Alt+C', copyRelativePath: 'Ctrl+Alt+Shift+C', delete: 'Delete', revealKey: 'sidebar.revealInFileExplorer' as const }
  }
  return { copy: 'Ctrl+C', copyPath: 'Ctrl+Alt+C', copyRelativePath: 'Ctrl+Alt+Shift+C', delete: 'Delete', revealKey: 'sidebar.revealInFileManager' as const }
}
