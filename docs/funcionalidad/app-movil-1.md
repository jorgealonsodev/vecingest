# Inventario de funcionalidad — App móvil (bloque 1/4)

Cubre las filas 1 a 20 de `docs/design/stitch-screens.md`, orden alfabético
**"Acta" → "Delegación activa"** (19 pantallas reales; la fila 12 de esa
tabla, "Clean modern condominium multipurpose community room...", es una
imagen de fondo del design system, no una pantalla). El resto del proyecto
Stitch `11075381530582947267` (filas 21-79) lo cubren otros tres bloques
paralelos; este documento no repite ni corrige su contenido.

Fuente: HTML descargado y parseado de cada pantalla vía
`mcp__stitch__get_screen` (proyecto "Vecingest APP", `deviceType: MOBILE`).

Línea base a fecha de este audit — **no se lista como pendiente**: la API
tiene exactamente 12 endpoints (auth, health, `/v1/me` y sesiones), ver
`api/openapi/openapi.yaml`. Las únicas tablas existentes son
`users, sessions, password_reset_tokens, user_mfa, otp_challenges` (auth),
`audit_log`, y las propias de River. No hay `communities`, `units`,
`incidents` ni ninguna otra tabla de dominio.

Convención de nombres: inglés para tablas/campos/rutas (según el glosario de
`PRD_go.md` §12), español solo en el texto visible de la pantalla. Todo
endpoint o tabla que el diseño no fija con precisión se marca
**(provisional)** — es una propuesta razonable a partir de la pantalla, no
un contrato ya decidido.

---

## M1 — Comunidades, viviendas, invitaciones, portales vacíos

### Pantallas

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Código de invitación | `4b98138c2d4747a7bd0567e5ac174f2a` | Introducir el código alfanumérico (8 caracteres) que vincula al vecino con su vivienda. |
| Crear cuenta | `5c6f491180994d9184ec675a04110286` | Alta de cuenta tras validar el código: nombre, contraseña, teléfono, aceptación de condiciones/privacidad, preferencia de canal para convocatorias (app+email vs. papel). |
| Código de verificación | `f209627c0f084971bef0a8968723f18a` | Verificación TOTP obligatoria "para administradores" (paso 2 de 2), con código de recuperación de 8 caracteres como vía alternativa. |
| Comunidad, resumen | `c7583dc16cb84abe8fa6dd8b57b52c42` | Ficha/panel de una comunidad desde el rol de administrador de fincas (despacho). Ver desglose abajo — mezcla datos de varios hitos y algunos elementos sin hito claro. |

### Datos que muestran

- **Código de invitación**: formato del código (8 caracteres alfanuméricos), origen esperado (convocatoria impresa o correo de bienvenida).
- **Crear cuenta**: nombre y apellidos, contraseña (mínimo 12 caracteres con medidor de fortaleza — coincide con la política de 15/12 caracteres de `PRD_go.md` §5.1 para cuentas sin/con 2FA), teléfono con prefijo `+34`, checkbox de condiciones/privacidad, checkbox de canal de convocatorias.
- **Código de verificación**: código TOTP de 6 dígitos con cuenta atrás de caducidad; código de recuperación de 8 caracteres.
- **Comunidad, resumen**: dirección, CIF y número de colegiado; contadores (viviendas, incidencias, avisos, documentos, juntas, recibos); vivienda/local desglosado (24 viviendas + 4 locales); presidencia en ejercicio con vigencia de mandato; **fondo de reserva con aviso de incumplimiento legal** ("Por debajo del 10 % legal obligatorio — Art. 9.1.f LPH"); resumen de la última junta ordinaria con **quorum**; contrato de encargo profesional del despacho (firmado, colegiado); saldo de cuenta corriente comunitaria; incidencia urgente en curso; y, en la zona de navegación del despacho, tres bloques que **no aparecen en ningún hito de `PRD_go.md`**: "Todas mis comunidades", "Tareas y despachos", "Remesas y liquidaciones" y "Reclamaciones monitorias".

### Acciones

- Código de invitación: introducir código → Continuar; enlace "Contactar con la administración".
- Crear cuenta: rellenar formulario → Crear mi cuenta; enlace a Iniciar sesión.
- Código de verificación: introducir TOTP → Verificar; Usar un código de recuperación; Validar clave de emergencia; Volver a 2FA.
- Comunidad, resumen: Emitir aviso a propietarios; ver acta firmada digitalmente (enlace a Acta); navegar a Panel de control del administrador, Todas mis comunidades, Tareas y despachos, Remesas y liquidaciones, Reclamaciones monitorias.

