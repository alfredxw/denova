import { execFileSync } from 'node:child_process'
import { appendFileSync, writeFileSync } from 'node:fs'
import { randomBytes } from 'node:crypto'
import path from 'node:path'

// CI-only credential preparation. Certificates remain in the ephemeral runner.
const mac = process.platform === 'darwin'
const value = process.env[mac ? 'MAC_CERTIFICATE' : 'WINDOWS_CERTIFICATE']
const password = process.env[mac ? 'MAC_CERTIFICATE_PASSWORD' : 'MYGO_WINDOWS_CERTIFICATE_PASSWORD']
if (!value && !password) {
  console.log('Platform code signing is disabled; no certificate is configured')
  process.exit(0)
}
if (!value || !password || !process.env.RUNNER_TEMP || !process.env.GITHUB_ENV) throw new Error('Missing native signing credentials or GitHub runner environment')
const certificate = path.join(process.env.RUNNER_TEMP, mac ? 'denova.p12' : 'denova.pfx')
const bytes = value.startsWith('https://') ? await fetch(value).then(r => { if (!r.ok) throw new Error(`Certificate download failed: ${r.status}`); return r.arrayBuffer() }) : Buffer.from(value, 'base64')
writeFileSync(certificate, Buffer.from(bytes), { mode: 0o600 })
const env = (name, value) => appendFileSync(process.env.GITHUB_ENV, `${name}=${value}\n`)
const run = (name, args) => {
  try { return execFileSync(name, args, { stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8' }) }
  catch { throw new Error(`Signing credential preparation failed: ${name} ${args[0]}`) }
}
if (mac) {
  const keychain = path.join(process.env.RUNNER_TEMP, 'denova-signing.keychain-db')
  const secret = randomBytes(32).toString('hex')
  run('security', ['create-keychain', '-p', secret, keychain])
  run('security', ['set-keychain-settings', '-lut', '21600', keychain])
  run('security', ['unlock-keychain', '-p', secret, keychain])
  run('security', ['import', certificate, '-k', keychain, '-P', password, '-T', '/usr/bin/codesign', '-T', '/usr/bin/security'])
  run('security', ['set-key-partition-list', '-S', 'apple-tool:,apple:,codesign:', '-s', '-k', secret, keychain])
  run('security', ['list-keychains', '-d', 'user', '-s', keychain])
  run('security', ['default-keychain', '-d', 'user', '-s', keychain])
  const identity = run('security', ['find-identity', '-v', '-p', 'codesigning', keychain]).match(/([A-F0-9]{40}) "Developer ID Application:/)?.[1]
  if (!identity) throw new Error('No Developer ID Application identity found')
  run('xcrun', ['notarytool', 'store-credentials', 'denova-release', '--apple-id', process.env.APPLE_ID, '--password', process.env.APPLE_APP_SPECIFIC_PASSWORD, '--team-id', process.env.APPLE_TEAM_ID])
  env('DENOVA_MAC_SIGNING_IDENTITY', identity)
  env('DENOVA_MAC_NOTARY_PROFILE', 'denova-release')
} else env('DENOVA_WINDOWS_CERTIFICATE', certificate)
