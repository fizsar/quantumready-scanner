import { Fragment, type ReactNode } from 'react'

// The story's running text is set in Bricolage Grotesque, which has no Greek and
// none of a few symbols. Left alone, the browser would paint those with whatever
// font has them (a change of style mid-line, a different one on every system).
// typeset() makes each change of face explicit and always the same:
// - a Greek letter, with the sub- and superscripts that follow it (ψ₁, σ²): in
//   the mono (.mono), which has them;
// - ℏ and 𝔽 (identifiers) and ∓ ∝ ⊗ (operators), which no self-hosted face has:
//   inside <math>, set by the math font;
// - ˣ (a superscript x, in no face): a raised x of the text's own face.
// fontCoverage.test.tsx fails on any rendered character outside its context's font.
const TOKEN = /([Ͱ-Ͽἀ-῿][Ͱ-Ͽ⁰-₟]*)|([ℏ𝔽])|([∓∝⊗])|(ˣ)/gu

export function typeset(text: string): ReactNode {
  const out: ReactNode[] = []
  let last = 0
  for (const m of text.matchAll(TOKEN)) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const [all, greek, ident, op] = m
    const k = m.index
    if (greek) out.push(<span key={k} className="mono">{greek}</span>)
    else if (ident) out.push(<math key={k}><mi>{ident}</mi></math>)
    else if (op) out.push(<math key={k}><mo>{op}</mo></math>)
    else out.push(<sup key={k}>x</sup>)
    last = k + all.length
  }
  if (last === 0) return text
  if (last < text.length) out.push(text.slice(last))
  return <Fragment>{out}</Fragment>
}