### Backend implicado

- `POST /v1/invitations/validate` (provisional) — valida el código antes de mostrar el alta; body `{code}`.
- `POST /v1/invitations/{code}/accept` (provisional) — crea el `user` y el `unit_member` correspondiente; body `{name, password, phone, notify_channel}`.
- `POST /v1/auth/mfa/verify` (provisional) — ya hay tabla (`user_mfa`, ver más abajo); falta el endpoint.
- `POST /v1/auth/mfa/recovery` (provisional) — consume uno de los `recovery_codes_hashed` ya existentes en `user_mfa`.
- `GET /v1/communities/{id}` (provisional) — ficha resumen; agrega datos de varios módulos (incidencias, avisos, documentos, juntas, recibos) que a su vez son endpoints propios de M2/M3/M5/M7.
- `GET /v1/communities/{id}/reserve-fund` (provisional) — saldo y porcentaje frente al mínimo legal.
- `GET /v1/offices/{id}/communities` (provisional) — listado "Todas mis comunidades" del despacho.
- Sin endpoint identificable — **no mapeado**: "Tareas y despachos", "Remesas y liquidaciones", "Reclamaciones monitorias" (ver sección de hallazgos al final).

### Tablas y conceptos de dominio

- `invitations (community_id, unit_id, code_hash, expires_at, used_at, failed_attempts)` (provisional) — la puerta de seguridad M1 exige caducidad, un solo uso y bloqueo a los 10 fallos; esta tabla es la pieza que lo soporta.
- `users`, tabla **ya existente** (`api/migrations/schema/00001_auth_schema.sql`); el alta solo añade filas.
- `unit_members (unit_id, user_id, role)` (provisional) — vínculo vecino-vivienda con rol (`owner`/`tenant`).
- **`user_mfa` ya existe** en la base (`user_id, totp_secret_encrypted, recovery_codes_hashed, ...`). La pantalla de código de verificación no necesita tabla nueva, solo el endpoint de verificación sobre esa tabla.
- `communities (name, cif, office_id, reserve_fund_target, reserve_fund_balance)` (provisional).
- `offices (name, cif)` (provisional) — despacho de administración de fincas.
- Conceptos sin tabla propuesta por falta de encaje en el PRD: remesas/liquidaciones, reclamaciones monitorias, "tareas y despachos" del panel de administrador.

### Elementos legalmente relevantes

- El fondo de reserva por debajo del 10 % se marca explícitamente como incumplimiento del **art. 9.1.f LPH** — es un aviso legal, no solo informativo; cualquier cálculo de este porcentaje debe leer el umbral de `legal_rules`, no una constante.
- La política de longitud de contraseña de "Crear cuenta" (mínimo 12) coincide con el caso "sin 2FA todavía" de §5.1; conviene comprobar en implementación que se aplican los 15 caracteres cuando el usuario aún no tiene TOTP activo, no 12 fijos.

---

## M2 — Incidencias completas con fotos, asignación y notificaciones

### Pantallas

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Asignar incidencia | `91e3c243c5fd46eba4a545c34838e78c` | El administrador asigna una incidencia ya reportada a una empresa mantenedora, fija prioridad y presupuesto máximo autorizado. |
| Bandeja de incidencias | `f64bd30bbb57401dad7ae6598beb60ec` | Panel de incidencias del despacho: multi-comunidad, filtros por estado, asignar proveedor, reiterar aviso, validar y cerrar, registrar incidencia nueva. |
| Bandeja | `ff58a5a74c9b4cdb96caad3ece760982` | Bandeja de encargos vista por la empresa proveedora: aceptar/rechazar incidencia asignada, aceptar inspección. |
| Bandeja (Modo oscuro) | `623e92b9e2fe4158b59c0ee06194d7f1` | Mismo contenido y acciones que "Bandeja", variante de tema oscuro — **no es una pantalla funcional distinta**. |
| Bandeja vacía | `65fe5c18b80c4e2d8a46c97f595d9975` | Estado vacío de la bandeja de la empresa, más un bloque de verificación/homologación del proveedor (seguro de RC, corriente de pagos, zonas de actuación). |

