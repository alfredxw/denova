import { execFileSync } from 'node:child_process'
import { copyFileSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const desktop = path.join(root, 'desktop')
const target = process.env.DENOVA_DESKTOP_TARGET || `${process.platform}-${process.arch}`
const platforms = { 'darwin-arm64': 'darwin/arm64', 'win32-x64': 'windows/amd64' }
if (!platforms[target]) throw new Error(`Unsupported desktop target: ${target}`)
const version = JSON.parse(readFileSync(path.join(desktop, 'package.json'))).version
const publicKey = process.env.DENOVA_DESKTOP_UPDATE_PUBLIC_KEY || ''
const release = process.env.DENOVA_DESKTOP_RELEASE === 'true'
if (release && (!publicKey || !process.env.MYGO_UPDATER_PRIVATE_KEY)) throw new Error('Release builds require both desktop update signing keys')
const config = {
  // Older Web clients select denova-* archives by platform. Keep a distinct prefix.
  name: 'Denova', slug: 'denovadesktop', identifier: 'app.denova.desktop', version,
  copyright: 'Copyright © Denova contributors', main: './cmd/denova-desktop',
  i18n: { dir: 'internal/desktop/locales', source: 'en-US' },
  icon: 'desktop/assets/icon.png', out: 'desktop/dist', resources: ['desktop/build/backend', 'desktop/assets/tray.png', 'desktop/assets/icon.png'],
  macos: { minimumSystemVersion: '12.0', category: 'public.app-category.productivity',
    ...(process.env.DENOVA_MAC_SIGNING_IDENTITY ? { signingIdentity: process.env.DENOVA_MAC_SIGNING_IDENTITY } : {}),
    ...(process.env.DENOVA_MAC_NOTARY_PROFILE ? { notarize: { keychainProfile: process.env.DENOVA_MAC_NOTARY_PROFILE } } : {}),
  },
  windows: { ...(process.env.DENOVA_WINDOWS_CERTIFICATE ? { certificate: process.env.DENOVA_WINDOWS_CERTIFICATE } : {}) },
  ...(publicKey ? { updates: { publicKey, github: 'alfredxw/denova', tagPrefix: 'v', deltas: 0, changelog: 'CHANGELOG.md' } } : {}),
}
mkdirSync(path.join(desktop, 'build'), { recursive: true })
writeFileSync(path.join(root, 'mygo.json'), JSON.stringify(config, null, 2) + '\n')
const mygoVersion = execFileSync('go', ['list', '-m', '-f', '{{.Version}}', 'github.com/egoist/mygo'], { cwd: root, encoding: 'utf8' }).trim()
const args = ['run', `github.com/egoist/mygo/cmd/mygo@${mygoVersion}`, 'build', '-platform', platforms[target]]
if (process.env.DENOVA_DESKTOP_E2E === 'true') args.push('-debug')
if (process.env.DENOVA_DESKTOP_SKIP_INSTALLER === 'true') args.push('-skip-installer', '-skip-dmg')
args.push(root)
execFileSync('go', args, { cwd: root, stdio: 'inherit', env: { ...process.env,
  GOFLAGS: [process.env.GOFLAGS, process.env.DENOVA_DESKTOP_E2E === 'true' ? '-tags=desktop_e2e' : ''].filter(Boolean).join(' '),
} })
if (process.platform === 'darwin' && target === 'darwin-arm64') {
  const out = path.join(desktop, 'dist/darwin-arm64')
  execFileSync('ditto', ['-c', '-k', '--sequesterRsrc', '--keepParent', path.join(out, 'Denova.app'), path.join(out, `denovadesktop-${version}-mac-arm64.zip`)], { stdio: 'inherit' })
}

if (release) {
  // Keep release artifacts flat when actions/upload-artifact computes its root.
  const directory = path.join(desktop, 'dist', platforms[target].replace('/', '-'))
  const stage = path.join(root, 'output/release-desktop')
  mkdirSync(stage, { recursive: true })
  for (const file of readdirSync(directory)) {
    if (/^(denovadesktop-|update-).*\.(dmg|zip|exe|tar\.gz|json|patch)$/.test(file)) copyFileSync(path.join(directory, file), path.join(stage, file))
  }
}
