# Funcionalidad de la consola web (Stitch) — inventario por hito

Estado a 2026-09-15. Auditoría de las 46 pantallas reales del proyecto Stitch
`14138416730329310203` ("VecinGest WEB", desktop, 1280 px), cuyo índice está en
`docs/design/stitch-screens-web.md`. De las 48 filas de ese índice se excluyen
`DESIGN.md` (copia embebida del design system) y "Logotipo Vecingest Pro"
(un asset), que no son pantallas.

Método: se descargó el HTML de cada pantalla (`htmlCode.downloadUrl` de
`mcp__stitch__get_screen`) y se procesó con un script que extrae título,
cabeceras de tabla, botones, enlaces, inputs, selects y chips de estado —
nunca se pegó el HTML completo en el contexto de trabajo. Dos pantallas,
"Deudores y certificados" (`04bd2f25a67f40f6b0bad9b65ddd618a`) y "Recibos"
(`7e89804d2e3d45e180ab448e9e7b43d5`), no devolvieron un `downloadUrl` sino un
`data:` URI embebido de 1280×1024 — la mitad de resolución del resto — con
sidebar y cabecera completos pero un `<main>` vacío: Stitch no llegó a generar
tabla ni tarjetas para ninguna de las dos. Su título y la pestaña de navegación
activa dan el alcance, pero no hay campos que extraer.

El objetivo es agrupar por hito del `PRD_go.md` (tabla de la sección 10, línea
~1493) qué endpoints y tablas nuevas implica cada pantalla, evitando listar
como pendiente lo que ya existe: hoy la API sólo tiene los 12 endpoints de
auth/salud/perfil de `api/openapi/openapi.yaml`, y la base de datos sólo tiene
`users`, `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges`,
`audit_log` y las tablas propias de River (`api/migrations/schema/`). No hay
comunidades, incidencias, juntas ni nada del dominio de negocio.

---

## M0 — Base (auth, `/me`, infraestructura)

Pantallas: Login web (`5f83c21f55d9493da67cc330fa8528d4`,
`2d4f7afe09fc4db18b425aeb88dc1bb1`; dos variantes de la misma pantalla, con
pestañas de rol Vecino/Administrador/Empresa), Código 2FA
(`d902704398cd41f3bc644698fa2b5e38`, `37ca6bfc56404f5998dc443b70007ba5`),
Sesión caducada (`7618a05ab7564c558069402e8819c3c4`), Seguridad
(`1f721fb92f31499bbf97fa0ae9434626`), Auditoría
(`11788b0cbdb54069907cdfed3a8991b7`), Jobs y salud — Superadmin
(`f06c2c6ede65414e8061d5a0d73af757`), 404 y Mantenimiento
(`2f0055280bd8444f9c33eb062d2f627e`, `cd15b4c3793840a6bbd01b8aa9b42b5b`).

**Ya existente, no pedir de nuevo:** login, logout, refresh + CSRF,
forgot/reset-password, login de superadmin, `/v1/me`, listar y revocar
sesiones (`/v1/me/sessions`, `/v1/me/sessions/{id}`), `/v1/health/*`. La tabla
`audit_log` y las tablas de River (colas de `worker`) también existen ya: las
pantallas de Auditoría y de Jobs y salud son, sobre todo, una interfaz sobre
infraestructura que ya está desplegada, no un dominio nuevo.

**Endpoints nuevos que implican estas pantallas:**
- `POST /v1/auth/2fa/verify` — el login no cubre el segundo paso; las dos
  pantallas de "Código 2FA" son un formulario de 6 dígitos independiente con
  su propio submit ("Verificar y acceder").
- `PATCH /v1/me/password` — cambio de contraseña autenticado (la pantalla de
  Seguridad pide contraseña actual + nueva); `reset-password` ya existente es
  el flujo de recuperación por token, no éste.
- `GET /v1/me/mfa/recovery-codes`, `POST /v1/me/mfa/recovery-codes/regenerate`,
  `POST /v1/me/mfa/reconfigure` — la tabla `user_mfa` existe, pero no hay
  endpoints para ver/regenerar códigos de respaldo ni reconfigurar la app.
- `GET /v1/admin/audit-log?user=&action=&community=&range=` y una exportación
  firmada (`GET /v1/admin/audit-log/export.{csv,pdf}`) — la pantalla ofrece
  "Verificar firma colegial" y "Descargar certificado de inalterabilidad" con
  una huella hash por fila, lo que la tabla `audit_log` actual (append-only
  simple) no calcula ni expone todavía.