### Datos que muestran

- Incidencia: número de expediente (`INC-2026-089`), título, descripción, prioridad (Baja/Normal/Urgente), quién la reportó y cuándo, empresa mantenedora candidata con su estado de homologación (seguro RC vigente, TGSS, contrato marco activo), presupuesto máximo autorizado, nota interna solo visible para la empresa, canal de notificación (SMS/push con acuse).
- Bandeja de incidencias (despacho): comunidades asignadas al colegiado, contadores por estado (sin asignar/urgentes/en curso/por validar), por incidencia: ubicación, antigüedad, prioridad, empresa asignada, último evento ("Notificado a...", "Material recibido... cita hoy", "Parte de trabajo aportado").
- Bandeja (empresa): contadores (pendientes, en curso, presupuestos), tiempo medio de respuesta, cada encargo con ubicación, comunidad, límite de presupuesto autorizado, fotografías adjuntas, contrato de mantenimiento vigente.
- Bandeja vacía: estado de homologación del proveedor — seguro de responsabilidad civil con fecha de validez, "corriente de pagos", zonas de actuación preferente (distritos asignados).

### Acciones

- Asignar incidencia: elegir prioridad; elegir empresa mantenedora; fijar presupuesto máximo; escribir nota interna; marcar urgente (notifica al presidente); Asignar / Cancelar.
- Bandeja de incidencias: filtrar (todas/sin asignar/urgentes/en curso/por validar); buscar; cambiar de comunidad; Ver (aviso crítico); Asignar; Reiterar (reenvía el aviso a la empresa); Ver orden; Validar y cerrar; Registrar nueva incidencia (abre modal: elegir gremio/proveedor homologado, instrucciones/SLA) → Confirmar aviso / Cancelar; Restablecer todos los filtros.
- Bandeja (empresa): Aceptar encargo / Rechazar; Aceptar inspección; Detalle.
- Bandeja vacía: Completar mi perfil.

### Backend implicado

- `GET /v1/incidents/{id}` / `PATCH /v1/incidents/{id}` (provisional).
- `PATCH /v1/incidents/{id}/assign` (provisional) — body `{company_id, priority, max_amount_authorized, internal_note, notify_urgent}`.
- `GET /v1/communities/{id}/incidents?status=&priority=` (provisional) — usada tanto por la bandeja del despacho como, filtrada por comunidad, por el resumen de comunidad de M1.
- `GET /v1/offices/{id}/incidents?status=&community_id=` (provisional) — vista multi-comunidad del despacho; es distinta de la anterior porque agrega varias comunidades.
- `POST /v1/incidents` (provisional) — registrar incidencia nueva desde el despacho.
- `POST /v1/incidents/{id}/reiterate` (provisional) — reenvía notificación a la empresa asignada.
- `PATCH /v1/incidents/{id}/validate-close` (provisional) — cierre tras validar el parte de trabajo.
- `POST /v1/incidents/{id}/accept` / `POST /v1/incidents/{id}/reject` (provisional) — respuesta de la empresa al encargo.
- `GET /v1/companies/{id}` (provisional) — perfil de homologación del proveedor (seguro RC, corriente de pagos, zonas de actuación); usada por "Bandeja vacía" y por el selector de empresa en "Asignar incidencia".
- `GET /v1/offices/{id}/companies` o `GET /v1/communities/{id}/companies` (provisional, ambigüedad marcada abajo) — listado de proveedores homologados para el selector de "Asignar incidencia" / "Registrar nueva incidencia".

### Tablas y conceptos de dominio

- `incidents (community_id, status, priority, reported_by, assigned_company_id, max_amount_authorized, internal_note, urgent, created_at)` — status incluye al menos: sin_asignar/urgente/en_curso/por_validar/resuelta/cerrada.
- `incident_photos (incident_id, file_key)`.
- `work_orders` o `parte_trabajo (incident_id, submitted_by, submitted_at, content)` (provisional) — el "parte de trabajo aportado" que hay que "validar y cerrar".
- `companies (name, tax_id, insurance_valid_until, payment_status, office_id_or_community_scope)` — proveedor/empresa de servicios.
- `company_service_areas (company_id, district)` (provisional) — zonas de actuación preferente.
- `offices`, `communities` (ya referenciadas en M1).

### Ambigüedad señalada

