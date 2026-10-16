import { useId, type ReactNode } from 'react'
import type { ApiError } from '../api'
import { authErrorMessage } from '../auth'
import { useLang } from '../i18n'

/** A labelled input; help and error texts are tied to it for screen readers. */
export function Field(props: {
  label: string
  type?: string
  value: string
  onChange: (v: string) => void
  autoComplete?: string
  help?: string
  required?: boolean
  readOnly?: boolean
  maxLength?: number
  invalid?: boolean
  errorId?: string
}) {
  const id = useId()
  const helpId = useId()
  const described = [props.help ? helpId : '', props.invalid && props.errorId ? props.errorId : ''].filter(Boolean).join(' ')
  return (
    <div className="field">
      <label htmlFor={id} className="label">
        {props.label}
      </label>
      <input
        id={id}
        className="input"
        type={props.type ?? 'text'}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        autoComplete={props.autoComplete}
        required={props.required}
        readOnly={props.readOnly}
        maxLength={props.maxLength}
        spellCheck={false}
        aria-invalid={props.invalid || undefined}
        aria-describedby={described || undefined}
      />
      {props.help && (
        <p id={helpId} className="muted small">
          {props.help}
        </p>
      )}
    </div>
  )
}

/** A titled terminal panel. */
export function Panel({ title, children, wide }: { title: string; children: ReactNode; wide?: boolean }) {
  const id = useId()
  return (
    <section className={wide ? 'terminal' : 'terminal auth'} aria-labelledby={id}>
      <h1 id={id} className="h-title">
        {title}
      </h1>
      {children}
    </section>
  )
}

/** The error of a form, as an alert (empty when there is none). */
export function ErrorLine({ id, error }: { id: string; error: ApiError | null }) {
  const { t } = useLang()
  return (
    <p id={id} className="error" role="alert">
      {error ? authErrorMessage(error, t) : ''}
    </p>
  )
}
