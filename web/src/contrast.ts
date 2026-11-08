// WCAG 2.2 contrast ratio, for the tests of the colour tokens (not used by the app).
export type RGB = [number, number, number]
export type RGBA = [number, number, number, number]

export const hex = (h: string): RGB => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16)) as RGB

export const luminance = ([r, g, b]: RGB) => {
  const c = (v: number) => {
    const s = v / 255
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)
}

export const contrast = (a: RGB, b: RGB) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

// A translucent colour composited over an opaque one.
export const over = (rgba: RGBA, back: RGB): RGB =>
  [0, 1, 2].map((i) => rgba[i] * rgba[3] + back[i] * (1 - rgba[3])) as RGB

// A `--name: #rrggbb` or `--name: rgba(r, g, b, a)` token from a stylesheet.
export const hexToken = (css: string, name: string): RGB => {
  const m = css.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})\\b`))
  if (!m) throw new Error(`no hex --${name}`)
  return hex(m[1])
}
export const rgbaToken = (css: string, name: string): RGBA => {
  const m = css.match(new RegExp(`--${name}:\\s*rgba\\((\\d+),\\s*(\\d+),\\s*(\\d+),\\s*([\\d.]+)\\)`))
  if (!m) throw new Error(`no rgba --${name}`)
  return m.slice(1).map(Number) as RGBA
}