- No queda claro en el diseño si el directorio de empresas homologadas es **por despacho** (un despacho gestiona su propia cartera de proveedores para todas sus comunidades) o **por comunidad** (cada comunidad tiene su propia lista). La pantalla "Directorio de empresas" (fuera de este bloque, filas 21-79) probablemente lo resuelve; aquí solo se deja constancia de que "Bandeja de incidencias" sugiere alcance de despacho y "Asignar incidencia" no lo dice explícitamente.
- La pertenencia de este flujo de empresa (Bandeja/Bandeja vacía) a M2 frente a M8 no es binaria: M2 dice explícitamente "vecino → admin → empresa → cierre en móvil", así que el ciclo de vida de la incidencia con la empresa es M2; en cambio Tareas/Fichaje/Facturas (que aparecen en la navegación inferior de estas mismas pantallas) son M8. Es decir, estas pantallas comparten navegación con M8 pero su contenido funcional es M2.

---

## M3 — Avisos, documentos, directorio

### Pantallas

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Avisos | `20823a4096754d44b6e9e6996fb399ae` | Listado de avisos/comunicados de la comunidad, con fijados, buscador y filtro. |
| Carpeta Actas | `3ab5dbbd890d497bb29269a1e3813213` | Carpeta de actas de junta filtrable por año, con descarga y validez registral. |

### Datos que muestran

- Avisos: tipo de aviso (urgente, convocatoria, informativo...), si está fijado por la administración, autor, fecha, resumen, contador de no leídos.
- Carpeta Actas: 12 documentos listados, versión (`v1`/`v2`), fecha, tamaño, estado (disponible / "pendiente de escaneo" para el histórico de 1998), sello de "validez registral garantizada" con mención a diligencia de cierre de secretaría y firma mancomunada.

### Acciones

- Avisos: buscar; abrir filtros (tune); fijar/desfijar (keep); abrir detalle de un aviso.
- Carpeta Actas: cambiar de año (Todos/2026/2025/2024/Histórico); descargar un acta; abrir menú (more_vert).

### Backend implicado

- `GET /v1/communities/{id}/announcements?pinned=&unread=` (provisional).
- `GET /v1/announcements/{id}` (provisional) — pantalla "Detalle de aviso" está fuera de este bloque, pero esta lista la referencia.
- `GET /v1/communities/{id}/minutes?year=` (provisional) — reutiliza el mismo recurso que "Acta" y "Convocatoria junta ordinaria" en M7; ver nota de solape abajo.
- Descarga: URL prefirmada de 5 minutos, igual que exige la puerta de seguridad M2 para ficheros en general.

### Tablas y conceptos de dominio

- `announcements (community_id, type, pinned, published_at, author_id, body)`.
- `announcement_reads (announcement_id, user_id, read_at)` (provisional) — soporta el contador "no leídos".
- `minutes (community_id, meeting_id, year, version, file_key, size, status)` — `status` incluye al menos `available` y `pending_scan` (el acta histórica de 1998 sin digitalizar).

### Solape con M7

- Las actas ("Carpeta Actas" aquí, "Acta" en M7) son un único recurso (`minutes`) visto desde dos ángulos: listado documental (M3) y detalle de firma/sello de tiempo (M7). Se cuenta el endpoint de listado una sola vez en M3 y el de detalle en M7 para no duplicar.

---

## M5 — Recibos y saldo, morosidad

### Pantalla

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Cambiar IBAN | `946660366172470b9362d15a944afc66` | Cambiar la cuenta de domiciliación de recibos, con reautenticación. |

### Datos que muestra

IBAN actual enmascarado (`ES91 •••• •••• •••• 4821`), estado (Activa), y campo para el nuevo IBAN. Aviso de que el administrador debe validarlo antes de usarlo y de que las cuotas seguirán cargándose en la cuenta actual hasta la aprobación.

### Acciones

Reautenticación con contraseña + código TOTP de 6 dígitos; introducir nuevo IBAN; Guardar IBAN.

### Backend implicado

- `POST /v1/me/reauth` (provisional) — verifica contraseña + TOTP antes de permitir el cambio; coincide con la puerta de seguridad M5 ("Cambio de IBAN exige reautenticación").
- `PATCH /v1/units/{id}/iban` o `PATCH /v1/me/iban` (provisional, según si el IBAN se guarda por vivienda o por titular) — crea una solicitud de cambio pendiente de validación del administrador, no un cambio inmediato.
- `POST /v1/units/{id}/iban/approve` (provisional) — acción del lado administrador que no aparece en esta pantalla pero que el texto ("Tu administrador lo validará") exige que exista.

