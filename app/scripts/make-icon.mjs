// SplitLane: draws build/icon.ico, the icon of SplitLane.exe, of the installer and of the uninstaller (spec 3 of
// stage 2): the lane glyph of the tray (laneGlyphBitmap of src/main/trayModel.ts, in the VPN colour of tray.ts, as
// the window's own icon) on a dark rounded square #0e1014, at 16, 24, 32, 48, 64, 128 and 256 px. Each size is a
// PNG inside the ICO (Windows Vista and later read them); the PNG is written here with node:zlib, no packages.
// Node 24 strips the types of trayModel.ts on import; it warns that package.json does not set the module type.
// Usage (from app): node scripts/make-icon.mjs. The result is kept in git; run it again when the glyph changes.
import { writeFileSync } from 'node:fs'
import { crc32, deflateSync } from 'node:zlib'
import { laneGlyphBitmap } from '../src/main/trayModel.ts'

const SIZES = [16, 24, 32, 48, 64, 128, 256]
/** COLORS.working of tray.ts: the window's icon is the glyph in the VPN colour. */
const VPN = [0x8b, 0x7c, 0xff]
const BACKGROUND = [0x0e, 0x10, 0x14]
/** The corner radius of the square and the margin around the glyph, in parts of the size. */
const RADIUS = 3 / 16
const MARGIN = 1 / 8
/** Samples per pixel along each axis for the rounded corners. */
const SAMPLES = 4

/** The share of the pixel (x, y) that lies inside the rounded square of size × size. */
function squareCoverage(x, y, size) {
  const r = size * RADIUS
  let inside = 0
  for (let sy = 0; sy < SAMPLES; sy++) {
    for (let sx = 0; sx < SAMPLES; sx++) {
      const px = x + (sx + 0.5) / SAMPLES
      const py = y + (sy + 0.5) / SAMPLES
      const dx = Math.max(r - px, px - (size - r), 0)
      const dy = Math.max(r - py, py - (size - r), 0)
      if (dx * dx + dy * dy <= r * r) inside++
    }
  }
  return inside / (SAMPLES * SAMPLES)
}

/** The icon at size × size: RGBA, straight alpha, the glyph over the square. */
function iconPixels(size) {
  const margin = Math.round(size * MARGIN)
  const inner = size - 2 * margin
  const glyph = laneGlyphBitmap(inner, VPN) // BGRA, straight alpha
  const out = Buffer.alloc(size * size * 4)
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const back = squareCoverage(x, y, size)
      let front = 0
      let rgb = [0, 0, 0]
      const gx = x - margin
      const gy = y - margin
      if (gx >= 0 && gy >= 0 && gx < inner && gy < inner) {
        const i = (gy * inner + gx) * 4
        front = glyph[i + 3] / 255
        rgb = [glyph[i + 2], glyph[i + 1], glyph[i]]
      }
      const alpha = front + back * (1 - front)
      const o = (y * size + x) * 4
      if (alpha === 0) continue
      for (let c = 0; c < 3; c++) {
        out[o + c] = Math.round((rgb[c] * front + BACKGROUND[c] * back * (1 - front)) / alpha)
      }
      out[o + 3] = Math.round(alpha * 255)
    }
  }
  return out
}

function chunk(type, data) {
  const head = Buffer.alloc(8)
  head.writeUInt32BE(data.length, 0)
  head.write(type, 4, 'ascii')
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(Buffer.concat([head.subarray(4), data])), 0)
  return Buffer.concat([head, data, crc])
}

/** A PNG of the RGBA pixels: 8 bits per channel, no filter, no interlace. */
function png(size, rgba) {
  const header = Buffer.alloc(13)
  header.writeUInt32BE(size, 0)
  header.writeUInt32BE(size, 4)
  header[8] = 8 // bits per channel
  header[9] = 6 // RGBA
  const rows = Buffer.alloc(size * (size * 4 + 1))
  for (let y = 0; y < size; y++) {
    rows[y * (size * 4 + 1)] = 0 // filter: none
    rgba.copy(rows, y * (size * 4 + 1) + 1, y * size * 4, (y + 1) * size * 4)
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(rows, { level: 9 })),
    chunk('IEND', Buffer.alloc(0))
  ])
}

/** An ICO with one PNG per size. */
function ico(images) {
  const dir = Buffer.alloc(6 + 16 * images.length)
  dir.writeUInt16LE(0, 0)
  dir.writeUInt16LE(1, 2) // icon
  dir.writeUInt16LE(images.length, 4)
  let offset = dir.length
  images.forEach(({ size, data }, n) => {
    const e = 6 + 16 * n
    dir[e] = size >= 256 ? 0 : size // 0 means 256
    dir[e + 1] = size >= 256 ? 0 : size
    dir.writeUInt16LE(1, e + 4) // planes
    dir.writeUInt16LE(32, e + 6) // bits per pixel
    dir.writeUInt32LE(data.length, e + 8)
    dir.writeUInt32LE(offset, e + 12)
    offset += data.length
  })
  return Buffer.concat([dir, ...images.map((i) => i.data)])
}

const images = SIZES.map((size) => ({ size, data: png(size, iconPixels(size)) }))
const file = new URL('../build/icon.ico', import.meta.url)
writeFileSync(file, ico(images))
console.log(`icon.ico: ${SIZES.join(', ')} px`)
