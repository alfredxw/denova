// Regenerates PWA / app icons from public/favicon.svg into public/.
// Run with: pnpm generate-icons
//
// Nova ships a single 1024x1024 favicon.svg. Browsers and iOS need raster
// PNGs for the installable manifest and the Apple touch icon, so we render
// the SVG onto an opaque Nova background (#1a1a1a = --nova-bg) for the
// standard icons, and add an 80% safe-zone for the maskable variant.
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import sharp from 'sharp'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const publicDir = path.resolve(__dirname, '..', 'public')
const svgPath = path.join(publicDir, 'favicon.svg')

// --nova-bg from src/index.css (dark theme). Kept opaque so home-screen
// tiles never show a transparent fringe.
const BACKGROUND = { r: 26, g: 26, b: 26 }

async function renderIcon(size, file) {
  const layer = await sharp(svgPath).resize(size, size).png().toBuffer()
  await sharp({ create: { width: size, height: size, channels: 4, background: BACKGROUND } })
    .composite([{ input: layer, blend: 'over' }])
    .png()
    .toFile(path.join(publicDir, file))
  console.log(`  ✓ ${file} (${size}x${size})`)
}

async function renderMaskable(size, file) {
  // Maskable icons reserve a center 80% "safe zone"; pad the artwork so
  // platform-chrome cropping never clips the book motif.
  const inner = Math.round(size * 0.8)
  const offset = Math.round((size - inner) / 2)
  const layer = await sharp(svgPath).resize(inner, inner).png().toBuffer()
  await sharp({ create: { width: size, height: size, channels: 4, background: BACKGROUND } })
    .composite([{ input: layer, blend: 'over', top: offset, left: offset }])
    .png()
    .toFile(path.join(publicDir, file))
  console.log(`  ✓ ${file} (${size}x${size}, maskable safe-zone)`)
}

async function main() {
  try {
    await readFile(svgPath)
  } catch {
    console.error(`favicon.svg not found at ${svgPath}`)
    process.exit(1)
  }
  console.log('Generating PWA icons from favicon.svg…')
  await renderIcon(180, 'apple-touch-icon.png')
  await renderIcon(192, 'pwa-192.png')
  await renderIcon(512, 'pwa-512.png')
  await renderMaskable(512, 'pwa-maskable-512.png')
  console.log('Done.')
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