- `GET /v1/admin/jobs`, `POST /v1/admin/jobs/{id}/retry`,
  `POST /v1/admin/queues/{name}/pause` — panel de observabilidad sobre River
  (colas, última ejecución, duración, reintentar, pausar colas no esenciales).

**Tablas/conceptos:** ninguno nuevo strictamente — es superficie de
administración sobre `users`, `sessions`, `user_mfa`, `audit_log` y River. Si
se añade firma/hash al log de auditoría, eso sí es una tabla o columna nueva.

Nota de mapeo: las pantallas de 404/Mantenimiento son una página de estado de
servicio (enlaces a aviso legal, monitor de estado, sello ISO/IEC 27001) sin
ningún dato de negocio; no encajan en ningún hito del PRD, son infraestructura
pura y se listan aquí sólo porque comparten el enrutado con el resto del
flujo de acceso.

---

## M1 — Comunidades, viviendas, invitaciones, portales vacíos

Pantallas: Comunidades (`3a685d03d6f844a0a7081c7fe1c6927e`), Despacho, datos y
miembros (`3290e51c371f4307b3460c95d451e5b1`), Despachos — Superadmin
(`10fef61a258f4848b778da27efc4201e`), Importar viviendas CSV
(`3ad375395ff34684922ba179801cefc5`), Panel general
(`88254e8fcc704e1291253ed95e1d894b`), Panel en carga / Skeleton
(`9657b329fd054ce8998f6e3a217dc119`), Panel sin comunidades
(`a07e848d1fe84347832805a853ed64a6`), Viviendas C/ Mayor 12
(`8c0cfb08c3a54a97bfc60697531e05c0`), Resumen C/ Mayor 12
(`1ad4966541b74637975b2e33c8fd59f7`, ficha-hub con pestañas Datos / Economía /
Junta directiva / Cumplimiento — las tres últimas dependen de datos de M5 y
M7, se listan aquí porque la ficha en sí es de comunidad). Selector de
contexto (`537dd208d9d649b6a56afa3f29f51de1`, `f0332c6fb21a41e18f769038896674ec`)
se incluye aquí con reserva: es tan de M0 (ocurre justo tras el login) como de
M1 (lista despachos/comunidades por rol — "Administración Ríos
administrador", "Fincas Bidasoa personal", "C/ Mayor 12 propietario" —, algo
que no existe sin membresías multi-tenant).

**Endpoints:**
- `GET/POST /v1/offices`, `GET/POST /v1/offices/{id}/members`
- `GET/POST /v1/communities`, `GET/PATCH /v1/communities/{id}`
- `POST /v1/communities/{id}/units/import` (CSV, con fila-a-fila de
  validación: portal/piso/puerta/tipo/cuota/titular/DNI-CIF y estado por fila)
- `GET /v1/communities/{id}/units`, `GET /v1/units/{id}`
- `POST /v1/communities/{id}/invitations`
- `GET /v1/me/memberships` (para el selector de contexto)

**Tablas:** `office`, `community`, `unit` (`flat`/`premises`/`garage`/
`storage`), `unit_members` (owner/tenant, con coeficiente de participación),
`invitation`, `board_role` (presidente/vicepresidente/secretario, ya aparece
como columna "Junta directiva" en la ficha de comunidad).

**Carga legal:** la tabla de Comunidades muestra una columna "Consentimiento
digital" — el consentimiento expreso de cada propietario para notificación
electrónica es un requisito de la LPH para que las comunicaciones telemáticas
sean válidas, no un campo decorativo. La importación CSV valida fila a fila
antes de guardar (columna "Estado de validación"), coherente con la puerta de
seguridad de M1 sobre inyección de fórmulas y *path traversal* en CSV.

---

## M2 — Incidencias

Pantallas: Bandeja de incidencias (`ab29ab847a684a36836d2d1bdbc93107`),
Bandeja con panel de asignación (`8894c172aafe497eab1dba68947b410e`), Detalle
de incidencia (`7a58d879f3f040aab1930b994dbd0303`), Rechazar incidencia
(`54c259325e194082bfffd2a49a50dae5`), Incidencias sin asignar y estado vacío
(`48edaf0a6212468f8366d091031cdd1e`), Error de carga en incidencias
(`941a6d9fca804e3099c555360fb9e733` — variante de estado de error/reintento
de la bandeja, no una función nueva).

**Endpoints:**
- `GET /v1/communities/{id}/incidents?status=&priority=&category=&company_id=`
- `POST /v1/communities/{id}/incidents`
- `PATCH /v1/incidents/{id}` (asignar proveedor, rechazar con motivo,
  transición de estado: abierta/asignada/en curso/resuelta/cerrada)
