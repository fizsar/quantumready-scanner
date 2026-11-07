// Scene 6: two sliders for x and y and the verdict on x + y > z, announced in a
// live region. One instance is active at a time (the panel on wide screens, the
// sheet on phones); the other is inert and hidden from assistive technology, so
// the controls are never duplicated or inside an aria-hidden container.
import { useId } from 'react'
import type { Lang } from '../../api'
import { sentence, verdict, X_MAX, Y_MAX } from './mosca'

export interface MoscaValue {
  x: number
  y: number
}

export function MoscaWidget({ value, onChange, lang, active }: { value: MoscaValue; onChange: (v: MoscaValue) => void; lang: Lang; active: boolean }) {
  const id = useId()
  const es = lang === 'es'
  const yrs = (n: number) => (es ? `${n} años` : `${n} years`)
  const row = (key: 'x' | 'y', label: string, max: number) => (
    <div className="story-mosca-row">
      <label htmlFor={`${id}-${key}`}>
        <span className="story-mosca-var">{key}</span> {label}
      </label>
      <input
        id={`${id}-${key}`}
        type="range"
        min={0}
        max={max}
        step={1}
        value={value[key]}
        aria-valuetext={yrs(value[key])}
        onChange={(e) => onChange({ ...value, [key]: Number(e.target.value) })}
      />
      <output htmlFor={`${id}-${key}`}>{yrs(value[key])}</output>
    </div>
  )
  return (
    <div className="story-mosca" inert={!active} aria-hidden={active ? undefined : true}>
      {row('x', es ? 'secreto' : 'secret', X_MAX)}
      {row('y', es ? 'migración' : 'migration', Y_MAX)}
      <p className="story-mosca-out" aria-live={active ? 'polite' : undefined}>
        {sentence(verdict(value.x, value.y), lang)}
      </p>
    </div>
  )
}