### Tablas y conceptos de dominio

- `units` o `unit_members` con `iban_encrypted` y `iban_key_version` (cifrado versionado, tal como exige la puerta de seguridad M5).
- `iban_change_requests (unit_id, requested_iban_encrypted, requested_by, status, approved_by, approved_at)` (provisional) — la pantalla dice explícitamente que el cambio no es inmediato, así que hace falta un estado intermedio.

### Elemento legalmente/estructuralmente relevante

- El cambio de IBAN no sustituye la cuenta activa hasta la validación del administrador: esto es una regla de negocio explícita en el texto de la pantalla, no una suposición. Cualquier implementación que aplique el cambio de forma inmediata contradice el propio diseño.

---

## M6 — Reservas de zonas comunes

### Pantallas

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Calendario pista de pádel | `8e6aa21510c548efa48c0df915e69708` | Calendario semanal con franjas horarias de una zona común para reservar. |
| Confirmar reserva | `91105543769d488fbcd670c60143ef8b` | Confirmación de una reserva concreta con normas de uso y localizador. |

### Datos que muestran

- Calendario: normativa (máx. 1 reserva diaria por vivienda, Art. 6 RRI — reglamento de régimen interno, no LPH), selector de día (semana visible), estado de cada franja de 60 min (disponible / ocupada / en mantenimiento / seleccionada).
- Confirmar reserva: estado "corriente de pago" de la vivienda solicitante, instalación, fecha y franja, vivienda y titular, coste (gratuito, incluido en cuota comunitaria), normas de uso (máx. 2 reservas/mes, cancelación gratuita hasta 24h antes, límites de cesión a terceros), localizador de confirmación (`PAD-2025-0913`).

### Acciones

- Calendario: elegir día; elegir franja; Reservar.
- Confirmar reserva: Confirmar reserva / Modificar horario; tras confirmar, Entendido.

### Backend implicado

- `GET /v1/common-areas/{id}/availability?date=` (provisional) — franjas y su estado.
- `POST /v1/common-areas/{id}/bookings` (provisional) — body `{unit_id, start_time, end_time}`; debe rechazar solapes (constraint de exclusión, según la puerta de seguridad M6) y devolver el localizador.

### Tablas y conceptos de dominio

- `common_areas (community_id, name, type, rules_ref)`.
- `bookings (community_id, common_area_id, unit_id, start_time, end_time, status, locator_code, created_by)` — necesita el constraint de exclusión por solapamiento de franja que exige la puerta de seguridad M6, probado bajo concurrencia (50 reservas simultáneas → exactamente una confirmada).

### Ambigüedad señalada

- "Confirmar reserva" muestra el estado **"corriente de pago"** de la vivienda como condición aparente para reservar, pero el diseño no dice explícitamente qué pasa si la vivienda es morosa en esta pantalla (existe una pantalla separada "Reserva bloqueada por deuda" fuera de este bloque que sugiere que sí bloquea). A diferencia del voto, donde el art. 15.2 LPH es explícito, **no hay base legal citada aquí** para bloquear reservas a deudores: es una decisión de producto, no un mandato legal, y debería documentarse como tal si se implementa.

---

## M7 — Juntas y voto online

### Pantallas

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Convocatoria junta ordinaria | `7ecf24f0044b447fb019ffbf222d4d9a` | Convocatoria oficial con orden del día, modalidad, quién no puede votar, y acciones de asistencia/delegación/propuesta. |
| Acta | `4acf5c66ebd54f118806ddc6d3ebb860` | Detalle de un acta ya firmada, con sellado de tiempo y envío certificado. |
| Delegación activa | `556b5e8219b647f490dfd4bb1f4025b1` | Gestión de una delegación de voto ya formalizada: ver, revocar. |

### Datos que muestran

