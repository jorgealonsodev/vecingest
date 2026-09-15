# Inventario de funcionalidad — App móvil, bloque 4/5

Filas 61 a 79 del índice `docs/design/stitch-screens.md` (79 pantallas totales del
proyecto Stitch `11075381530582947267`, "Vecingest APP"), orden alfabético desde
**"Proponer un punto"** hasta **"Zonas comunes"** inclusive. Otros tres agentes
cubren las filas 1-20, 21-40 y 41-60; un quinto cubre el proyecto de escritorio.

Fuentes cruzadas: `PRD_go.md` (tabla de hitos §10, líneas ~1542-1557; modelo de
datos §7.3, líneas ~490-784; API §7.4, líneas ~784-903), `api/openapi/openapi.yaml`
y `api/migrations/schema/*.sql` (lo que ya existe hoy).

Método: cada pantalla se descargó como HTML (`mcp__stitch__get_screen` →
`downloadUrl`) y se procesó con un script Python que extrae texto visible,
botones, inputs y enlaces sin volcar el HTML crudo al contexto.

## Lo que ya existe hoy (no se repite como pendiente)

12 endpoints, todos de auth/salud/perfil: `/v1/auth/{login,logout,refresh,
refresh/csrf,forgot-password,reset-password,superadmin/login}`,
`/v1/health/{live,ready}`, `/v1/me`, `/v1/me/sessions`, `/v1/me/sessions/{id}`.
Tablas: `users, sessions, password_reset_tokens, user_mfa, otp_challenges,
audit_log` (más las propias de River). Nada del dominio de comunidades,
incidencias, reservas, juntas, recibos o empresas existe todavía en base de
datos ni en el contrato OpenAPI.

---

## M0 — Base de auth (extensiones ya especificadas en el PRD, sin construir)

Estas pantallas no abren dominio nuevo: extienden la familia `/v1/me` que el
PRD ya nombra en §7.4 pero que el backend actual no implementa más allá de lo
listado arriba.

| Pantalla | Screen ID |
|---|---|
| Recuperar contraseña | `ba621c2b26b7433f812fd5ea34d0a929` |
| Verificar teléfono | `5ec90e06425542269b6c1ac2c79a9cf9` |
| Seguridad | `9e9573626b7640bab8747464b195ed9f` |

**Recuperar contraseña** es un calco de `POST /v1/auth/forgot-password`, ya
implementado; no aporta nada nuevo.

**Verificar teléfono** (paso 2 de 3 de un flujo de registro que esta pantalla
no completa por sí sola: no incluye "Crear cuenta" ni "Código de invitación",
que caen en otro bloque) pide un código SMS de 6 dígitos con reenvío
temporizado. El modelo de datos ya prevé esto exactamente:
`users.phone_verified_at` y `otp_challenges.purpose = 'phone_verify'`
(`otp_challenges` ya existe en `api/migrations/schema/00001_auth_schema.sql`).
Falta el endpoint que lo dispare y lo verifique; el PRD no lo nombra en su
lista de API (§7.4). Provisional: `POST /v1/me/phone` (solicitar código),
`POST /v1/me/phone/verify` (confirmar).

**Seguridad** es una pantalla de ajustes de cuenta que mezcla trabajo ya
resuelto con trabajo pendiente:
- "Sesiones activas" con botón "Cerrar" por sesión → ya cubierto por
  `GET /v1/me/sessions` y `DELETE /v1/me/sessions/{id}` (existen).
- "Exportar mis datos" → el PRD ya nombra `GET /v1/me/export` (RGPD) en §7.4;
  no implementado.
- "Eliminar mi cuenta" → el PRD ya nombra `DELETE /v1/me` (baja con
  anonimización) en §7.4; no implementado.
- "Verificación en dos pasos" (activar/desactivar) → tabla `user_mfa` ya
  existe; no hay endpoint. Provisional: `POST /v1/me/mfa`, `DELETE /v1/me/mfa`.
