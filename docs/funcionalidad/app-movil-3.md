# Inventario de funcionalidad — App móvil, franja 3 (filas 41–60)

Franja alfabética de `docs/design/stitch-screens.md`: filas 41 a 60, de **"Iniciar
sesión"** a **"Professional outdoor padel court…"** (ambas inclusive). Proyecto
Stitch `11075381530582947267` ("Vecingest APP"), 79 pantallas en total; este
documento cubre 20 filas de esa tabla. Las franjas 1–20, 21–40 y 61–79 y el
proyecto de escritorio se documentan aparte; no se repite aquí nada de esas
franjas salvo referencia puntual.

Método: cada pantalla se descargó como HTML (`htmlCode.downloadUrl` de
`mcp__stitch__get_screen`) y se procesó con un script que extrae títulos,
botones, enlaces, campos de formulario y texto visible — no se ha pegado HTML
completo en el contexto de trabajo.

Los endpoints y tablas citados en M1–M8 no existen todavía: se toman
literalmente de la lista de endpoints y del esquema de datos que ya recoge
`PRD_go.md` (sección 7.4, endpoints, y el bloque de tablas hacia la línea 505 y
siguientes), no se inventan aquí, precisamente para que esta franja y las otras
tres puedan cotejarse sin ambigüedad. Donde el diseño implica algo que el PRD
no resuelve, se marca explícitamente como provisional.

API real hoy: 12 endpoints, todos de auth/health/perfil (`api/openapi/openapi.yaml`
es la fuente autoritativa). Tablas reales hoy: `users`, `sessions`,
`password_reset_tokens`, `user_mfa`, `otp_challenges`, `audit_log`, más las
propias de River. Nada más existe todavía.

---

## M0 — Auth (ya construido en este slice)

### Iniciar sesión — `c4453d1dd9a946f8890b185eff6eac34`

Ya implementado en `app/src/screens/LoginScreen.tsx`. El diseño pide email,
contraseña (con toggle de visibilidad), botón "Entrar", enlace "¿Has olvidado
la contraseña?" y enlace "Tengo un código de invitación", más un pie "Conexión
cifrada de alta seguridad".

- Lo construido cubre: formulario email/contraseña validado contra
  `schemas.LoginRequest`, `POST /v1/auth/login`, enlace a recuperación de
  contraseña.
- Lo que el diseño pide y no está: el enlace "Tengo un código de invitación"
  es visualmente fiel pero no tiene backend — no hay endpoint de aceptación de
  invitación en las 12 rutas actuales (ver M1). Documentado ya en el propio
  comentario de cabecera de `LoginScreen.tsx` y en `docs/pendientes-funcionalidad.md`.
  El selector de perfil "Vecino / Administrador / Empresa" que sí aparece en el
  diseño de escritorio fue retirado deliberadamente de la versión construida
  porque `POST /v1/auth/login` no acepta un parámetro de rol.

### Nueva contraseña — `502fdbc3adb84b7da3ae388ece6da9dd`

Ya implementado en `app/src/screens/ResetPasswordScreen.tsx`, contra
`POST /v1/auth/reset-password` (`ResetPasswordRequest`: `token` +
`new_password`). El diseño muestra dos campos (nueva contraseña / repetir),
una barra de fortaleza animada ("Débil", "Mínimo 12 caracteres") y un aviso
fijo de "Esta contraseña aparece en filtraciones conocidas".

- Lo construido cubre: los dos campos, el cierre de sesiones activas en otros
  dispositivos al cambiar la contraseña, y el aviso de contraseña filtrada
  como estado real servido por la API (`ErrorCode.AuthPasswordBreached`), no
  como texto estático.
- Lo que el diseño pide y no está: la barra de fortaleza animada — no hay
  endpoint que devuelva una señal de fortaleza antes de enviar el formulario;
  documentado en el propio comentario de cabecera del archivo.

---

## M1 — Comunidades, viviendas, invitaciones

### Invitación reconocida — `607f8aeb9682403987dc2b297615be76`

Paso 2 de 3 del alta por invitación: código verificado, muestra la comunidad
asignada (`C/ Mayor 12, Irun`), el inmueble (`Vivienda 3.º B`, planta 3), la
titularidad (`Propietario`, etiquetada "Art. 15 LPH") y el despacho
administrador. Acciones: "Crear mi cuenta", "No soy yo".

