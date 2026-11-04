import { execFileSync } from 'node:child_process'
import { cpSync, mkdirSync, readFileSync, rmSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const desktop = path.join(root, 'desktop')
const target = process.env.DENOVA_DESKTOP_TARGET || `${process.platform}-${process.arch}`
const platforms = { 'darwin-arm64': ['darwin', 'arm64', 'darwin-arm64'], 'win32-x64': ['windows', 'amd64', 'windows-x64'] }
if (!platforms[target]) throw new Error(`Unsupported desktop target: ${target}`)
const [goos, goarch, ripgrep] = platforms[target]
const webPackage = JSON.parse(readFileSync(path.join(root, 'web/package.json')))
const desktopPackage = JSON.parse(readFileSync(path.join(desktop, 'package.json')))
const version = (process.env.DENOVA_VERSION || webPackage.version).replace(/^v/, '')
if (version !== webPackage.version || version !== desktopPackage.version) throw new Error('Desktop, frontend and release versions must match')
const run = (command, args, env = process.env) => execFileSync(command, args, { cwd: root, stdio: 'inherit', env })
let frontend = process.env.DENOVA_RELEASE_FRONTEND_DIR
if (!frontend) {
  // npm_execpath is the actual pnpm JS entry on Windows as well as POSIX.
  run(process.execPath, [process.env.npm_execpath, '--dir', 'web', 'build'])
  frontend = path.join(root, 'web/dist')
}
if (!readFileSync(path.join(frontend, 'index.html')).length) throw new Error('Missing frontend build')
rmSync(path.join(root, 'internal/webfs/dist'), { recursive: true, force: true })
cpSync(frontend, path.join(root, 'internal/webfs/dist'), { recursive: true })
const output = path.join(desktop, 'build/backend')
rmSync(output, { recursive: true, force: true })
mkdirSync(output, { recursive: true })
run('go', ['build', '-tags', 'embedweb', '-trimpath', '-ldflags', `-s -w -X denova/internal/buildinfo.Version=${version} -X denova/internal/buildinfo.Distribution=desktop`, '-o', path.join(output, goos === 'windows' ? 'denova.exe' : 'denova'), './cmd/denova'], { ...process.env, CGO_ENABLED: '0', GOOS: goos, GOARCH: goarch })
run('go', ['run', './scripts/ripgrep-assets', '-target', ripgrep, '-destination', output])
for (const file of ['skills', 'config.toml', 'CHANGELOG.md', 'LICENSE']) cpSync(path.join(root, file), path.join(output, file), { recursive: true })
// Reuse the product artwork and frontend's image dependency.
const requireWeb = createRequire(path.join(root, 'web/package.json'))
const sharp = requireWeb('sharp')
mkdirSync(path.join(desktop, 'assets'), { recursive: true })
await sharp(path.join(root, 'web/public/favicon.svg')).resize(1024, 1024).png().toFile(path.join(desktop, 'assets/icon.png'))
await sharp(path.join(root, 'web/public/favicon.svg')).resize(32, 32).png().toFile(path.join(desktop, 'assets/tray.png'))
cpSync(path.join(desktop, "assets/tray.png"), path.join(desktop, "build/tray.png"))
console.log(`Prepared Denova Desktop ${version} (${target})`)