- "Cambiar contraseña" (autenticado, no por token de recuperación) → no existe
  ni tabla ni endpoint específico; el único cambio de contraseña actual pasa
  por el flujo de reset con token. Provisional: `PATCH /v1/me/password`.

---

## M2 — Incidencias completas

| Pantalla | Screen ID |
|---|---|
| Reabrir incidencia | `36fa3ce8fcfa4fe4a1b2b18118e0f24f` |
| Rechazar encargo | `c04b01c84b814dbab6e30e0336e0e65d` |

**Endpoints** (ambos ya nombrados literalmente en `PRD_go.md` §7.4, tabla de
transiciones §5.3 — no son invención):
- `POST /v1/incidents/{id}/transition` con `{to: "in_progress", note}` para
  reabrir.
- `POST /v1/incidents/{id}/decline` (empresa, motivo obligatorio) para
  rechazar la asignación.

**Tablas/conceptos**: `incidents (community_id, unit_id, status, category,
priority, assigned_company_id, reopen_count, rejection_reason, ...)`,
`incident_events (incident_id, from_status, to_status, note)`.

**Detalle de "Reabrir incidencia"**: la pantalla muestra "Puedes reabrir una
incidencia como máximo dos veces", que coincide exactamente con la transición
`resolved → in_progress` del §5.3 del PRD ("reabrir... máximo 2 veces") y con
la columna `incidents.reopen_count`. Es un límite de negocio, no solo de UI:
el backend tiene que rechazar la tercera reapertura.

