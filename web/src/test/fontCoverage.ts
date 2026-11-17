/// <reference types="node" />
// Which code points a self-hosted font really covers, read from its woff2 files (the
// ones the browser downloads), for the font coverage test. Node's zlib has Brotli;
// the cmap table is never transformed in WOFF2, so it can be read as it is.
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { brotliDecompressSync } from 'node:zlib'

const KNOWN_TAGS = 'cmap head hhea hmtx maxp name OS/2 post cvt  fpgm glyf loca prep CFF  VORG EBDT EBLC gasp hdmx kern LTSH PCLT VDMX vhea vmtx BASE GDEF GPOS GSUB EBSC JSTF MATH CBDT CBLC COLR CPAL SVG  sbix acnt avar bdat bloc bsln cvar fdsc feat fmtx fvar gvar hsty just lcar mort morx opbd prop trak Zapf Silf Glat Gloc Feat Sill'
  .match(/.{4}\s?/g)!
  .map((t) => t.slice(0, 4).trimEnd())

/** The code points a woff2 file maps to a glyph (glyph id ≠ 0). */
export function woff2Cmap(path: string): Set<number> {
  const buf = readFileSync(path)
  if (buf.toString('latin1', 0, 4) !== 'wOF2') throw new Error(`${path}: not a woff2 file`)
  const numTables = buf.readUInt16BE(12)
  let p = 48
  const base128 = () => {
    let v = 0
    for (let i = 0; i < 5; i++) {
      const b = buf[p++]
      v = v * 128 + (b & 0x7f)
      if (!(b & 0x80)) return v
    }
    throw new Error('bad UIntBase128')
  }
  const tables: { tag: string; length: number }[] = []
  for (let i = 0; i < numTables; i++) {
    const flags = buf[p++]
    const tag = (flags & 0x3f) === 63 ? buf.toString('latin1', p, (p += 4)) : KNOWN_TAGS[flags & 0x3f]
    const version = flags >> 6
    const origLength = base128()
    // glyf and loca are transformed with version 0; every other table with a version ≠ 0.
    const transformed = tag === 'glyf' || tag === 'loca' ? version === 0 : version !== 0
    tables.push({ tag, length: transformed ? base128() : origLength })
  }
  const data = brotliDecompressSync(buf.subarray(p, p + buf.readUInt32BE(20)))
  let off = 0
  for (const t of tables) {
    if (t.tag === 'cmap') return parseCmap(data.subarray(off, off + t.length))
    off += t.length
  }
  throw new Error(`${path}: no cmap`)
}

function parseCmap(c: Buffer): Set<number> {
  const out = new Set<number>()
  const n = c.readUInt16BE(2)
  const subtables: { platform: number; encoding: number; offset: number }[] = []
  for (let i = 0; i < n; i++) subtables.push({ platform: c.readUInt16BE(4 + i * 8), encoding: c.readUInt16BE(6 + i * 8), offset: c.readUInt32BE(8 + i * 8) })
  const pick = subtables.find((s) => s.platform === 3 && s.encoding === 10) ?? subtables.find((s) => (s.platform === 3 && s.encoding === 1) || s.platform === 0)
  if (!pick) throw new Error('no Unicode cmap')
  const t = c.subarray(pick.offset)
  const format = t.readUInt16BE(0)
  if (format === 12) {
    const groups = t.readUInt32BE(12)
    for (let g = 0; g < groups; g++) {
      const start = t.readUInt32BE(16 + g * 12)
      const end = t.readUInt32BE(20 + g * 12)
      const glyph = t.readUInt32BE(24 + g * 12)
      for (let cp = start; cp <= end; cp++) if (glyph + (cp - start) !== 0) out.add(cp)
    }
  } else if (format === 4) {
    const segs = t.readUInt16BE(6) / 2
    const ends = 14
    const starts = ends + segs * 2 + 2
    const deltas = starts + segs * 2
    const ranges = deltas + segs * 2
    for (let s = 0; s < segs; s++) {
      const end = t.readUInt16BE(ends + s * 2)
      const start = t.readUInt16BE(starts + s * 2)
      const delta = t.readInt16BE(deltas + s * 2)
      const range = t.readUInt16BE(ranges + s * 2)
      for (let cp = start; cp <= end && cp !== 0xffff; cp++) {
        let glyph
        if (range === 0) glyph = (cp + delta) & 0xffff
        else {
          const g = t.readUInt16BE(ranges + s * 2 + range + (cp - start) * 2)
          glyph = g === 0 ? 0 : (g + delta) & 0xffff
        }
        if (glyph !== 0) out.add(cp)
      }
    }
  } else throw new Error(`cmap format ${format} not supported`)
  return out
}

/** unicode-range → a predicate (absent: every code point). */
function inRange(range: string | undefined) {
  if (!range) return () => true
  const parts = range.split(',').map((r) => {
    const m = r.trim().match(/^U\+([0-9A-F?]+)(?:-([0-9A-F]+))?$/i)
    if (!m) throw new Error(`bad unicode-range ${r}`)
    if (m[1].includes('?')) return [parseInt(m[1].replace(/\?/g, '0'), 16), parseInt(m[1].replace(/\?/g, 'F'), 16)]
    return [parseInt(m[1], 16), parseInt(m[2] ?? m[1], 16)]
  })
  return (cp: number) => parts.some(([a, b]) => cp >= a && cp <= b)
}

/**
 * What a family covers, from the @font-face rules of a stylesheet: the union, over
 * its faces of one weight, of each woff2's cmap limited to the face's unicode-range
 * (the browser only uses a face inside its range).
 */
export function familyCoverage(cssPath: string, family: string, weight: number): (cp: number) => boolean {
  const css = readFileSync(cssPath, 'utf8')
  const faces = [...css.matchAll(/@font-face\s*\{([^}]*)\}/g)]
    .map((m) => m[1])
    .filter((f) => f.includes(`font-family: '${family}'`))
    .filter((f) => {
      const w = f.match(/font-weight:\s*(\d+)(?:\s+(\d+))?;/)
      return !w || (Number(w[1]) <= weight && weight <= Number(w[2] ?? w[1]))
    })
  if (!faces.length) throw new Error(`no @font-face for ${family} ${weight} in ${cssPath}`)
  const sets = faces.map((f) => {
    const url = f.match(/url\(['"]?([^'")]+\.woff2)['"]?\)/)?.[1]
    if (!url) throw new Error(`a ${family} face without woff2`)
    const cmap = woff2Cmap(resolve(dirname(cssPath), url))
    const range = inRange(f.match(/unicode-range:\s*([^;]+);/)?.[1])
    return (cp: number) => cmap.has(cp) && range(cp)
  })
  return (cp) => sets.some((s) => s(cp))
}
