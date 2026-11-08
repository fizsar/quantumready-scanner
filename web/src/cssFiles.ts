/// <reference types="node" />
// The app's stylesheets, read from disk, for the tests of the design tokens and
// fonts (vitest's CSS handling empties `?raw` imports of .css files). Tests run in web/.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, resolve } from 'node:path'

export const read = (path: string) => readFileSync(resolve(path), 'utf8')

export const cssFiles: string[] = (function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : p.endsWith('.css') ? [p] : []
  })
})(resolve('src'))