**Detalle de "Rechazar encargo"**: el encabezado dice "Detalle De Tarea" y usa
un identificador `EXP-2024-0892` (formato distinto del `INC-2024-0892` que usa
"Reabrir incidencia" para el mismo tipo de entidad), pero el contenido
—asignación a empresa, motivo de rechazo obligatorio con radio buttons ("No
cubrimos esta zona", "Sin disponibilidad esta semana", "Fuera de nuestra
especialidad", "Otro") y texto libre— encaja con la transición
`assigned → open` del §5.3 ("empresa rechaza, motivo obligatorio; se notifica
al admin y se desasigna"), no con la tabla `tasks` (que no tiene aceptar/
rechazar en su máquina de estados). Es una incidencia, pese al rótulo de UI.
La discrepancia de formato del identificador (`EXP-` vs `INC-`) es una
inconsistencia de diseño a corregir, no dos entidades distintas.

### Fuera de hito claro: solicitud de presupuesto a empresa (§5.6, directorio)

| Pantalla | Screen ID |
|---|---|
| Solicitud respondida | `e18d7dd4bead45a0a24a78edfac12605` |

Esta pantalla (aceptar/rechazar un presupuesto formal de una empresa del
directorio, con desglose de IVA, PDF firmado y garantía legal) corresponde a
`service_requests` (§7.3) y a `PATCH /v1/service-requests/{id}` (responder,
aceptar, cerrar — nombrado literalmente en §7.4). **No mapea a ninguna fila
de la tabla de hitos M0-M8**: el PRD trata "Directorio de empresas" (§5.6)
como sección funcional propia, distinta de "Incidencias" (M2), y la tabla de
hitos no le asigna número. Lo más cercano por criterio funcional es M2
("vecino → admin → empresa → cierre"), pero `service_requests` es
explícitamente "una solicitud privada, no una incidencia" (§5.6), así que
forzarlo en M2 sería impreciso. Se deja aquí como advertencia para la fase de
fusión con los otros tres bloques.

**Tablas/conceptos**: `service_requests (company_id, user_id, unit_id,
description, status, quote_amount, quote_notes)`, `service_request_attachments`.

---

## M5 — Recibos y saldo, morosidad

| Pantalla | Screen ID |
|---|---|
| Recibos | `7357d43ea58444a5a892d70b681f4173` |

**Tablas**: `receipts (community_id, unit_id, debtor_user_id, concept, amount,
issue_date, due_date, status, paid_at, file_key)` — nombre y campos exactos
de §7.3.

**Endpoints**: el PRD no los lista en su resumen de API (§7.4 solo cubre
MVP fase 1 y un resumen de fase 2 centrado en juntas); son provisionales:
`GET /v1/units/{id}/receipts` (listado con filtro por ejercicio/tipo),
`GET /v1/receipts/{id}/download` (PDF).

**Punto ambiguo, no forzado**: la pantalla tiene un botón "Pagar recibo
pendiente" con icono de tarjeta. El §5.9 del PRD dice explícitamente que "el
cobro se gestiona fuera de la plataforma" y que el propietario "descarga los
recibos en PDF" — no hay mención a pago dentro de la app. Los propios recibos
pagados en el histórico dicen "Adeudo SEPA", lo que sugiere que el cobro real
es domiciliación bancaria gestionada por el administrador, no un pago
iniciado desde el móvil. No se propone un endpoint `POST /v1/receipts/{id}/pay`
porque el PRD no respalda esa función; puede que el botón solo navegue a
información de la domiciliación (`ES91 **** **** **** 4821`) en vez de
disparar un cobro real. Señalado como contradicción entre diseño y PRD, a
resolver con producto antes de implementar.

---

## M6 — Reserva de zonas comunes

| Pantalla | Screen ID |
|---|---|
| Zonas comunes | `9d9b93b953254b4585e4f07db7399cb7` |
| Reserva bloqueada por deuda | `a74bf4b8dc754a5591194ea12c9eaf84` |

**Tablas** (nombres exactos de §7.3): `common_areas (community_id, name,
capacity, opens_at, closes_at, slot_minutes, max_duration_minutes,
max_advance_days, max_bookings_per_unit_month, price, rules)`, `bookings
(common_area_id, unit_id, user_id, starts_at, ends_at, status, cancelled_at,
cancel_reason)`, `common_area_blocks`.

**Endpoints** (provisionales; §7.4 no los detalla, solo dice que existen en
fase 2): `GET /v1/communities/{id}/common-areas`, `GET
/v1/common-areas/{id}/availability`, `POST /v1/common-areas/{id}/bookings`,
`GET /v1/units/{id}/bookings` (para "Mis reservas activas").

**Requisito de blindaje (M6 — criterio de aceptación explícito del PRD, no
una preferencia de diseño)**: `PRD_go.md` §7.3 especifica la tabla `bookings`
con
```sql
exclude using gist (common_area_id with =, tstzrange(starts_at, ends_at) with &&)
  where (status in ('pending','confirmed'))
```
Esto exige la extensión `btree_gist` de Postgres. La pantalla "Reserva
bloqueada por deuda" pinta explícitamente turnos como "Ocupado" y
"Disponible" en un calendario de franjas — esa vista solo es correcta si el
backend impide a nivel de base de datos que dos vecinos reserven la misma
franja, que es justo el criterio funcional de M6 ("Dos vecinos no pueden
reservar la misma franja"). No basta con validarlo en la capa de aplicación.

**Segundo requisito, también explícito en el PRD (§5.7)**: "Reserva bloqueada
por deuda" implementa `debtors_lose_bookings` — "si está activo (solo con
acuerdo de junta registrado, art. 21.1 LPH), las viviendas con deuda vencida
no pueden reservar; el mensaje indica el acuerdo que lo habilita". La
pantalla cita literalmente "Art. 21.1 LPH" y "Medida acordada por la junta
ordinaria el 12 mar 2026", coincidiendo con el texto del PRD casi palabra por
palabra. Esto cruza con M5: el bloqueo depende de `receipts.status` /
morosidad, no es un campo propio de `bookings`.

---

## M7 — Juntas y voto online

| Pantalla | Screen ID |
|---|---|
| Proponer un punto | `085bf63a7886499fb65bd1cdbb9b12dc` |
| Resultado del punto | `1353ae94759041609e28ff2fcf972854` |
| Visor de documento (acta) | `634f16e39afa482fa52c1d145ebfd7c5` |
| Votar un punto | `d2617cd284784f34ab44bd5e9bd698b1` |
| Votar un punto (Modo oscuro) | `3f0ad429012f40bb83cb92d1f1d4b8e6` |
| Votar, privado de voto | `fd72b2fa38224f43b7c4f7774333014c` |
| Voto registrado | `62b2f10bc2424ac79b65061f3ac6d0e3` |

"Votar un punto" y su variante "(Modo oscuro)" son la misma pantalla en dos
temas — contenido idéntico, deduplicado aquí como una sola funcionalidad.

**Endpoints exactos, ya nombrados en `PRD_go.md` §7.4**:
- `POST /v1/communities/{id}/agenda-requests` — "Proponer un punto".
- `POST /v1/meeting-items/{id}/votes` — "Votar un punto" (OTP en remoto,
  bloqueado para deudores).
- `POST /v1/meeting-items/{id}/save-vote` — botón "Salvar mi voto en contra a
  efectos del art. 18" en la misma pantalla, coincide literalmente con
  "salvar el voto (art. 18.2)" del §5.8.

**Endpoints provisionales** (el PRD describe el comportamiento en §5.8 pero
no los nombra en la lista de §7.4):
- `GET /v1/communities/{id}/agenda-requests` (o `/v1/me/agenda-requests`) para
  el listado "Tus propuestas registradas" de la misma pantalla de proponer.
- `GET /v1/meeting-items/{id}/results` para "Resultado del punto".
- `GET /v1/meeting-items/{id}/votes/me` para el resguardo de "Voto
  registrado".
- `POST /v1/meeting-items/{id}/interventions` (o similar) para "Votar,
  privado de voto": el §5.8 dice que los privados de voto "pueden asistir y
  comentar pero no votar", pero no nombra el endpoint del comentario.

**Tablas** (nombres exactos de §7.3): `agenda_requests (community_id,
unit_member_id, title, description, status, meeting_id)`, `meetings`,
`meeting_items (majority_type, legal_basis, absent_vote_applies, ...)`,
`meeting_voters (is_debtor, debt_amount, coefficient, ...)`, `votes
(meeting_item_id, unit_id, choice, coefficient, prev_hash, hash, cast_at,
superseded_by)`, `meeting_item_results (yes_coef, no_coef, presumed_yes_coef,
quorum_coef, outcome, ...)`, `signature_evidence`, `chain_anchors`.

**Requisitos de blindaje explícitos, no preferencias de diseño**:
1. **Cadena de hash e inmutabilidad del voto**: "Voto registrado" muestra una
   "Firma criptográfica" (hash) copiable y dice "Cotejado y firmado mediante
   certificado digital". Esto es exactamente `votes.hash` / `votes.prev_hash`
   del §7.3 ("Registro inmutable: hash SHA-256 encadenado en `votes`") y la
   tabla `signature_evidence`. Cambiar de voto no debe sobrescribir el
   registro: el PRD dice "se guarda como nuevo registro en la cadena y cuenta
   el último" (`votes.superseded_by`).
2. **Voto presunto de ausentes, art. 17.8**: "Resultado del punto" dice
   textualmente "6 propietarios ausentes (12,6 %) tienen hasta el 25 oct para
   manifestar discrepancia (art. 17.8)" y computa el resultado "Aprobado
   provisionalmente" antes de que venza ese plazo. Esto es
   `meeting_item_results.presumed_yes_coef` / `presumed_yes_count` y la tabla
   `meeting_item_dissents` (discrepancias de ausentes) — el resultado inicial
   no es definitivo hasta el recálculo tras los 30 días naturales que marca
   el §5.8.
3. **Privación de voto por morosidad, art. 15.2**: "Votar, privado de voto"
   bloquea las tres opciones de voto (mostrando `lock`/`do_not_disturb_on`)
   para un propietario con recibos vencidos, pero permite la intervención
   escrita. Coincide con `meeting_voters.is_debtor` recalculado "al abrir la
   junta" (§5.8) — el bloqueo no es estático, se recalcula en ese momento
   concreto del ciclo de vida de la junta, no en el momento de publicar la
   convocatoria.

**Punto ambiguo — "Visor de documento"**: el contenido mostrado es
específicamente un acta de junta ordinaria (orden del día, quórum, acuerdos,
firma digital con CSV), lo que encaja con `meetings.minutes_file_key` y el
criterio funcional de M7 ("acta firmada y notificada"). Pero el PRD también
modela un repositorio genérico de documentos (§5.5, tabla `document_folders`
con carpeta "actas", endpoint `GET /v1/documents/{id}/download`). No queda
claro en el diseño si esta pantalla de visor es el mismo componente genérico
reutilizado para mostrar cualquier documento (incluidas actas subidas a esa
carpeta) o una vista dedicada al acta con datos que solo vive en `meetings`.
Se señala la ambigüedad en vez de forzar una tabla.

---

## M8 — Empresa: tareas, registro de facturas, fichaje

| Pantalla | Screen ID |
|---|---|
| Tareas | `c3eefd96744e4e6a9cb0b9a65e91d66e` |
| Registrar factura | `ecd1d7481a9146c48cb06ab2c8c8b074` |

**Tablas** (nombres exactos de §7.3): `tasks (company_id, incident_id,
community_id, title, status, scheduled_at, completed_at)`,
`task_assignments (task_id, user_id)`, `task_reports (task_id, user_id, body,
hours, file_keys)`, `invoices (company_id, community_id, incident_id, series,
year, number, issue_date, subtotal, tax_amount, withholding_amount, total,
status, file_key, reported_347)`, `invoice_lines (invoice_id, description,
quantity, unit_price, tax_rate, amount)`.

**Endpoints provisionales** (el PRD describe el comportamiento en §5.10 pero
no los nombra en §7.4, que solo llega hasta `GET /v1/companies/me/time-
reports` y `POST /v1/companies/me/documents` para certificados de empresa —
ninguno de los dos sirve para tareas o facturas): `GET
/v1/companies/me/tasks`, `POST /v1/tasks/{id}/reports`, `POST
/v1/companies/me/invoices`.

**Requisito de blindaje**: `PRD_go.md` §9 (Definition of Done) y la puerta de
seguridad de M8 exigen que fichajes y facturas sean *append-only*: "Una
factura enviada no se edita: se emite rectificativa" (§5.10). "Registrar
factura" no tiene ningún control visible en el diseño que impida editar una
factura ya en estado distinto de borrador — el diseño muestra un flujo lineal
de creación, sin distinguir "editar borrador" de "editar enviada". El control
de append-only tiene que vivir en el backend, no se puede inferir de esta
pantalla.

**Detalle de "Registrar factura"**: la numeración serie+número, la retención
IRPF opcional, el desglose de IVA por línea y el PDF adjunto coinciden campo
a campo con `invoices`/`invoice_lines` del §7.3. El vínculo "a encargo"
(`#ENC-2026-089`) es `invoices.incident_id`, pese a que el diseño lo llama
"encargo" y no "incidencia" — mismo patrón de nomenclatura suelta de UI que
en "Rechazar encargo".

---

## Pantallas sin determinar y filas no-pantalla

- Ninguna de las 18 pantallas reales de este bloque quedó sin propósito
  identificable; todas tienen título, contenido y al menos una acción clara.
- Fila no-pantalla omitida: **"Residential outdoor community barbecue area
  with stone grill..."** (`9a22ec3a5ca64096a079abb4c3a55392`, 1200×896) es
  fotografía de stock usada como fondo/ilustración para la sección de
  barbacoa en "Zonas comunes", no una pantalla de la app.
