// Anchors of /examples, apart from examples.ts so that linking to them does
// not pull the example reports into the main bundle.
export type ExampleKey = 'fizsar' | 'modern' | 'kyber' | 'export'

/** Anchor that opens an example's "Under the hood" panel. */
export const uthAnchor = (key: ExampleKey) => `${key}-under-the-hood`
