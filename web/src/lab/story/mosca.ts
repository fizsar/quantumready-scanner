// Scene 6: "harvest now, decrypt later" and Mosca's inequality, x + y > z.
//
// x: how many years the data must stay secret (its shelf life); y: how long the
// migration to quantum-safe cryptography takes; z: how many years until a
// cryptographically relevant quantum computer (CRQC). If x + y > z, data sent
// today could be decrypted while it still has to be secret. M. Mosca,
// "Cybersecurity in an Era with Quantum Computers: Will We Be Ready?", IEEE
// Security & Privacy 16(5), 38–41 (2018).
//
// z is not known. What we use is an expert survey: Global Risk Institute,
// "Quantum Threat Timeline Report 2025" (M. Mosca, M. Piani, evolutionQ;
// published March 2026): 26 experts estimated the likelihood of a quantum
// computer able to factor a 2048-bit number in less than 24 hours within 5, 10,
// 15, 20 and 30 years. The report averages the answers twice: taking the lower
// end of each likelihood bin ("pessimistic interpretation") and the upper end
// ("optimistic"). Only the values the report states in its text are here; the
// rest (bars in its figures) are not read off charts.

export interface Anchor {
  /** Years from the survey. */
  years: number
  /** Average likelihood of a CRQC within that time, lower end of the bins (null: not stated in the text). */
  low: number | null
  /** The same, upper end of the bins (null: not stated in the text). */
  high: number | null
}

/** The report's stated averages (executive summary and §4.2.1 of the full report). */
export const GRI_2025: readonly Anchor[] = [
  { years: 5, low: null, high: 0.15 },
  { years: 10, low: 0.28, high: 0.49 },
  { years: 15, low: 0.51, high: 0.7 },
  { years: 20, low: 0.69, high: null },
]
export const GRI_EXPERTS = 26

/** Slider ranges (years). */
export const X_MAX = 30
export const Y_MAX = 20

export type Verdict =
  /** x + y reaches a horizon with a full range: P(CRQC before x + y) is at least that range. */
  | { kind: 'range'; total: number; anchor: Anchor & { low: number; high: number }; also?: Anchor & { low: number } }
  /** x + y passes 5 years but not 10: the report gives only an upper-end average at 5 years. */
  | { kind: 'partial'; total: number; anchor: Anchor }
  /** x + y is below every horizon of the report. */
  | { kind: 'below'; total: number }

/**
 * What the survey says about x + y > z for these x and y. The likelihood that a
 * CRQC exists before x + y years is at least its likelihood within any shorter
 * horizon (a cumulative probability never decreases), so the answer is the
 * longest horizon of the report not beyond x + y that has a full range. Never a
 * date: a range of likelihoods, with its source.
 */
export function verdict(x: number, y: number): Verdict {
  const total = x + y
  let best: Verdict = { kind: 'below', total }
  let lowOnly: (Anchor & { low: number }) | undefined
  for (const a of GRI_2025) {
    if (a.years > total) break
    if (a.low !== null && a.high !== null) {
      best = { kind: 'range', total, anchor: { ...a, low: a.low, high: a.high } }
      lowOnly = undefined
    } else if (a.low !== null) lowOnly = { ...a, low: a.low }
    else if (best.kind === 'below') best = { kind: 'partial', total, anchor: a }
  }
  // A longer horizon with only its lower end stated (20 years): a further lower bound.
  if (best.kind === 'range' && lowOnly && lowOnly.low > best.anchor.low) best = { ...best, also: lowOnly }
  return best
}

/** Percent, rounded (the report gives whole percents). */
export const pct = (p: number) => Math.round(p * 100)

/**
 * The verdict in words, for the panel and its live region. Short (the phone
 * sheet is as tall as its tallest step), with the survey named, never a date.
 */
export function sentence(v: Verdict, lang: 'es' | 'en'): string {
  const es = lang === 'es'
  const yrs = (n: number) => (es ? `${n} años` : `${n} years`)
  const five = GRI_2025.find((a) => a.years === 5)!
  const ten = GRI_2025.find((a) => a.years === 10)!
  const head = `x + y = ${yrs(v.total)}: `
  if (v.kind === 'range') {
    const { years, low, high } = v.anchor
    const also = v.also ? (es ? ` A ${v.also.years}, al menos el ${pct(v.also.low)}\u00a0%.` : ` At ${v.also.years}, at least ${pct(v.also.low)}%.`) : ''
    return es
      ? `${head}en ${years} años, los expertos ven un ${pct(low)}–${pct(high)}\u00a0% de probabilidad de un ordenador cuántico capaz (GRI 2025). Riesgo.${also}`
      : `${head}within ${years} years, experts put a capable quantum computer at ${pct(low)}–${pct(high)}% (GRI 2025). At risk.${also}`
  }
  const tail = es
    ? `en 5 años la media no pasa del ${pct(five.high!)}\u00a0%; en 10, ${pct(ten.low!)}–${pct(ten.high!)}\u00a0% (GRI 2025).`
    : `within 5 years the average is at most ${pct(five.high!)}%; within 10, ${pct(ten.low!)}–${pct(ten.high!)}% (GRI 2025).`
  return head + tail
}