- Convocatoria: quién convoca (rol de presidencia), modalidad (híbrida: sala + videoconferencia), 1.ª y 2.ª convocatoria con fecha/hora, propietarios **sin derecho a voto** por impago (con su vivienda y coeficiente de participación, y la referencia legal exacta **art. 15.2 LPH**), orden del día con 4 puntos y, por punto, el **tipo de mayoría exigido** (mayoría simple art. 17.7, unanimidad art. 17.6, un tercio del total art. 17.1), documentación adjunta firmada.
- Acta: estado de formalización (diligenciada), firmas de presidenta y secretario con fecha, envío certificado por correo con fecha (de la que "cuentan los plazos de impugnación, art. 18 LPH"), copia digitalizada con folios, resumen de acuerdos adoptados, verificación de sello de tiempo (FNMT, hash, fecha UTC), anexo de subsanación posterior.
- Delegación activa: representante, coeficiente propio + delegado, fecha y hora de firma, **Código Seguro de Verificación (CSV)**, documento de delegación firmado descargable, base legal de revocación (**art. 15.1 LPH**: la asistencia presencial o telemática anula automáticamente la delegación).

### Acciones

- Convocatoria: Confirmar asistencia; Delegar mi voto; Proponer un punto para la siguiente junta; descargar documentación adjunta; expandir el detalle de un propietario sin voto.
- Acta: Descargar; Verificar sello de tiempo.
- Delegación activa: Revocar delegación → modal Confirmar y revocar / Mantener delegación.

### Backend implicado

- `GET /v1/meetings/{id}` (provisional) — convocatoria, modalidad, agenda con tipo de mayoría por punto, adjuntos, lista de propietarios sin voto por impago.
- `POST /v1/meetings/{id}/attendance` (provisional) — confirmar asistencia.
- `POST /v1/meetings/{id}/delegations` (provisional) — delegar voto.
- `DELETE /v1/meetings/{id}/delegations/{id}` (provisional) — revocar delegación; body/lógica debe comprobar que no haya asistencia registrada (art. 15.1).
- `GET /v1/meetings/{id}/delegations/me` (provisional) — ver mi delegación activa (pantalla "Delegación activa").
- `POST /v1/meetings/{id}/agenda-requests` (provisional) — proponer un punto.
- `GET /v1/communities/{id}/minutes/{id}` (provisional) — detalle de acta firmada; distinto del listado de M3.
- `GET /v1/minutes/{id}/timestamp-verify` (provisional) — verificación del sello RFC 3161 mostrado en pantalla ("Verificar sello de tiempo").

### Tablas y conceptos de dominio

