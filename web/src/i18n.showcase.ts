// UI strings of the showcase additions, kept apart from i18n.ts so the
// showcase branch merges into main without touching its dictionary.
import type { Lang } from './api'
import { useLang } from './i18n'

const es = {
  'uth.title': 'Bajo el capó',
  'uth.lead': 'Los datos técnicos del escaneo tal y como los devuelve la API: qué probó el escáner y qué respondió el servidor.',
  'uth.download': 'Descargar JSON',
  'uth.downloadHelp': 'La respuesta de la API para este escaneo, sin cambios.',
  'uth.scan.title': 'Escaneo',
  'uth.scan.duration': 'Duración',
  'uth.scan.ip': 'IP escaneada',
  'uth.scan.resolved': 'IPs resueltas',
  'uth.scan.connections': 'Conexiones',
  'uth.scan.intensity': 'Intensidad',
  'uth.scan.rawProbes': 'Sondas en crudo',
  'uth.scan.budget': '{n} (se agotó el presupuesto)',
  'uth.intensity.basic': 'básica (solo crypto/tls)',
  'uth.intensity.full': 'completa (crypto/tls y ClientHello en crudo)',
  'uth.seconds': '{s} s',
  'uth.ms': '{ms} ms',
  'uth.none': 'ninguna',
  'uth.yes': 'sí',
  'uth.no': 'no',
  'uth.source.cryptotls': 'crypto/tls',
  'uth.source.raw': 'en crudo',
  'uth.versions.title': 'Versiones de TLS',
  'uth.versions.tried': 'Probadas: {list}',
  'uth.versions.accepted': 'Aceptadas: {list}',
  'uth.groups.title': 'Grupos de intercambio de claves',
  'uth.groups.offered': 'Oferta tipo navegador: {list}',
  'uth.groups.selected': 'Elegido por el servidor: {group}',
  'uth.groups.selectedNone': 'Elegido por el servidor: ninguno',
  'uth.groups.raw13': 'Grupos de TLS 1.3 encontrados en crudo: {list}',
  'uth.groups.raw12': 'Curvas de TLS 1.2 encontradas en crudo: {list}',
  'uth.col.group': 'Grupo',
  'uth.col.id': 'ID',
  'uth.col.hybrid': 'Híbrido',
  'uth.col.reply': 'Respuesta del servidor',
  'uth.col.suite': 'Suite',
  'uth.col.fs': 'FS',
  'uth.col.aead': 'AEAD',
  'uth.col.notes': 'Notas',
  'uth.col.source': 'Sonda',
  'uth.col.offered': 'Se ofreció',
  'uth.suites.title': 'Suites de cifrado aceptadas',
  'uth.suites.lead': 'Lo que el servidor aceptó; las suites que rechazó no se listan.',
  'uth.suites.incomplete': 'Lista incompleta: no hubo sondas para todas.',
  'uth.suites.none': 'Ninguna suite aceptada.',
  'uth.note.export': 'EXPORT',
  'uth.note.anonymous': 'anónima',
  'uth.note.null': 'sin cifrado',
  'uth.note.bits': '{bits} bits',
  'uth.dh': 'DH en {suite} ({version}): primo de {bits} bits',
  'uth.probes.title': 'Sondas, una a una',
  'uth.probes.lead': 'Cada fila es una conexión: lo que ofreció el escáner y lo primero que respondió el servidor.',
  'uth.probes.rawNote': 'Las demás sondas en crudo (hasta {n} en total) enumeran suites y grupos; la API guarda lo que encontraron, en las tablas de arriba.',
  'uth.offer.default': 'Oferta tipo navegador · grupos: {groups}',
  'uth.offer.version': 'Solo {version}',
  'uth.offer.group': 'Solo el grupo {group} ({id})',
  'uth.reply.hello': 'ServerHello',
  'uth.reply.ciphers': '{n} cifrados',
  'uth.reply.alert': 'Alerta {code} {name}',
  'uth.reply.alertNoCode': 'Alerta {name}',
  'uth.reply.ianaCode': 'código según IANA',
  'uth.reply.closed': 'Cierre de la conexión',
  'uth.reply.timeout': 'Tiempo agotado',
  'uth.reply.refused': 'Rechazada, sin más detalle',

  'footer.architecture': 'Arquitectura',
  'arch.meta.title': 'Arquitectura · QuantumReady',
  'arch.meta.description':
    'Cómo funciona QuantumReady por dentro: el flujo de un escaneo, el motor TLS con ClientHello en crudo, el modelo de puntuación, la seguridad del servicio, las pruebas y la operación.',
  'arch.title': 'Arquitectura',
  'arch.lead': 'Cómo funciona QuantumReady por dentro, a alto nivel.',
  'arch.flow.title': 'El flujo de un escaneo',
  'arch.flow.svgTitle': 'Flujo de un escaneo en QuantumReady',
  'arch.flow.svgDesc':
    'El navegador se conecta a Caddy, que reparte entre la web (nginx) y la API (Go). La API guarda el escaneo en PostgreSQL, que hace de cola. Un worker lo reclama, ejecuta el escáner y guarda el resultado. El escáner solo se conecta al puerto 443 del servidor objetivo.',
  'arch.flow.browser': 'Navegador',
  'arch.flow.caddy': 'Caddy',
  'arch.flow.caddySub': 'TLS y reparto',
  'arch.flow.web': 'web',
  'arch.flow.webSub': 'nginx, estáticos',
  'arch.flow.api': 'API',
  'arch.flow.apiSub': 'Go',
  'arch.flow.db': 'PostgreSQL',
  'arch.flow.dbSub': 'datos y cola',
  'arch.flow.worker': 'worker',
  'arch.flow.workerSub': 'Go, proceso aparte',
  'arch.flow.scanner': 'escáner',
  'arch.flow.scannerSub': 'crypto/tls + en crudo',
  'arch.flow.target': 'servidor objetivo',
  'arch.flow.targetSub': 'puerto 443',
  'arch.flow.step1': 'El navegador solo habla con Caddy, que termina TLS y envía las rutas de la API a la API (Go) y el resto a la web (nginx).',
  'arch.flow.step2': 'La API valida el dominio y guarda el escaneo en PostgreSQL, que hace también de cola: no hay un broker de mensajes aparte.',
  'arch.flow.step3': 'Un worker, en otro proceso, reclama el escaneo con una concesión de tiempo limitado, ejecuta el escáner y guarda el resultado y su puntuación. Si muere a mitad, otro lo retoma.',
  'arch.flow.step4': 'El escáner solo se conecta al puerto 443 del servidor objetivo.',
  'arch.engine.title': 'Motor de escaneo',
  'arch.engine.p1':
    'La biblioteca estándar de Go (crypto/tls) hace los handshakes completos: uno con la oferta de un navegador actual, uno por versión de TLS y uno por grupo de intercambio de claves. Así se ve qué elige el servidor de verdad y si acepta los grupos híbridos post-cuánticos.',
  'arch.engine.p2':
    'Pero crypto/tls solo ofrece lo que considera seguro: no sabe pedir SSLv2 ni SSLv3, ni suites sin cifrado, EXPORT, anónimas o DES, ni la mayoría de los grupos. Para ver lo que acepta un servidor mal configurado, el escáner construye además ClientHello en crudo, byte a byte, con los registros de suites y grupos de IANA.',
  'arch.engine.p3':
    'Detecta las versiones de SSLv2 a TLS 1.3, las suites aceptadas en cada versión, los grupos clásicos y los híbridos post-cuánticos como X25519MLKEM768, el tamaño de los parámetros Diffie-Hellman y el certificado.',
  'arch.engine.p4':
    'Solo hace handshakes: no pide ninguna página, en las sondas en crudo no envía nada después del ClientHello (lee la primera respuesta y cierra) y no intenta explotar nada.',
  'arch.scoring.title': 'Modelo de puntuación (2026.2)',
  'arch.scoring.p1':
    'Una nota de 0 a 100 y una letra de A a F. La mitad de los puntos depende de que el servidor elija un intercambio de claves híbrido con ML-KEM cuando se conecta un navegador actual: es lo que protege hoy el tráfico frente a «cosechar ahora, descifrar después».',
  'arch.scoring.p2':
    'El resto premia TLS 1.3 (los grupos híbridos solo existen ahí), el secreto hacia adelante, tener más de un grupo híbrido y un certificado válido con renovación automatizada, porque pasar a certificados post-cuánticos exigirá reemitirlos todos.',
  'arch.scoring.p3':
    'Lo que está roto hoy limita la letra aunque sume puntos: SSLv2, suites sin cifrado o EXPORT, o un certificado no válido dejan una F. Un TLS clásico bien configurado y sin nada roto nunca baja de D.',
  'arch.scoring.p4':
    'Es determinista, y cada regla cita su fuente: un RFC, un CVE, un borrador del IETF, una publicación del NIST o una votación del CA/Browser Forum. Un test comprueba que ninguna se queda sin ella.',
  'arch.security.title': 'Seguridad del propio servicio',
  'arch.security.ssrf':
    'Anti-SSRF: el escáner resuelve el nombre y rechaza el dominio entero si alguna de sus direcciones es privada, reservada o local. Luego se conecta a la IP ya validada, nunca al nombre, para que una segunda respuesta DNS no pueda desviarlo, y la comprueba otra vez en el propio socket antes de conectar.',
  'arch.security.containers':
    'Contenedores endurecidos: ninguno corre como root, todos tienen el sistema de archivos de solo lectura, sin capacidades de Linux y con no-new-privileges. Solo Caddy da la cara a Internet; la base de datos está en una red interna, y el worker sale a Internet por una red propia desde la que no llega a Caddy ni a la API.',
  'arch.security.supply':
    'Cadena de suministro: las acciones de GitHub están fijadas por SHA. Cada imagen pasa por Trivy antes de publicarse: cualquier vulnerabilidad alta o crítica que ya tenga corrección bloquea la publicación. Las excepciones quedan documentadas con su motivo y caducan; al caducar, vuelven a bloquear. Las imágenes se publican en GHCR con una etiqueta inmutable por commit, y se despliega siempre por esa etiqueta.',
  'arch.quality.title': 'Calidad',
  'arch.quality.lab':
    'Laboratorio TLS con 10 servidores: 9 deliberadamente vulnerables (SSLv2, SSLv3, EXPORT, DH débil, curvas antiguas, Kyber experimental…) y uno bien configurado sin criptografía post-cuántica, solo en una red Docker interna sin salida a Internet: nunca se prueba contra terceros.',
  'arch.quality.testssl': 'Los resultados del escáner contra esos servidores se comparan con los de testssl.sh, una herramienta de referencia.',
  'arch.quality.fuzz': 'El código que lee las respuestas de los servidores se prueba con fuzzing: toda respuesta es una entrada no fiable.',
  'arch.ops.title': 'Operación',
  'arch.ops.backup':
    'Copia diaria de la base de datos, cifrada, en un almacenamiento externo al servidor. La clave con la que se sube no tiene permiso para borrar.',
  'arch.ops.restore': 'Cada mes, un ensayo automático restaura la última copia en una base de datos desechable y compara las filas de cada tabla con las de la copia.',
  'arch.ops.alerts': 'Si una copia o un ensayo falla o no llega a tiempo, salta una alerta.',
  'arch.ops.deploy': 'Cada despliegue comprueba la salud de los servicios al terminar; volver atrás es desplegar la etiqueta anterior.',
  'arch.dev.title': 'En desarrollo',
  'arch.dev.badge': 'en desarrollo',
  'arch.dev.lead': 'Esto existe en la rama principal, pero aún no está en producción:',
  'arch.dev.accounts': 'Cuentas y organizaciones, aisladas entre sí con Row Level Security de PostgreSQL.',
  'arch.dev.domains': 'Verificación de la propiedad de cada dominio con un registro DNS TXT.',
  'arch.dev.public': 'Escaneo público de cualquier dominio, con prueba de trabajo contra abusos.',
  'arch.dev.idor': 'Pruebas de acceso entre organizaciones, validadas con pruebas de mutación.',
  'arch.contact.title': 'Código',
  'arch.contact.text': 'Código disponible para revisión técnica bajo petición:',
  'arch.see.title': 'Verlo en marcha',
  'arch.see.examples': 'Ejemplos de informes',
  'arch.see.examplesText': ': fizsar.com y tres servidores del laboratorio.',
  'arch.see.report': 'Un informe real con «Bajo el capó» abierto',
  'arch.see.reportText': ': lo que el escáner ofreció a fizsar.com y lo que respondió.',

  'links.label': 'Más sobre QuantumReady',
  'links.examples': 'Ejemplos',
  'links.architecture': 'Arquitectura',

  'examples.meta.title': 'Ejemplos · QuantumReady',
  'examples.meta.description':
    'Informes reales de QuantumReady: fizsar.com, con intercambio de claves post-cuántico, y tres servidores de laboratorio aislados: uno bien configurado sin criptografía post-cuántica y dos con configuraciones antiguas o rotas.',
  'examples.title': 'Ejemplos',
  'examples.lead':
    'Informes reales, con el mismo formato que verás al escanear tu web: uno de fizsar.com y tres de servidores de laboratorio, uno bien configurado según los estándares de hoy y dos configurados mal a propósito.',
  'examples.labNote':
    'Los servidores de laboratorio viven en una red Docker interna, sin salida a Internet. Allí no hay certificados reales, así que su informe lleva uno válido simulado: la nota juzga solo la configuración de TLS.',
  'examples.nav': 'Ejemplos en esta página',
  'examples.heading': '{host} · {grade} · {score}',
  'examples.lab': 'Servidor de laboratorio aislado; no es un sitio real',
  'examples.real': 'Escaneo real del {date}',
  'examples.labDate': 'Escaneado en el laboratorio el {date}',
  'examples.uthLink': 'Abrir «Bajo el capó» de este informe',
  'examples.config': 'Cómo está configurado',
  'examples.why': 'Por qué saca esta nota',
  'examples.fix': 'Qué tendría que cambiar',
  'examples.fizsar.config':
    'TLS 1.2 y 1.3. Ante la oferta de un navegador actual elige el intercambio híbrido X25519MLKEM768. Certificado ECDSA de Let’s Encrypt que se renueva solo.',
  'examples.fizsar.why':
    'El tráfico ya va protegido frente a «cosechar ahora, descifrar después», y el resto está bien hecho. Pierde 5 puntos porque acepta un solo grupo híbrido.',
  'examples.fizsar.fix':
    'Para llegar a 100, aceptar también SecP256r1MLKEM768. El servidor web que usa todavía no permite activarlo.',
  'examples.modern.config':
    'TLS 1.2 y 1.3, solo intercambio ECDHE con cifrado autenticado (AES-GCM y ChaCha20-Poly1305) y los grupos x25519 y secp256r1. Ningún grupo post-cuántico. Es la configuración que recomiendan hoy las guías habituales.',
  'examples.modern.why':
    'Hoy es seguro: no hay nada roto ni débil, y tiene TLS 1.3, secreto hacia adelante y un buen certificado. Pero sin intercambio híbrido, el tráfico que se capture ahora podría descifrarse cuando exista un ordenador cuántico, y ese bloque, la mitad de la nota, se queda en 0.',
  'examples.modern.fix': 'Actualizar el software TLS a uno con ML-KEM y activar X25519MLKEM768: con eso pasaría a una A.',
  'examples.kyber.config':
    'Solo TLS 1.3, con X25519Kyber768Draft00 (un borrador de intercambio post-cuántico anterior al estándar ML-KEM) y x25519 clásico.',
  'examples.kyber.why':
    'Los navegadores ya no ofrecen ese borrador, así que la conexión acaba en x25519: no hay protección post-cuántica real y el intercambio de claves, la mitad de la nota, se queda en 0. Lo demás está bien: TLS 1.3, secreto hacia adelante y certificado.',
  'examples.kyber.fix': 'Actualizar el software TLS a uno con ML-KEM estándar y activar X25519MLKEM768.',
  'examples.export.config':
    'TLS 1.0 a 1.2 con suites EXPORT de 40 bits, DES simple, suites sin cifrado y anónimas, y un grupo Diffie-Hellman de 512 bits. Sin TLS 1.3.',
  'examples.export.why':
    'Cualquiera puede degradar esas conexiones o leerlas, así que la letra se queda en F. Solo puntúa el certificado: sin TLS 1.3 no hay intercambio post-cuántico posible.',
  'examples.export.fix': 'Retirar todas esas suites, dejar TLS 1.2 y 1.3 con suites AEAD y activar X25519MLKEM768.',
}