- Datos mostrados: comunidad, inmueble, titularidad, despacho colegiado.
- Backend implicado:
  - `POST /v1/auth/accept-invitation` (`{ token }` o `{ short_code }`) — ya
    está en el plan de endpoints del PRD, cubre el paso final ("Crear mi
    cuenta").
  - **Provisional** — no hay en el PRD un endpoint de *previsualización* del
    código antes de aceptarlo (esta pantalla necesita leer comunidad/inmueble/
    rol antes de que el usuario decida "Crear mi cuenta" o "No soy yo"). Algo
    como `GET /v1/invitations/resolve?token=…` no está enumerado en la
    sección 7.4 del PRD.
  - Tabla `invitations (community_id, unit_id, email, role, token_hash,
    short_code_hash, expires_at, accepted_at, sent_count, failed_attempts)`.
  - Al aceptar, resuelve en un `unit_members (unit_id, user_id, role, tenure,
    ...)`.
- Requisito legal explícito en la propia pantalla: el texto "Al continuar
  vincularás tu identidad a las actas, recibos y convocatorias oficiales de
  esta comunidad conforme a la Ley de Propiedad Horizontal" liga el alta a
  `unit_members.role`/`tenure`, que son los campos que después determinan
  quién vota (art. 15.1) y a quién le son válidas las notificaciones (art. 9).
  Un alta con rol o titularidad incorrectos no es solo un dato de perfil: es
  un defecto de legitimación para votar o recibir citaciones.

### Mi vivienda — `e172887f782349f9bd3be95a06fb8966`

Detalle de la vivienda del vecino: dirección, tipo, cuota de participación
(4,25 %), estado de pago ("Al corriente"), miembros (propietaria principal,
copropietario, ambos "Titular"), domiciliación (IBAN enmascarado + botón
"Cambiar"), y consentimiento de notificaciones electrónicas ("Canal telemático
habilitado, Aceptado el 3 mar 2026").

- Datos mostrados: `units.block/floor/door/type/participation_coefficient`,
  estado de pago derivado de `receipts`, `unit_members` (rol, titularidad),
  IBAN enmascarado, consentimiento electrónico con fecha.
- Backend implicado:
  - `GET /v1/communities/:id/units`, `PATCH /v1/units/:id`,
    `GET /v1/units/:id/members`, `PATCH /v1/units/:id/members/:memberId`.
  - Cambiar IBAN: campo `unit_members.iban_encrypted` (solo si
    `is_payer=true`), cifrado AES-256-GCM, nunca se devuelve completo por API
    (solo los 4 últimos dígitos) — coincide exactamente con el enmascarado
    `ES** **** **** **** 4821` del diseño. La pantalla dedicada "Cambiar IBAN"
    (`946660366172470b9362d15a944afc66`) queda fuera de esta franja.
  - Consentimiento electrónico: `unit_members.notification_address`,
    `electronic_notifications_consent_at`, `consent_text_version` — los tres
    campos existen ya en el esquema previsto y encajan con lo que pide la
    pantalla (canal + fecha de aceptación + versión del texto aceptado).
- Requisito legal explícito: "Comunicaciones oficiales y citaciones a juntas
  con plena validez jurídica conforme al Art. 9 de la Ley de Propiedad
  Horizontal" — el consentimiento electrónico no es una preferencia de UX,
  es la base legal para que una citación por app sustituya al correo postal;
  de ahí que el esquema fije `consent_text_version` (hay que poder probar qué
  texto aceptó el usuario y cuándo).

---

## M2 — Incidencias

### Nueva incidencia — `e3a86083f80b457bb4c22d48a6b6d381` / Nueva incidencia (Modo oscuro) — `b05e1402960e43dcbd71728876fe8e5f`

Mismo contenido en ambas variantes (solo cambia el tema). Formulario: título,
categoría (desplegable con 5 valores: Fontanería, Electricidad, Cerrajería,
Ascensores, Limpieza y mantenimiento), ámbito (Zona común / Mi vivienda),
descripción, hasta 5 fotos, prioridad sugerida (fija en "Normal", con nota "la
prioridad definitiva la fija el administrador"), aviso de incidencia
duplicada ("Ya hay una incidencia abierta de fontanería en el portal — Ver y
sumarme"), botón "Enviar incidencia".

- Backend implicado:
  - `POST /v1/communities/:id/incidents`.
  - `GET /v1/communities/:id/incidents` filtrado por categoría/estado, para
    la detección de duplicados antes de crear.
  - `POST /v1/incidents/:id/follow` para "sumarme" — incrementa
    `incidents.affected_count` vía `incident_followers (incident_id,
    user_id)`.
  - `POST /v1/incidents/:id/attachments` + `.../attachments/:attId/confirm`
    (hasta 5 fotos).
  - Tabla `incidents (community_id, unit_id, created_by, title, description,
    category, priority, status, scope, location_text, affected_count,
    ...)`.
- Nota de exactitud: el desplegable de categorías del diseño solo lista 5
  valores; el enum previsto en el PRD para `incidents.category` tiene 10,
  incluida "obras obligatorias art. 10 LPH" y "ruidos y convivencia" — el
  diseño no cubre el catálogo completo.
- La prioridad "definitiva la fija el admin" coincide literalmente con la
  regla de negocio ya descrita en el PRD para `incidents.priority`.

### Nueva incidencia sin conexión — `149367ae4844406b9d3328fe69ea7828`

Mismo formulario que "Nueva incidencia", con estado "Sin conexión. Se enviará
cuando recuperes red", fotos marcadas "Local" con icono de sincronización
pendiente, y botón "Guardar y enviar después" en vez de "Enviar incidencia".

- No implica un endpoint nuevo: cuando recupera conexión debe llamar a los
  mismos `POST /v1/communities/:id/incidents` +
  `POST /v1/incidents/:id/attachments` de la pantalla anterior.
- Sí implica un requisito de cliente que hoy no existe en ninguna de las 12
  rutas actuales: cola de creación local (borrador + adjuntos) con reintento
  al recuperar red. Es responsabilidad de la app móvil, no de un endpoint
  nuevo; se señala aquí porque condiciona el diseño del flujo de creación de
  incidencias (debe admitir un estado "pendiente de envío" antes de tocar la
  API).

### Panel de Administrador — `0ee2cb24f1464b11b117359f7063951f` (parte de incidencias)

Dashboard del despacho: contadores "Abiertas" (14), "Sin asignar" (3,
"requieren acción"), "Atención inmediata" (2, "Sin respuesta 48 h"), tiempo
medio de resolución (1,8 d); lista de incidencias urgentes con acciones
"Asignar" / "Seguimiento"; acceso directo "Registrar nueva incidencia"; nav
Panel/Comunidades/Incidencias/Avisos/Juntas/Despacho.

- Backend implicado:
  - `GET /v1/communities/:id/incidents` por comunidad — el diseño agrega
    varias comunidades del despacho en un único panel; el PRD no enumera un
    endpoint de agregación cross-comunidad para el despacho (**provisional**:
    algo como `GET /v1/offices/me/incidents` o un parámetro de agregación en
    el existente).
  - `POST /v1/incidents/:id/assign` (botón "Asignar").
  - El aviso "Sin respuesta 48 h" coincide con la regla ya descrita en el PRD
    ("Si no responde en 48 h, el admin recibe un aviso").
  - `GET /v1/offices/me`, `GET /v1/communities` para el nav "Comunidades" /
    "Despacho".
- La sección "Recordatorios legales" de esta misma pantalla es un requisito
  de M7, no de M2 — se documenta abajo, en M7, para no perder su naturaleza
  legal dentro de un apartado de métricas operativas.

---

## M3 — Avisos, documentos, directorio de empresas

*(Ningún screen de "Avisos" ni "Documentos" cae en esta franja; sí caen tres
del directorio de empresas y la pantalla de solicitudes del vecino.)*

### Mis solicitudes — `0182f6fbdd5944f9823a7d5cec49021e`

Lista de solicitudes de presupuesto del vecino, con pestañas Todas/Pendientes/
Respondidas/Aceptadas. Cada fila: empresa, estado, importe si lo hay, fecha y
motivo, acción "Revisar" cuando requiere decisión del vecino.

- Backend implicado: `GET /v1/companies/me/service-requests` no aplica (esa
  ruta es para la empresa); para el vecino sería el simétrico del lado
  `service_requests` filtrado por `user_id` — **no está enumerado** en la
  lista de endpoints del PRD (solo aparecen `POST /v1/companies/:id/service-requests`
  y `PATCH /v1/service-requests/:id`, sin un `GET` de listado propio del
  vecino). Marcado como hueco, no como invención: el PRD no lo resuelve.
- Tabla `service_requests (company_id, user_id, unit_id, description,
  address, contact_phone, share_contact_consent_at, status, quote_amount,
  quote_notes, answered_at, closed_at)`. Las pestañas del diseño
  (Pendientes/Respondidas/Aceptadas) son un subconjunto del enum de estado
  real (`pending | answered | accepted | rejected | done`); el diseño no
  muestra pestaña para `rejected` ni `done`.

### Pedir presupuesto — `157081ff7508445bbcc71457e7311153`

Formulario de solicitud a una empresa concreta: descripción (hasta 800
caracteres), fotos opcionales, checkbox "Autorizo compartir mi teléfono y la
dirección de mi vivienda con esta empresa para este presupuesto" con cita
expresa al "Art. 6.1.a del RGPD", plazo de atención estimado (24–48 h),
"Enviar solicitud" / "Cancelar".

- Backend implicado: `POST /v1/companies/:id/service-requests`.
- El checkbox de consentimiento mapea uno a uno con
  `service_requests.share_contact_consent_at` — es un consentimiento con
  marca de tiempo, no una casilla decorativa, y así lo trata ya el esquema
  previsto.
- Requisito legal (RGPD, no LPH): el consentimiento explícito para compartir
  teléfono/dirección con un tercero (la empresa) debe quedar registrado con
  fecha, exactamente como prevé `share_contact_consent_at` — cualquier
  implementación que comparta esos datos sin ese registro incumple la base
  legal que la propia pantalla invoca.

### Perfil de empresa (vista del vecino) — `78a73743bc9b442d9f61c413147ee7e2`

Ficha pública de una empresa del directorio: razón social, NIF, gremio, zona,
horario, seguro de responsabilidad civil, valoraciones con respuesta pública
de la empresa, botón "Pedir presupuesto".

- Backend implicado: `GET /v1/companies/:id`.
- Valoraciones: tabla `ratings (company_id, user_id, incident_id nullable,
  service_request_id nullable, score, comment, company_reply, replied_at)`.
  El PRD solo enumera `POST /v1/companies/:id/ratings` y
  `POST /v1/ratings/:id/reply`; no hay un `GET` de listado paginado de
  valoraciones — probablemente va embebido en `GET /v1/companies/:id`, pero
  el PRD no lo precisa.

### Perfil de empresa (autogestión de la empresa) — `f8507e39c4f24eb79ea3c3218af2bf13`

Pantalla de la propia empresa sobre sí misma: "Datos públicos", "Documentación
(1 caduca pronto)", "Equipo (3)", "Solicitudes de presupuesto (1 nueva)",
"Valoraciones (4,6/5, 38)", más nav inferior Bandeja/Tareas/Fichaje/
Facturas/Perfil y "Cambiar de comunidad o empresa" / "Cerrar sesión".

Las dos primeras secciones y el nav Tareas/Fichaje/Facturas pertenecen a M8
(ver abajo); "Solicitudes de presupuesto" y "Valoraciones" son directorio
(M3):
- `GET /v1/companies/me/service-requests` (ya enumerado en el PRD).
- Valoraciones: mismo hueco de listado que en la ficha del vecino.

---

## M7 — Juntas y voto online

### Juntas — `6c75125fbf6f48aaa9746de9ef3805ed`

Lista de juntas por estado: "Convocadas" (junta ordinaria, modalidad híbrida,
"Art. 16.2 LPH", orden del día resumido, "Delegación de voto disponible", "Ver
convocatoria"), "En votación" (vacío en este mock), "Cerradas" (dos juntas con
acta firmada, porcentaje de concurrencia por coeficiente, "Protocolizada y
remitida", "Descargar acta").

- Backend implicado:
  - `GET /v1/communities/:id/meetings`.
  - "Ver convocatoria" → detalle de una `meetings` concreta (fuera de esta
    franja, no hay pantalla de detalle de convocatoria aquí).
  - "Descargar acta" → `meetings.minutes_file_key`, URL prefirmada de
    descarga (mismo patrón que documentos: R2 + prefirmada de tiempo
    limitado).
  - "Delegación de voto disponible" → `vote_delegations (meeting_id,
    from_unit_id, from_user_id, to_user_id nullable, to_person_name
    nullable, ...)`, endpoint `POST /v1/meetings/:id/delegations`.
- Requisitos legales explícitos en la propia pantalla, todos ya modelados en
  el PRD:
  - **Art. 16.2 LPH** (convocatoria): la modalidad "Híbrida" solo es válida
    con base registrada (`meetings.mode_basis`: `bylaws` o
    `unanimous_consent`) — no es una preferencia de UI.
  - **Art. 16 / 19 LPH** (texto del pie de pantalla): "Convocatorias y actas
    formalizadas conforme a los Artículos 16 y 19... Los acuerdos adoptados
    obligan a todos los comuneros salvo excepciones legales previstas."
  - **Concurrencia por coeficiente** ("78,40% coef.", "82,15% coef."): el
    porcentaje de quorum se calcula sobre coeficiente de participación, no
    por cabeza — coincide con `meeting_item_results.quorum_coef` /
    `eligible_coef` del esquema previsto.
  - **Acta firmada / "Protocolizada y remitida"**: implica firma avanzada
    (OTP + sello de tiempo) de presidente y secretario, con plazo de 10 días
    naturales (art. 19.3), y cadena de hash + anclaje RFC 3161 —
    `signature_evidence`, `chain_anchors (chain='votes'|'time_entries'|
    'audit_log', method='tsa_rfc3161'|'email_to_board')`, `votes.prev_hash /
    hash`. La pantalla solo muestra el resultado ("Acta firmada",
    "Protocolizada y remitida"); el libro de actas con cadena de hash y el
    timestamping RFC 3161 son requisitos de fondo, no detalles visuales.
  - **Retención de al menos 5 años** (art. 19.4) de convocatorias, actas y
    delegaciones — no es visible en la pantalla pero condiciona cualquier
    implementación de borrado o purga de estas tablas.

### Panel de Administrador — `0ee2cb24f1464b11b117359f7063951f` (recordatorios legales)

Sección "Recordatorios legales" de la misma pantalla descrita en M2: "Firma
del acta C/ Mayor 12 — Quedan 4 días — Plazo de 10 días naturales para firma
de Presidencia y Administrador — Art. 19.3 LPH — Reclamar firma"; y "Pl.
Urdanibia 2 — 11 meses sin junta — Obligación legal de convocar junta
ordinaria anual... — Art. 16.1 LPH — Borrador orden del día".

- Backend implicado: ambos recordatorios se derivan de campos que ya existen
  en el esquema previsto, no de una tabla nueva:
  - Plazo de firma del acta: `meetings.minutes_signed_at` vs. el cierre de la
    junta + 10 días naturales (art. 19.3).
  - "11 meses sin junta": `communities.last_ordinary_meeting_at`, comparado
    contra la obligación anual del art. 16.1.
  - No hay un endpoint de "recordatorios legales" enumerado en el PRD;
    **provisional**: `GET /v1/offices/me/legal-reminders`, calculado a partir
    de los dos campos anteriores en vez de persistido.
- Este es exactamente el tipo de pantalla que la tarea pide señalar como
  requisito legal explícito: los dos plazos (10 días naturales, art. 19.3; un
  año, art. 16.1) son obligaciones de la Ley de Propiedad Horizontal, no
  simples recordatorios de producto — un incumplimiento tiene consecuencia
  legal para la comunidad, no solo una mala UX.

---

## M8 — Empresas: tareas, facturas y control horario

### Parte de trabajo — `0229fa0e20f446babb649672165f7317`

Parte de trabajo de un técnico sobre una incidencia asignada: horas dedicadas
(selector en pasos de 0,5 h), descripción de lo hecho (hasta 500 caracteres),
fotos de la intervención con hora de captura, botón "Cerrar parte", nota "se
remitirá firmado a la administración de fincas tras su cierre".

- Backend implicado:
  - Tabla `task_reports (task_id, user_id, body, hours, file_keys)` —
    coincide campo a campo con el diseño (horas, cuerpo de texto, fotos).
  - El parte cuelga de una `tasks (company_id, incident_id nullable,
    community_id nullable, title, description, status, scheduled_at,
    completed_at)`, no directamente de la incidencia — el diseño muestra el
    vínculo a través del número de incidencia/comunidad, pero el dato
    persistido es la tarea.
  - No hay endpoint de creación/cierre de partes enumerado en la sección 7.4
    del PRD (esa sección es un resumen, no lista todo fase 2).
    **Provisional**: `POST /v1/tasks/:id/reports` para crear el parte,
    `PATCH /v1/tasks/:id` con `status: done` para "Cerrar parte", siguiendo
    la misma convención REST que el resto de rutas ya enumeradas.
  - "Se remitirá firmado" implica alguna forma de firma o al menos evidencia
    de cierre — el PRD no detalla firma de partes de trabajo (a diferencia de
    las actas, donde sí exige OTP + sello de tiempo); queda como ambigüedad:
    no está claro si "firmado" aquí es una firma criptográfica real o una
    forma de hablar de "conformado".

### Perfil de empresa (autogestión) — `f8507e39c4f24eb79ea3c3218af2bf13` (Documentación, Equipo, nav)

Ver también M3 arriba para las partes de la misma pantalla que son de
directorio. Lo que corresponde a M8:

- "Documentación (1 caduca pronto)": tabla `company_documents (company_id,
  type, file_key, issued_at, expires_at, uploaded_by)` — el "caduca pronto"
  del diseño es exactamente el semáforo de vigencia que el PRD ya describe
  ("el admin ve un semáforo de vigencia al asignar").
- "Equipo (3)": tabla `company_members (company_id, user_id, role)`.
- Nav inferior "Tareas" → `tasks`/`task_assignments`; "Fichaje" →
  `time_entries (company_id, user_id, clock_in, clock_out, lat, lng, note,
  edited_at, edited_by, edit_reason, row_hash)`; "Facturas" → `invoices` +
  `invoice_lines`.
- Requisito legal explícito para "Fichaje" (no visible en esta pantalla
  concreta, pero es la puerta de entrada al módulo): registro horario legal
  español (art. 34.9 ET), conservación 4 años, inalterabilidad — de ahí el
  `row_hash` en `time_entries` y que solo el propio trabajador pueda corregir
  un fichaje en las 24 h siguientes (después, solo la empresa, y siempre
  queda en `audit_log`).
- "Facturas": la plataforma no emite factura, solo registra los datos y el
  PDF que sube la empresa (decisión de alcance ya tomada en el PRD para no
  quedar sujeta a Verifactu).

---

## Pantallas transversales (no atribuibles a un único hito)

### Inicio — `34a75f66fe58451d997d27394fca25d7` / Inicio (Modo oscuro) — `35dd3891e632453fbdfd075cc9047b2e`

Mismo contenido en ambas variantes (solo tema; el mock ni siquiera usa el
mismo nombre de marca en las dos — "Vencingest" vs. "Horizontalia" — que
parece un descuido del propio mock, no un dato funcional). Dashboard de
inicio del vecino: dirección de la vivienda, aviso fijado, dos incidencias
recientes con estado, próxima junta con fecha y convocatorias, recibo del mes
con importe y estado.

Es exactamente la pantalla que el propio PRD describe en 7.5 como "Inicio
(últimos avisos y estado de mis incidencias)": agrega piezas de M2
(incidencias con estado), M3 (aviso fijado), M5 (recibo pendiente) y M7
(próxima junta). No tiene modelo de datos propio: cada widget llama al
endpoint que ya se ha descrito en la milestone correspondiente
(`GET /v1/communities/:id/incidents` filtrado a "mías", el aviso fijado más
reciente vía `announcements`, la próxima `meetings`, el último `receipts`).
No se lista aquí un endpoint nuevo porque no lo hay: es una pantalla de
agregación, no una entidad.

### Más — `d68752ffaa5644c09c39a6cbad7338e9` / Más — `c342c3c46ed2456f89044ecbf4ec6eae`

Dos variantes casi idénticas del mismo menú "Más" del vecino (difieren en
iconografía y en el número de versión mostrado al pie — "2.4.1" vs. "3.14.2",
irrelevante funcionalmente). Es puro menú de navegación, sin datos propios:
Mi vivienda (M1), Juntas (M7), Reservas (M6 — sin pantalla en esta franja),
Recibos (M5 — sin pantalla en esta franja), Directorio de empresas (M3),
Notificaciones (M0/general), Seguridad (M0), Perfil (M0/M1), Ayuda y
normativa (sin milestone claro, probablemente contenido estático), y
"Cerrar sesión" con confirmación (`POST /v1/auth/logout`, ya construido).

No implica backend propio más allá de lo ya recogido en cada milestone de
destino.

---

## Pantallas cuyo propósito no se ha podido determinar

Ninguna. Las 18 pantallas de esta franja tienen un propósito identificable a
partir de su HTML.

## Filas no-pantalla de esta franja (se listan, no se analizan)

- **Logotipo Vencingest** (`eded32ea26d04441b698f40ba0b70edf`, 240×64) — asset
  de logotipo, no una pantalla.
- **Professional outdoor padel court with blue glass walls and blue turf,
  neat modern residential condominium facilities, daylight, architectural
  photography, no people** (`adcfc9945c9b43b3a85b7f0a63d86020`, 1200×896) —
  fotografía de stock (imagen de fondo para la ficha de la pista de pádel,
  probablemente usada en la pantalla "Calendario pista de pádel" de otra
  franja), no una pantalla de la app.