- `meetings (community_id, modality, first_call_at, second_call_at, called_by, status)`.
- `meeting_items` / agenda (`meeting_id, order, title, majority_type_ref)` — `majority_type_ref` debe apuntar a `legal_rules`, no ser un valor fijo, porque el propio `PRD_go.md` exige que ninguna mayoría se escriba como constante.
- `meeting_attachments` o reutilización de `documents (meeting_id, ...)`.
- `vote_delegations (meeting_id, delegator_unit_id, delegate_unit_id, signed_at, csv_code, document_key, revoked_at, revoked_reason)`.
- `agenda_requests (community_id, proposed_by, title, target_meeting_id, status)`.
- `minutes` (ya referenciada en M3) + `signature_evidence (minutes_id, signer_user_id, signed_at, channel, otp_challenge_id)` — el canal de firma (email/TOTP) es exactamente lo que exige la puerta de seguridad M7.
- **`otp_challenges` ya existe** en la base con `purpose IN ('phone_verify', 'vote', 'sign_minutes', 'sensitive_action')` — el propósito `sign_minutes` y `vote` ya están previstos en el esquema M0, aunque no haya endpoints ni lógica de negocio todavía. Es un hallazgo relevante: parte de la infraestructura de OTP de este hito ya está en el esquema base.
- `debtor` (concepto, no tabla nueva) — la exclusión de voto por impago (art. 15.2 LPH) necesita poder resolver, para cada `unit`, si está al corriente de pago en la fecha de la convocatoria; probablemente una vista o cálculo sobre `receipts` (M5), no una tabla propia.

### Elementos legalmente relevantes (los más cargados de este bloque)

- **Art. 15.2 LPH** citado explícitamente para excluir del voto a propietarios morosos, mostrando su coeficiente excluido.
- **Art. 17.1 / 17.6 / 17.7 LPH** citados para fijar el tipo de mayoría de cada punto del orden del día — confirma que `legal_rules` debe modelar mayorías **por tipo de acuerdo**, no una sola regla general.
- **Art. 18 LPH**: el envío certificado del acta es el hecho que abre el plazo de impugnación; la fecha de ese envío es un dato con valor probatorio, no solo informativo.
- **Art. 15.1 LPH**: la delegación se anula automáticamente si el titular asiste (presencial o telemática) — esto es una regla de negocio con mandato legal directo, debe aplicarse en el momento de registrar la asistencia, no dejarse a que el usuario revoque manualmente.
- Sello de tiempo (RFC 3161 vía FNMT) y CSV de delegación son evidencia probatoria append-only — coherente con `signature_evidence` inmutable que ya exige la puerta de seguridad M7.

---

## M8 — Empresa: tareas, registro de facturas, fichaje

### Pantalla

| Pantalla | Screen ID | Qué hace |
|---|---|---|
| Corregir fichaje | `3bc1d8b612a3446d872696efc5d5d15a` | Un trabajador o su empresa corrige un fichaje ya registrado, dejando constancia del motivo. |

### Datos que muestra

Identidad del trabajador (nombre, ID, referencia a Art. 34.9 ET), fichaje original (entrada, salida registrada, cómputo inicial), nuevo cómputo tras la corrección, motivo justificado (obligatorio), y una nota legal explícita: "conforme al Real Decreto-ley 8/2019 e Inspección de Trabajo, cualquier modificación genera un registro inmutable con usuario emisor, timestamp cronológico y motivo alegado".

### Acciones

Introducir hora de salida corregida; escribir motivo justificado; Guardar corrección / Cancelar sin modificar; confirmación final Finalizar.

### Backend implicado

- `POST /v1/time-entries/{id}/corrections` (provisional) — body `{new_clock_out, reason}`; **no debe ser un `UPDATE`** sobre el registro original.

### Tablas y conceptos de dominio

- `time_entries (worker_id, company_id, clock_in, clock_out, corrected_from_id, correction_reason, corrected_by, corrected_at)` — el propio texto de la pantalla exige que sea append-only (nueva fila que referencia a la original), exactamente lo que pide la puerta de seguridad M8 ("corrección solo por nuevo registro con motivo").

### Elemento legalmente relevante

- Cita explícita del **Real Decreto-ley 8/2019** (registro de jornada) como base de la obligación de trazabilidad; esta pantalla es la evidencia de diseño más directa de que `time_entries` debe ser append-only a nivel de rol de base de datos, no solo de convención de aplicación.

---

## Hallazgos que no encajan en ningún hito M0-M8

Se listan aparte, tal como pide el encargo, en vez de forzarlos dentro de un hito:

1. **"Tareas y despachos"** y **"Remesas y liquidaciones"** (navegación del panel de administrador en "Comunidad, resumen"): no hay hito en `PRD_go.md` que hable de gestión de tareas internas del despacho ni de remesas/liquidaciones de cobros. Podría ser una funcionalidad de despacho multi-comunidad no contemplada aún en el plan de entregas, o simplemente un elemento de navegación reservado para una fase posterior sin definir.
2. **"Reclamaciones monitorias"** (mismo panel): gestión de reclamación judicial de deuda a morosos. Tampoco aparece en el plan de entregas; es un concepto legal (procedimiento monitorio) más allá de "certificado de deuda" que sí está en M5.
3. **Homologación de proveedores** (seguro de responsabilidad civil, estar al corriente de pagos, zonas de actuación): aparece en "Bandeja vacía" como requisito para que una empresa reciba encargos, pero ningún hito dice explícitamente quién gestiona esa homologación ni si es un requisito de alta (M1, cuando se crea la cuenta de empresa) o de M2 (validación antes de asignar). Se deja documentado en M2 por proximidad funcional, pero la ambigüedad es real.

## Pantallas cuyo propósito no se pudo determinar

Ninguna de las 19 pantallas de este bloque quedó sin propósito identificable. Las únicas incertidumbres son las de negocio ya señaladas en cada sección (alcance del directorio de proveedores, bloqueo de reservas a morosos, y los tres conceptos de despacho sin hito).

## Nota sobre duplicados

"Bandeja" (`ff58a5a74c9b4cdb96caad3ece760982`) y "Bandeja (Modo oscuro)" (`623e92b9e2fe4158b59c0ee06194d7f1`) son la misma pantalla en dos temas visuales; se cuentan como una sola funcionalidad en este inventario y en cualquier recuento total de pantallas útil para estimar coste.
