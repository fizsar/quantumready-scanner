/** The still image of a story step, rendered at build time (build/storyImages.ts). */
export const storyStillURL = (step: string) => `/lab/story-${step}.png`

/** Loads the story engine (three.js): a separate chunk, fetched only in 3D mode. */
export const loadStory = () => import('./engine')
