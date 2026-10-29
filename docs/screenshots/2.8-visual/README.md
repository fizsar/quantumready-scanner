# Revisión visual del 2.8 (30/10/2026)

Chromium headless (zenika/alpine-chrome 124 con JetBrains Mono y Noto, para ver la tipografía real) contra el build de la web servido con la configuración de nginx de producción, en una red Docker interna. Para los informes se usaron las respuestas de la API generadas con el código actual a partir de escaneos reales: fizsar.com y `export` del 29/10/2026 (`internal/scoring/testdata/lab_v3`, con el registro por sonda) y fizsar.com analizado con el modelo 2026.2. Se revisaron 375, 768 y 1280 px, en ES y EN, con todos los paneles «Bajo el capó» y **todos** los `<details>` abiertos.

Aquí está el principio de cada página (`<página>-<idioma>-<ancho>.jpeg`) y, del informe de fizsar.com, el tramo de «Bajo el capó» con «Oferta completa» desplegada (`report-fizsar-*-under-the-hood.jpeg`). La revisión completa, página entera y con todo abierto, son unas 400 capturas: no se guardan.

## Fallos encontrados y arreglos

| # | Dónde | Fallo | Arreglo |
|---|---|---|---|
| 1 | «Bajo el capó», «Oferta completa» | El `<details>` caía en la primera columna (5,5 rem) de la rejilla de cada sonda: contenido apretado y hueco a la derecha. | Ocupa toda la fila (`grid-column: 1 / -1`). |
| 2 | «Bajo el capó», línea de resumen de cada sonda | Cortada: los elementos de una rejilla no se encogen por debajo de su contenido y la lista en monoespaciada no se partía. | `min-width: 0` y `overflow-wrap: anywhere` en las celdas. |
| 3 | «Oferta completa» a 768 y 1280 px (aparecía al arreglar el 1) | La página se ensanchaba a 3 600–3 800 px y se alargaba a 220 000: cada par etiqueta/valor era un `<div>` que caía entero en la columna `max-content`, con la lista completa sin partir. | `display: contents` en `.uth-fact`: etiqueta y valor son las dos celdas. |
| 4 | «Oferta completa» | La columna de etiquetas medía lo que la más larga («Firmas de certificado (signature_algorithms_cert)»): cada oferta con un ancho distinto y poco sitio para las listas. | Columna de etiquetas entre 7 y 13 rem, que se parten; en móvil, una sola columna. |
| 5 | Cabecera de la nota | «Preparación cuánticaPreparado75 de 100 puntos»: tres textos sin estilo, pegados, con mucho menos peso que la letra. | Dos tarjetas del mismo peso (lado a lado o apiladas): título, valor con el mismo tamaño y brillo (letra / nivel en su color), puntos y barra; la nota de los 85 y, si la letra está limitada, que lo está. Grupos con nombre para lectores de pantalla. |
| 6 | Paneles de riesgo y desglose | «Shor…», «Grover…» y los dos ejes del desglose eran `h3`/`h4` con el tamaño y los márgenes del navegador: más grandes que el título del panel y con mucho hueco. | `.h-threat` y `.h-axis2`: por debajo del título del panel, sin márgenes del navegador. |
| 7 | /examples | Los títulos de panel son `h3` (informes anidados) y conservaban el margen del navegador: hueco encima de «Riesgo cuántico». | `margin: 0` en `.h-section`. |
| 8 | /examples | El informe de cada ejemplo empezaba pegado a su presentación y nada separaba un ejemplo del siguiente. | Más espacio entre presentación e informe y una línea discontinua entre ejemplos. |
| 9 | Móvil (375 px) | Paneles y hallazgos con mucho relleno: el texto de un hallazgo quedaba en unos 260 px. | Menos relleno por debajo de 30 rem. |
| 10 | Diagrama (portada, /architecture) | Con JetBrains Mono, «servidor objetivo» y «crypto/tls + en crudo» llegaban al borde de su caja. | 10 px de margen a cada lado (el texto que no cabe se comprime, como antes). |
| 11 | /architecture, «Calidad» | Texto desfasado: «Laboratorio TLS con 10 servidores». | 12, con los dos de certificados ML-DSA (ES y EN). |

Sin desbordamientos de página en ninguna de las 42 combinaciones (14 páginas × 3 anchos) tras los arreglos. Para que no vuelva a pasar, `web/scripts/overflow-check.sh` abre ahora todos los desplegables, revisa también informes (2026.3 con registro por sonda, `export` y 2026.2) en ES y EN, y falla si algo se sale del viewport, se corta o se sale de su contenedor (las tablas que se desplazan dentro de su caja están permitidas). `web/src/reportHeader.test.tsx` comprueba que título, nivel y puntos de cada tarjeta son bloques separados.
