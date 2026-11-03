// MathML Core elements for JSX (React renders them in the MathML namespace
// inside <math>; @types/react does not declare them). Only what the lab uses.
import 'react'

type MathProps = React.DetailedHTMLProps<React.HTMLAttributes<MathMLElement>, MathMLElement> & {
  display?: 'block' | 'inline'
  mathvariant?: 'normal'
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      math: MathProps
      mrow: MathProps
      mi: MathProps
      mn: MathProps
      mo: MathProps
      mfrac: MathProps
      msub: MathProps
      msup: MathProps
      msqrt: MathProps
      msubsup: MathProps
      mtext: MathProps
      mspace: MathProps & { width?: string }
    }
  }
}