- `POST /v1/incidents/{id}/comments` (con visibilidad interna/pública —
  botón "Interno (Empresa)" vs "Público (Vecinos)")
- `GET /v1/incidents/{id}`, `GET /v1/companies` (directorio de proveedores
  para el selector de asignación)

**Tablas:** `incident` (`community_id`, `status`, `priority`, `category`,
`company_id`, `assigned_at`, presupuesto máximo autorizado), `incident_comment`
(con campo de visibilidad), `company`/`provider`.

**Carga legal:** la pantalla de detalle muestra explícitamente los artículos
LPH 10.1.a (obras necesarias de conservación, justifica quién paga una avería
como "Ascensor parado") y 20 (funciones del administrador) como chips junto
al proveedor asignado — el reparto de coste de una incidencia no es un texto
libre, tiene que poder citar el fundamento legal.

---

## M3 — Avisos, documentos, directorio

Pantallas: Avisos (`a268b92a22cc45fbb1e2958e2abb1fac`), Nuevo aviso
(`3845fea6b3f444599b307da9207d22fd`), Aprobar aviso del presidente
(`d2b1afc67ef94b488deec04297f77309`), Documentos C/ Mayor 12
(`b0a8ac80aa2842a3a286a2d9ebe94e2c`).

**Endpoints:**
- `GET /v1/communities/{id}/announcements?status=`
- `POST /v1/communities/{id}/announcements`
- `PATCH /v1/announcements/{id}` (aprobar, publicar, programar, fijar/`pin`)
- `GET /v1/communities/{id}/documents?category=`
- `POST /v1/communities/{id}/documents`
- `GET /v1/documents/{id}/download` (URL prefirmada, nunca `file_key` público)
- `GET /v1/documents/{id}/access-log`

**Tablas:** `announcement` (`community_id`, `status`:
borrador/pendiente_aprobacion/programado/publicado, segmento de destinatarios,
fecha de fijado/caducidad), `document` (categoría: actas/estatutos/
presupuestos/seguros/contratos/otros, versión, ámbito propietarios/junta
directiva).

**Carga legal:** un aviso de tipo convocatoria lleva el chip "LPH Art. 16" y
pasa por un paso de aprobación del presidente con "Validación jurídica",
censo de notificación y log de auditoría del trámite antes de publicarse —
es decir, un aviso ordinario y una convocatoria de junta comparten pantalla
de listado pero no el mismo flujo de publicación. La carpeta de documentos
muestra "Custodia Legal Certificada (Art. 9 LPH)" y una carpeta específica
"Libro de Actas Diligenciado": la retención de actas no es almacenamiento de
ficheros normal, es custodia legal obligatoria — se conecta con el libro de
actas de M7.

---

## M4 — Publicación, RGPD, backups

Pantalla: Protección de datos (`f73780efe3f741e786e1ebf8d50e0c89`).

**Endpoints:**
- `GET /v1/offices/{id}/dpa-contracts`,
  `POST /v1/offices/{id}/dpa-contracts/{community_id}/sign`
- `GET /v1/offices/{id}/processing-records` (RAT), con exportación
  `.xlsx`/`.pdf`
- `GET /v1/offices/{id}/gdpr-requests`,
  `POST /v1/gdpr-requests/{id}/respond`

**Tablas:** `dpa_contract` (encargado del tratamiento, `dpa_signed_at`,
por comunidad), `processing_activity_record`, `gdpr_request` (tipo,
interesado, vivienda vinculada, fecha de recepción, plazo legal máximo,
estado).

