/** The still image of an orbital, rendered at build time from hydrogen.ts (build/orbitalImages.ts). */
export const stillURL = (id: string) => `/lab/orbital-${id}.png`

/** Loads the 3D scene module (three.js): a separate chunk, fetched only in 3D mode. */
export const loadScene = () => import('./orbitalScene')
