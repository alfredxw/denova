const LANGUAGE_BY_EXTENSION: Readonly<Record<string, string>> = {
  bash: 'shell',
  c: 'c',
  cc: 'cpp',
  cpp: 'cpp',
  cs: 'csharp',
  css: 'css',
  go: 'go',
  gql: 'graphql',
  graphql: 'graphql',
  h: 'c',
  hpp: 'cpp',
  html: 'html',
  java: 'java',
  js: 'javascript',
  json: 'json',
  jsonc: 'json',
  jsx: 'javascript',
  kt: 'kotlin',
  kts: 'kotlin',
  less: 'less',
  lua: 'lua',
  md: 'markdown',
  markdown: 'markdown',
  mdx: 'markdown',
  mjs: 'javascript',
  php: 'php',
  prisma: 'graphql',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  sass: 'scss',
  scss: 'scss',
  sh: 'shell',
  sql: 'sql',
  swift: 'swift',
  toml: 'ini',
  ts: 'typescript',
  tsx: 'typescript',
  vue: 'html',
  xml: 'xml',
  yaml: 'yaml',
  yml: 'yaml',
  zsh: 'shell',
}

const LANGUAGE_BY_NAME: Readonly<Record<string, string>> = {
  dockerfile: 'dockerfile',
  makefile: 'plaintext',
}

/** Resolves Monaco's language ID without coupling the editor to one project type. */
export function projectFileLanguage(path: string): string {
  const name = path.split('/').at(-1)?.toLowerCase() ?? ''
  if (LANGUAGE_BY_NAME[name]) return LANGUAGE_BY_NAME[name]
  const extension = name.includes('.') ? name.split('.').at(-1) ?? '' : ''
  return LANGUAGE_BY_EXTENSION[extension] ?? 'plaintext'
}

/** MDX stays in source mode because rendering executable MDX is not safe here. */
export function isPreviewableMarkdown(path: string): boolean {
  const name = path.split('/').at(-1)?.toLowerCase() ?? ''
  return name.endsWith('.md') || name.endsWith('.markdown')
}