**Carga legal:** la pantalla cita explícitamente RGPD Art. 28 (contrato de
encargo de tratamiento) y LOPD-GDD Art. 33 para los plazos de respuesta a
derechos de los interesados — coincide con la puerta de seguridad de M4
("Registro de actividades de tratamiento y contrato de encargo firmados con
los despachos piloto"). Nota de mapeo: esa puerta de M4 es un hito puntual de
lanzamiento; esta pantalla es una consola de gestión continua (bandeja de
solicitudes con plazo corriendo), más amplia que el criterio de M4 tal como
está redactado. Se deja en M4 por ser el hito más cercano, no por encajar del
todo.

---

## M5 — Recibos y saldo, morosidad

Pantallas: Recibos (`7e89804d2e3d45e180ab448e9e7b43d5`) y Deudores y
certificados (`04bd2f25a67f40f6b0bad9b65ddd618a`) — **ambas con `<main>`
vacío**, ver nota de método al principio: sólo se puede documentar que
existen (por su título y su entrada de navegación activa "Recibos" / "Fiscal
y Deudores"), no qué columnas o acciones tienen. Además, Nueva convocatoria:
Paso 5 Deudores (`4b362c2b0d184bdfaa76f2f4f86c5bf4`), que sí trae contenido y
es la que realmente documenta el dominio de morosidad.

**Datos que sí se pudieron extraer** (de la pantalla de convocatoria, Paso 5):
tabla Vivienda | Titular registral | Deuda vencida | Recibos pendientes |
Estado de voto.

**Endpoints:**
- `GET /v1/communities/{id}/receipts?unit_id=&status=`
- `GET /v1/units/{id}/balance`
- `GET /v1/communities/{id}/debtors`
- `POST /v1/communities/{id}/debt-certificates`

**Tablas:** `receipt` (`community_id`, `unit_id`, periodo, importe, estado),
vista o flag de deudor por `unit`, `debt_certificate` (emitido por, emitido
en — con "acceso registrado" según la puerta de seguridad de M5).

**Carga legal:** el estado de deudor calculado aquí es el que bloquea el
derecho a voto bajo LPH Art. 15.2 en las pantallas de M7 (Paso 5, Abrir
junta, Punto cerrado) — Recibos/Deudores es la fuente de verdad de la que
depende esa regla, no un módulo aislado.

---

## M6 — Reservas de zonas comunes

Ninguna de las 46 pantallas documenta esta función en detalle. El enlace
"Reservas" aparece en el menú lateral de prácticamente todas las pantallas de
administrador, pero no hay una pantalla propia de calendario/reserva en este
proyecto de Stitch — es un hueco del diseño, no del backend: no se puede
inventar su contenido a partir de un enlace de menú.

---

## M7 — Juntas y voto online

El bloque con más peso legal del conjunto. Pantallas: Nueva convocatoria —
Paso 1 Tipo y modalidad (`0fcdfd559fcd4887b0e20e6c6916dd56`), Paso 2 Fechas
(`e3294b263de9404fad24eb608fdd5eb8`), Paso 3 Orden del día
(`2338c3d84d9e431aae0f55797d36fdc9`), Paso 6 Revisar y publicar
(`d5cfe2f806d94c429fef5e8c69250a5c`) — el Paso 5 (Deudores) se documenta en
M5 —, Asistencia y quórum de junta (`4b185b977d1a452b96c36041aac188b0`),
Abrir junta y control de deudores (`ce9c4b5983d94063998d76f025cfd5a5`), Punto
en votación en directo (`854933893a1c4835840693df751e0019`), Punto cerrado y
cómputo de ausentes (`80bc21936ac443cd9049d0b73ac14617`), Acta de junta y
firmas (`2808c40a8f2847b9801f8dd4f58bdffc`). También pertenece aquí, aunque es
una pantalla de superadmin, Reglas legales
(`d190292747af4cb6b20dc08d6f05e535`): es la consola que edita la tabla
`legal_rules` descrita en la sección 9 del PRD (mayorías, plazos, con
vigencia por fecha), y M7 es el hito que más depende de que ese motor exista.

**Endpoints:**
- `POST /v1/communities/{id}/meetings` (tipo, modalidad, convocante)
- `PATCH /v1/meetings/{id}` (fechas de primera/segunda convocatoria, lugar)
- `POST /v1/meetings/{id}/agenda-items` (con régimen de mayoría por punto)
- `POST /v1/meetings/{id}/publish` (genera cédula de citación fehaciente)
- `GET /v1/meetings/{id}/debtors` (para excluir voto)
- `POST /v1/meetings/{id}/attendance` (presente/representado/remoto/ausente,
  con delegaciones)
- `POST /v1/meetings/{id}/open`
- `POST /v1/agenda-items/{id}/votes`, `POST /v1/agenda-items/{id}/close`
- `GET /v1/agenda-items/{id}/results`
- `POST /v1/meetings/{id}/minutes`, `POST /v1/minutes/{id}/sign` (2FA/OTP)
- `GET /v1/meetings/{id}/verify-chain` (coincide literalmente con el
  criterio de M7: "comando de línea `vecingest verify-chain <meeting_id>`")
- `GET/POST /v1/admin/legal-rules` (alta de nueva versión con vigencia y
  fuente normativa)

**Tablas:** `meeting` (`community_id`, tipo, modalidad, convocante,
`first_call_at`, `second_call_at`, lugar), `meeting_item`/agenda (régimen de
mayoría por artículo 17.x, flag de "coste sólo a quien vote a favor"),
`vote` (`agenda_item_id`, `unit_id`, sentido, canal, campos de cadena de hash
`head_hash`/`prev_hash`), `vote_delegation`, registro de asistencia, `minutes`,
`signature_evidence` (canal `email`/`totp`, rol del firmante), `legal_rules`
(clave, valor, vigente_desde/hasta, fuente normativa).

**Carga legal explícita en las pantallas** (esto es lo que este bloque tiene
de distinto a una tabla más):

- **Doble convocatoria (Art. 16.2/16.3 LPH):** el Paso 2 trae un botón
  "Ajustar automáticamente a fecha mínima hábil" con el texto "Criterio legal
  de doble convocatoria" — el cálculo de la segunda fecha no es libre.
- **Mayorías por punto, no globales (Art. 17):** el Paso 3 tiene un selector
  de "Apartado del art. 17" por cada punto del orden del día (17.1, 17.2,
  17.6, 17.7 aparecen como opciones reales), más un flag "el coste solo se
  repercute a quien vote a favor (art. 17.1 in fine)" — confirma que
  `legal_rules` tiene que resolverse por punto y por fecha de vigencia, no
  como una constante.
- **Exclusión de deudores del voto (Art. 15.2 LPH), con excepciones:** además
  del filtro simple en el Paso 5, la pantalla "Abrir junta" tiene botones
  "Impugnada judicialmente" y "Consignada / Pagada" — la deuda impugnada o ya
  consignada no puede tratarse igual que una deuda firme a efectos de
  privación de voto, así que el modelo de datos necesita más que un booleano
  "es moroso".
- **Cómputo de ausentes con plazo (Art. 17.8 LPH):** "Punto cerrado" calcula
  un 12,60% de "Ausentes Art. 17.8" y muestra el plazo exacto para oponerse:
  "25 Octubre 2026 · 23:59 CET (30 días naturales)" — coincide con los plazos
  del Art. 19.4 que el PRD marca como no codificables como constante.
- **Cadena de hash verificable:** el enlace "Verificar cadena de hash en nodo
  colegiado" en Punto cerrado, y el botón "Verificar cadena y sello
  criptográfico" en el Acta, son la interfaz de usuario del requisito
  `vecingest verify-chain` y del anclaje RFC 3161 de M7.
- **Doble firma preceptiva y libro de actas:** el Acta exige firma de
  presidente y secretario por separado ("Doble Firma Preceptiva del Libro"),
  cada una con 2FA o certificado FNMT, cita Art. 19.2 (contenido mínimo del
  acta: convocatoria, autor, quorum, censo, orden del día, acuerdos) y Art.
  19.3 (remisión fehaciente), y termina con una acción "Diligenciar y
  Sellar" — el flujo de firma no es un solo clic de aprobación, son dos
  identidades distintas con evidencia de firma propia cada una
  (`signature_evidence` por firmante, tal como pide el hito).

---

## M8 — Empresa: tareas, registro de facturas, fichaje

No hay ninguna pantalla en este conjunto de 46 que documente `time_entry`,
`invoice` ni gestión de tareas de una empresa proveedora. La única pantalla
del dominio "empresa" es **Empresas pendientes de verificación — Superadmin**
(`b7bb87206a25489292380475ba47d7a7`), y **no encaja limpiamente en M8**: su
contenido es un alta/verificación de proveedores a nivel de plataforma (NIF,
gremios declarados, cotejo contra el BORME, documentación preceptiva como
seguros, verificar/rechazar/suspender), no el criterio funcional de M8
("informe mensual de jornada exportado y factura vinculada a incidencia").
Es más cercana a un prerrequisito de directorio de proveedores (M1/M2) hecho
desde la consola de superadmin que a una función del propio hito M8. Se deja
aquí por ser el hito de dominio más próximo, señalando la discrepancia en vez
de forzarla.

**Endpoints (sólo para esta pantalla, no para el resto de M8):**
- `GET /v1/companies?status=pending`
- `POST /v1/companies/{id}/verify`, `POST /v1/companies/{id}/reject`,
  `POST /v1/companies/{id}/suspend`
- `GET /v1/companies/{id}/documents`

**Tablas:** `company` (NIF, gremios, provincia, estado de verificación),
`company_document` (tipo, estado — "Cotejado BORME", "Válida").

---

## Pantallas cuyo propósito no se pudo determinar

Ninguna. Las 46 pantallas tienen un título y una posición de navegación que
identifican su propósito sin ambigüedad. La única limitación real es de
**contenido**, no de propósito: "Recibos" y "Deudores y certificados"
(sección M5) devolvieron un `<main>` vacío en Stitch, así que se sabe qué son
pero no qué campos o acciones tendrían.
