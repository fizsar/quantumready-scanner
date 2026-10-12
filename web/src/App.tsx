import { useEffect, useState } from 'react'

type ApiStatus = { kind: 'checking' } | { kind: 'ok'; version: string } | { kind: 'down' }

function useApiStatus(): ApiStatus {
  const [status, setStatus] = useState<ApiStatus>({ kind: 'checking' })

  useEffect(() => {
    const ctrl = new AbortController()
    fetch('/api/health', { signal: ctrl.signal })
      .then(async (res) => {
        const body = (await res.json()) as { status?: string; version?: string }
        setStatus(res.ok && body.status === 'ok' ? { kind: 'ok', version: body.version ?? '?' } : { kind: 'down' })
      })
      .catch(() => {
        if (!ctrl.signal.aborted) setStatus({ kind: 'down' })
      })
    return () => ctrl.abort()
  }, [])

  return status
}

const checks = [
  ['scan', 'ML-KEM híbrido (X25519MLKEM768)'],
  ['scan', 'versiones TLS y suites de cifrado'],
  ['scan', 'algoritmos de certificado'],
  ['report', 'NIS2 · DORA · hoja de ruta PQC de la UE'],
] as const

export default function App() {
  const api = useApiStatus()

  return (
    <div className="page">
      <header className="topbar">
        <a href="https://fizsar.com">[~/fizsar]</a>
        <span className="muted">/ quantumready</span>
      </header>

      <main className="hero">
        <h1 className="brand">
          <span className="logo" aria-hidden="true">
            Q&#770;
          </span>
          <span className="wordmark">QuantumReady</span>
        </h1>

        <section className="terminal" aria-label="Estado del proyecto">
          <p className="prompt">
            <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./status
          </p>
          <p className="soon">
            &gt; Próximamente<span className="en"> / Coming soon</span>
          </p>
          <p className="lead">
            Auditoría de preparación post-cuántica para tus endpoints TLS.
            <br />
            <span className="muted">Post-quantum readiness audits for your TLS endpoints.</span>
          </p>
          <ul className="checks">
            {checks.map(([tag, label]) => (
              <li key={label}>
                <span className="tag">[{tag}]</span> {label}
              </li>
            ))}
          </ul>
          <p className="prompt">
            <span className="user">guest@quantumready</span>:<span className="path">~</span>$ <span className="cursor" aria-hidden="true" />
          </p>
        </section>
      </main>

      <footer className="footer">
        <span className={`api api-${api.kind}`} role="status">
          api: {api.kind === 'checking' ? 'comprobando…' : api.kind === 'ok' ? `ok · ${api.version}` : 'sin conexión'}
        </span>
        <a href="https://fizsar.com">← fizsar.com</a>
      </footer>
    </div>
  )
}
