const search = {
  'search.placeholder': 'Search current book...',
  'search.failed': 'Search failed',
  'search.resultCount': 'Found {{count}} result(s)',
  'search.noWorkspace': 'Choose a book in Book Management first',
  'search.empty': 'Search prose, lore, and project files',
  'search.noResults': 'No matching content found',
  'search.line': 'Line {{line}}',
  'search.pathMatch': 'Path match',
  'search.column': 'Column {{column}}',
  'search.toggleRegex': 'Regular expression',
  'search.toggleReplace': 'Replace',
  'search.replacePlaceholder': 'Replace with...',
  'search.replaceAll': 'Replace All',
  'search.replaceConfirmTitle': 'Confirm Global Replace',
  'search.replaceConfirmDescription': 'Replace every occurrence of "{{query}}" with "{{replacement}}" in the content of all workspace files. This applies beyond the currently listed results. A restorable version is created before replacing.',
  'search.replaceDone': 'Replaced {{count}} occurrence(s) in {{files}} file(s)',
  'search.replaceNoMatches': 'No matches to replace',
  'search.replaceSkipped': '{{count}} file(s) were skipped because they changed during the replace',
} as const

export default search