export type ShowcaseKey = keyof typeof es

const en: Record<ShowcaseKey, string> = {
  'uth.title': 'Under the hood',
  'uth.lead': "The scan's technical data as the API returns it: what the scanner tried and what the server answered.",
  'uth.download': 'Download JSON',
  'uth.downloadHelp': "The API's response for this scan, unchanged.",
  'uth.scan.title': 'Scan',
  'uth.scan.duration': 'Duration',
  'uth.scan.ip': 'Scanned IP',
  'uth.scan.resolved': 'Resolved IPs',
  'uth.scan.connections': 'Connections',
  'uth.scan.intensity': 'Intensity',
  'uth.scan.rawProbes': 'Raw probes',
  'uth.scan.budget': '{n} (budget exhausted)',
  'uth.intensity.basic': 'basic (crypto/tls only)',
  'uth.intensity.full': 'full (crypto/tls and raw ClientHello)',
  'uth.seconds': '{s} s',
  'uth.ms': '{ms} ms',
  'uth.none': 'none',
  'uth.yes': 'yes',
  'uth.no': 'no',
  'uth.source.cryptotls': 'crypto/tls',
  'uth.source.raw': 'raw',
  'uth.versions.title': 'TLS versions',
  'uth.versions.tried': 'Tried: {list}',
  'uth.versions.accepted': 'Accepted: {list}',
  'uth.groups.title': 'Key exchange groups',
  'uth.groups.offered': 'Browser-like offer: {list}',
  'uth.groups.selected': 'Chosen by the server: {group}',
  'uth.groups.selectedNone': 'Chosen by the server: none',
  'uth.groups.raw13': 'TLS 1.3 groups found by raw probes: {list}',
  'uth.groups.raw12': 'TLS 1.2 curves found by raw probes: {list}',
  'uth.col.group': 'Group',
  'uth.col.id': 'ID',
  'uth.col.hybrid': 'Hybrid',
  'uth.col.reply': 'Server reply',
  'uth.col.suite': 'Suite',
  'uth.col.fs': 'FS',
  'uth.col.aead': 'AEAD',
  'uth.col.notes': 'Notes',
  'uth.col.source': 'Probe',
  'uth.col.offered': 'Offered',
  'uth.suites.title': 'Accepted cipher suites',
  'uth.suites.lead': 'What the server accepted; the suites it refused are not listed.',
  'uth.suites.incomplete': 'Incomplete list: not every suite could be probed.',
  'uth.suites.none': 'No suite accepted.',
  'uth.note.export': 'EXPORT',
  'uth.note.anonymous': 'anonymous',
  'uth.note.null': 'no encryption',
  'uth.note.bits': '{bits} bits',
  'uth.dh': 'DH in {suite} ({version}): {bits}-bit prime',
  'uth.probes.title': 'Probes, one by one',
  'uth.probes.lead': 'Each row is one connection: what the scanner offered and the first thing the server answered.',
  'uth.probes.rawNote': 'The other raw probes (up to {n} in all) enumerate suites and groups; the API keeps what they found, in the tables above.',
  'uth.offer.default': 'Browser-like offer · groups: {groups}',
  'uth.offer.version': '{version} only',
  'uth.offer.group': 'Group {group} only ({id})',
  'uth.reply.hello': 'ServerHello',
  'uth.reply.ciphers': '{n} ciphers',
  'uth.reply.alert': 'Alert {code} {name}',
  'uth.reply.alertNoCode': 'Alert {name}',
  'uth.reply.ianaCode': 'code per IANA',
  'uth.reply.closed': 'Connection closed',
  'uth.reply.timeout': 'Timed out',
  'uth.reply.refused': 'Refused, no further detail',

  'footer.architecture': 'Architecture',
  'arch.meta.title': 'Architecture · QuantumReady',
  'arch.meta.description':
    'How QuantumReady works inside: the path of a scan, the TLS engine with raw ClientHello probes, the scoring model, the security of the service, testing and operations.',
  'arch.title': 'Architecture',
  'arch.lead': 'How QuantumReady works inside, at a high level.',
  'arch.flow.title': 'The path of a scan',
  'arch.flow.svgTitle': 'The path of a scan in QuantumReady',
  'arch.flow.svgDesc':
    'The browser connects to Caddy, which routes to the website (nginx) and the API (Go). The API stores the scan in PostgreSQL, which also serves as the queue. A worker claims it, runs the scanner and stores the result. The scanner only connects to port 443 of the target server.',
  'arch.flow.browser': 'Browser',
  'arch.flow.caddy': 'Caddy',
  'arch.flow.caddySub': 'TLS and routing',
  'arch.flow.web': 'web',
  'arch.flow.webSub': 'nginx, static',
  'arch.flow.api': 'API',
  'arch.flow.apiSub': 'Go',
  'arch.flow.db': 'PostgreSQL',
  'arch.flow.dbSub': 'data and queue',
  'arch.flow.worker': 'worker',
  'arch.flow.workerSub': 'Go, own process',
  'arch.flow.scanner': 'scanner',
  'arch.flow.scannerSub': 'crypto/tls + raw',
  'arch.flow.target': 'target server',
  'arch.flow.targetSub': 'port 443',
  'arch.flow.step1': 'The browser only talks to Caddy, which terminates TLS and sends API routes to the API (Go) and everything else to the website (nginx).',
  'arch.flow.step2': 'The API validates the domain and stores the scan in PostgreSQL, which is also the queue: there is no separate message broker.',
  'arch.flow.step3': 'A worker, in a separate process, claims the scan with a time-limited lease, runs the scanner and stores the result and its score. If it dies halfway, another one picks the scan up.',
  'arch.flow.step4': 'The scanner only connects to port 443 of the target server.',
  'arch.engine.title': 'Scanning engine',
  'arch.engine.p1':
    "Go's standard library (crypto/tls) performs full handshakes: one with a current browser's offer, one per TLS version and one per key exchange group. That shows what the server really chooses and whether it accepts the hybrid post-quantum groups.",
  'arch.engine.p2':
    "But crypto/tls only offers what it considers safe: it cannot ask for SSLv2 or SSLv3, for suites without encryption, EXPORT, anonymous or DES suites, or for most groups. To see what a misconfigured server accepts, the scanner also builds raw ClientHello messages, byte by byte, from IANA's suite and group registries.",
  'arch.engine.p3':
    'It detects the versions from SSLv2 to TLS 1.3, the suites accepted in each version, classical groups and hybrid post-quantum ones such as X25519MLKEM768, the size of the Diffie-Hellman parameters and the certificate.',
  'arch.engine.p4':
    'It only performs handshakes: it never requests a page, its raw probes send nothing after the ClientHello (they read the first answer and close), and it never tries to exploit anything.',
  'arch.scoring.title': 'Scoring model (2026.2)',
  'arch.scoring.p1':
    "A score from 0 to 100 and a grade from A to F. Half of the points depend on the server choosing a hybrid key exchange with ML-KEM when a current browser connects: that is what protects today's traffic against “harvest now, decrypt later”.",
  'arch.scoring.p2':
    'The rest rewards TLS 1.3 (hybrid groups only exist there), forward secrecy, having more than one hybrid group, and a valid certificate with automated renewal, because moving to post-quantum certificates will mean reissuing all of them.',
  'arch.scoring.p3':
    'What is broken today caps the grade whatever the points: SSLv2, suites without encryption or EXPORT suites, or an invalid certificate mean an F. A well-configured classical TLS setup with nothing broken never goes below D.',
  'arch.scoring.p4':
    'It is deterministic, and every rule cites its source: an RFC, a CVE, an IETF draft, a NIST publication or a CA/Browser Forum ballot. A test checks none is missing one.',
  'arch.security.title': 'Security of the service itself',
  'arch.security.ssrf':
    'Anti-SSRF: the scanner resolves the name and refuses the whole domain if any of its addresses is private, reserved or local. It then connects to the validated IP, never to the name, so a second DNS answer cannot divert it, and checks the address again on the socket itself before connecting.',
  'arch.security.containers':
    'Hardened containers: none runs as root, all have a read-only file system, no Linux capabilities and no-new-privileges. Only Caddy faces the Internet; the database sits on an internal network, and the worker reaches the Internet through its own network, from which it cannot reach Caddy or the API.',
  'arch.security.supply':
    'Supply chain: GitHub Actions are pinned by SHA. Every image goes through Trivy before it is published: any high or critical vulnerability that already has a fix blocks the release. Exceptions are documented with their reason and expire; once expired, they block again. Images are published to GHCR with an immutable tag per commit, and deployments always use that tag.',
  'arch.quality.title': 'Quality',
  'arch.quality.lab':
    'A TLS lab with 10 servers: 9 deliberately vulnerable ones (SSLv2, SSLv3, EXPORT, weak DH, old curves, experimental Kyber…) and one well configured without post-quantum cryptography, only on an internal Docker network with no way out to the Internet: third parties are never probed.',
  'arch.quality.testssl': "The scanner's results against those servers are compared with those of testssl.sh, a reference tool.",
  'arch.quality.fuzz': "The code that reads servers' answers is fuzzed: every answer is untrusted input.",
  'arch.ops.title': 'Operations',
  'arch.ops.backup': 'A daily encrypted backup of the database, stored outside the server. The key that uploads it has no permission to delete.',
  'arch.ops.restore': 'Every month, an automatic drill restores the latest backup into a throwaway database and compares each table’s rows with the backup.',
  'arch.ops.alerts': 'If a backup or a drill fails or does not arrive on time, an alert goes off.',
  'arch.ops.deploy': 'Every deployment checks the health of the services at the end; rolling back means deploying the previous tag.',
  'arch.dev.title': 'In development',
  'arch.dev.badge': 'in development',
  'arch.dev.lead': 'This exists on the main branch but is not in production yet:',
  'arch.dev.accounts': "Accounts and organizations, isolated from each other with PostgreSQL's Row Level Security.",
  'arch.dev.domains': 'Verification of domain ownership with a DNS TXT record.',
  'arch.dev.public': 'Public scanning of any domain, with a proof of work against abuse.',
  'arch.dev.idor': 'Cross-organization access tests, validated with mutation testing.',
  'arch.contact.title': 'Code',
  'arch.contact.text': 'Code available for technical review on request:',
  'arch.see.title': 'See it working',
  'arch.see.examples': 'Example reports',
  'arch.see.examplesText': ': fizsar.com and three lab servers.',
  'arch.see.report': 'A real report with “Under the hood” open',
  'arch.see.reportText': ': what the scanner offered fizsar.com and what it answered.',

  'links.label': 'More about QuantumReady',
  'links.examples': 'Examples',
  'links.architecture': 'Architecture',

  'examples.meta.title': 'Examples · QuantumReady',
  'examples.meta.description':
    'Real QuantumReady reports: fizsar.com, with post-quantum key exchange, and three isolated lab servers: one well configured without post-quantum cryptography and two with outdated or broken setups.',
  'examples.title': 'Examples',
  'examples.lead':
    'Real reports, in the same format you get when you scan your website: one of fizsar.com and three of lab servers, one well configured by today’s standards and two misconfigured on purpose.',
  'examples.labNote':
    'The lab servers live on an internal Docker network with no way out to the Internet. There are no real certificates there, so their report carries a simulated valid one: the grade judges the TLS setup only.',
  'examples.nav': 'Examples on this page',
  'examples.heading': '{host} · {grade} · {score}',
  'examples.lab': 'Isolated lab server; not a real website',
  'examples.real': 'Real scan of {date}',
  'examples.labDate': 'Scanned in the lab on {date}',
  'examples.uthLink': 'Open this report’s “Under the hood”',
  'examples.config': 'How it is set up',
  'examples.why': 'Why it gets this grade',
  'examples.fix': 'What it would need to change',
  'examples.fizsar.config':
    'TLS 1.2 and 1.3. Given a current browser’s offer it chooses the hybrid X25519MLKEM768 key exchange. ECDSA certificate from Let’s Encrypt, renewed automatically.',
  'examples.fizsar.why':
    'Its traffic is already protected against “harvest now, decrypt later”, and the rest is done right. It loses 5 points for accepting a single hybrid group.',
  'examples.fizsar.fix': 'To reach 100, also accept SecP256r1MLKEM768. The web server it uses cannot enable it yet.',
  'examples.modern.config':
    'TLS 1.2 and 1.3, ECDHE key exchange with authenticated encryption only (AES-GCM and ChaCha20-Poly1305) and the x25519 and secp256r1 groups. No post-quantum group. It is what the usual guides recommend today.',
  'examples.modern.why':
    'It is secure today: nothing is broken or weak, and it has TLS 1.3, forward secrecy and a good certificate. But without a hybrid key exchange, traffic captured now could be decrypted once a quantum computer exists, and that block, half of the score, stays at 0.',
  'examples.modern.fix': 'Update the TLS software to one with ML-KEM and enable X25519MLKEM768: that alone would make it an A.',
  'examples.kyber.config':
    'TLS 1.3 only, with X25519Kyber768Draft00 (a post-quantum key exchange draft that predates the ML-KEM standard) and classical x25519.',
  'examples.kyber.why':
    'Browsers no longer offer that draft, so connections end up on x25519: there is no real post-quantum protection and the key exchange, half of the score, stays at 0. The rest is fine: TLS 1.3, forward secrecy and the certificate.',
  'examples.kyber.fix': 'Update the TLS software to one with standard ML-KEM and enable X25519MLKEM768.',
  'examples.export.config':
    'TLS 1.0 to 1.2 with 40-bit EXPORT suites, single DES, suites without encryption and anonymous ones, and a 512-bit Diffie-Hellman group. No TLS 1.3.',
  'examples.export.why':
    'Anyone can downgrade those connections or read them, so the grade stays at F. Only the certificate scores: without TLS 1.3 no post-quantum key exchange is possible.',
  'examples.export.fix': 'Remove all those suites, keep TLS 1.2 and 1.3 with AEAD suites and enable X25519MLKEM768.',
}

export const showcaseDict: Record<Lang, Record<ShowcaseKey, string>> = { es, en }

/** Translator for the showcase strings, in the current UI language. */
export function useShowcaseT() {
  const { lang } = useLang()
  const t = (key: ShowcaseKey, vars: Record<string, string | number> = {}) =>
    showcaseDict[lang][key].replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m))
  return { lang, t }
}
