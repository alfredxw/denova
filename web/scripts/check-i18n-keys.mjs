import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const root = path.resolve('src/i18n/locales')
const locales = ['zh-CN', 'en-US']

function readLocale(locale) {
  const dir = path.join(root, locale)
  const files = fs.readdirSync(dir).filter((file) => file.endsWith('.ts')).sort()
  const keys = new Map()
  const errors = []

  for (const file of files) {
    const namespace = file.replace(/\.ts$/, '')
    const text = fs.readFileSync(path.join(dir, file), 'utf8')
    const matches = [...text.matchAll(/^  '([^']+)': /gm)]
    if (matches.length === 0) {
      errors.push(`${locale}/${file}: namespace has no translation keys`)
      continue
    }
    for (const match of matches) {
      const key = match[1]
      if (!key.startsWith(`${namespace}.`)) {
        errors.push(`${locale}/${file}: key ${key} does not match namespace ${namespace}`)
      }
      if (keys.has(key)) {
        errors.push(`${locale}/${file}: duplicate key ${key}; first seen in ${keys.get(key)}`)
      }
      keys.set(key, file)
    }
  }

  return { keys: new Set(keys.keys()), errors }
}

const loaded = new Map(locales.map((locale) => [locale, readLocale(locale)]))
const errors = locales.flatMap((locale) => loaded.get(locale).errors)
const [baseLocale, ...otherLocales] = locales
const baseKeys = loaded.get(baseLocale).keys

for (const locale of otherLocales) {
  const keys = loaded.get(locale).keys
  for (const key of baseKeys) {
    if (!keys.has(key)) errors.push(`${locale}: missing key ${key}`)
  }
  for (const key of keys) {
    if (!baseKeys.has(key)) errors.push(`${locale}: extra key ${key}`)
  }
}

if (errors.length > 0) {
  console.error(errors.join('\n'))
  process.exit(1)
}

console.log(`i18n keys aligned: ${baseKeys.size} keys across ${locales.join(', ')}`)
