# Inventario funcional — App móvil, bloque 2 de 4

Filas 21 a 40 del índice de pantallas de `docs/design/stitch-screens.md`, orden
alfabético, de **"Delegar mi voto"** a **"Informe mensual"** inclusive.
Proyecto Stitch `11075381530582947267` ("Vecingest APP"), dispositivo `MOBILE`.

20 pantallas analizadas de 20. Método: descarga del HTML de cada pantalla
(`htmlCode.downloadUrl` de `get_screen`) y extracción programática de botones,
enlaces, campos de formulario, encabezados y texto visible; no se ha pegado
HTML completo en ningún momento del análisis.

Fuentes cruzadas: `PRD_go.md` (§5 requisitos funcionales, §7.3 modelo de
datos, §7.4 API, §10 plan de entregas con la tabla de hitos hacia la línea
1493), `api/openapi/openapi.yaml` (12 endpoints reales: auth, health, `/me`)
y `api/migrations/schema/` (tablas reales: `users`, `sessions`, `audit_log`,
tokens de reset, MFA de usuario, más las propias de River). Ninguna otra
tabla ni endpoint existe hoy en el backend: todo lo que sigue es diseño sin
implementar salvo que se diga lo contrario.

La numeración de endpoints y tablas usa la convención de `PRD_go.md` §7.4
adaptada al estilo OpenAPI real del repo (`{id}` en vez de `:id`, que es como
aparece en `api/openapi/openapi.yaml`). Donde el PRD no define un endpoint
pero la pantalla lo requiere, se propone uno por analogía con los que sí
existen y se marca **(provisional)**.

---

## M1 — Comunidades, viviendas, invitaciones, portales por rol

### Elegir dónde entrar
- **Propósito**: selector de contexto de trabajo cuando el usuario tiene más
  de una membresía (propietario de una vivienda, inquilino de otra, y
  trabajador de una empresa, en el ejemplo mostrado).
- **Datos mostrados**: lista de contextos con dirección o razón social, rol
  en cada uno (`propietario` / `inquilino` / `trabajador`) y cuál está
  seleccionado actualmente. Checkbox "Preguntarme siempre al iniciar sesión".
- **Acciones**: seleccionar un contexto, pulsar "Acceder", "Añadir otra
  comunidad o empresa con código" (reutiliza el flujo de invitación/código
  corto ya cubierto en otro bloque del inventario), volver.
- **Backend implicado**:
  - `GET /v1/me` — ya listado en el PRD como fuente de "usuario + membresías"; de ahí sale la lista de esta pantalla.
  - No hay endpoint de "fijar contexto activo" en el PRD: las rutas ya van
    ámbito por `community_id`/`office_id`/`company_id` en la URL, así que la
    selección puede resolverse en cliente sin llamada nueva. Lo único sin
    modelar es la preferencia "preguntarme siempre": no hay columna para ella
    en `users` (§7.3). Si se quiere persistir, sería un campo nuevo en
    `users.notification_prefs` o una columna dedicada — **(provisional, no
    especificado en el PRD)**.
  - Tablas: `unit_members`, `company_members`, `office_members`.
- **Hito**: M1 ("portales vacíos por rol") por eliminación — no hay una fila
  de hito que mencione esta pantalla explícitamente; es la pieza de
  navegación que hace falta en cuanto existe más de una membresía, que es
  justamente lo que M1 introduce.

---

## M2 — Incidencias completas con fotos, asignación y notificaciones

El criterio funcional de M2 en la tabla de hitos es literalmente "Flujo
vecino → admin → empresa → cierre en móvil", así que las pantallas de
"Encargo" (vista de la empresa sobre una incidencia que se le ha asignado)
se agrupan aquí y no en el bloque de empresa (M8): son la misma máquina de
estados de `incidents` (§5.3), vista desde el otro lado.

### Pantallas
| Pantalla | Screen ID |
|---|---|
| Detalle incidencia: Asignada | `dee3aae04f8b434d8fbfc28da43c9860` |
| Detalle incidencia: Cerrada y valorar | `0ea87c3fb03442cbba67be75ba1bbddc` |
| Detalle incidencia: Resuelta | `43199a8bafb245d4835761c42756f93e` |
| Incidencias | `5d4caeb3444645a5b47ea1333389681d` |
| Incidencias (vacío) | `dc67ad7c54784d40b43f3f6100e11365` |
| Encargo en curso | `49e333743076441bb9fec5c99908595b` |
| Encargo, detalle | `b2b00fe8566245c4826edbc6bc1b4c43` |

### Datos mostrados
- Lista de incidencias: título, tiempo relativo ("hace 40 min", "ayer"),
  ubicación, estado (`Abierta`/`Asignada`/`En curso`/`Resuelta`/`Cerrada`),
  filtros "Mías" y "Zonas comunes".
- Detalle de incidencia: estado, empresa asignada, título, descripción,
  contador de vecinos afectados, referencia/ID, cronología de eventos
  (creada → asignada → aceptada → resuelta → cerrada) con actor y hora,
  hilo de comentarios con autor y timestamp, fotografías adjuntas.
- "Cerrada y valorar": valoración 1–5 estrellas + comentario opcional,
  parte de intervención en PDF firmado descargable, historial completo.
- "Resuelta": cuenta atrás implícita al cierre automático ("se cerrará sola
  en 7 días si no hay objeciones"), acción de confirmar cierre o de marcar
  "no está resuelta" con motivo obligatorio (reapertura).
- "Encargo en curso" (vista empresa): ID de encargo, estado, prioridad
  contractual con plazo de urgencia ("Alta · Urgencia 4h"), campo de parte
  de trabajo obligatorio al cerrar, toggle "Visible para el vecino", fotos
  de evidencia, cronología de actividad, referencia a la póliza de la
  comunidad.
- "Encargo, detalle" (vista empresa, antes de aceptar): fotos periciales,
  ubicación de la finca, contacto del administrador, presupuesto máximo
  autorizado, nota interna del administrador ("solo la ves tú").

### Acciones
- Crear incidencia, filtrar por "Mías"/"Zonas comunes", buscar.
- Enviar comentario en el hilo de seguimiento.
- Enviar valoración (1–5 + comentario) al cerrar.
- Confirmar cierre / marcar "no está resuelta" con motivo (reabrir).
- Aceptar o rechazar el encargo (empresa).
- Añadir fotos, redactar parte de trabajo, marcar como resuelta (empresa).

### Backend implicado (consolidado, ya definido en el PRD salvo que se marque)
- `GET /v1/communities/{id}/incidents` — filtros `status`, `category`,
  `unit`; "Mías" es `created_by={me}`, "Zonas comunes" es `scope=common`.
- `POST /v1/communities/{id}/incidents`
- `GET /v1/incidents/{id}`, `PATCH /v1/incidents/{id}`
- `POST /v1/incidents/{id}/transition` (`{ to: status, note? }`) — cubre
  confirmar cierre (`resolved → closed`), reabrir (`resolved → in_progress`,
  motivo obligatorio, máximo 2 veces) y marcar resuelta (`in_progress →
  resolved`).
- `GET /v1/incidents/{id}/comments`, `POST /v1/incidents/{id}/comments`
  (`{ body, is_internal }`)
- `POST /v1/incidents/{id}/attachments` + `POST
  /v1/incidents/{id}/attachments/{attId}/confirm` — fotos y parte de
  intervención en PDF.
- `GET /v1/incidents/{id}/events` — cronología.
- `POST /v1/incidents/{id}/follow` — "sumarse" a una incidencia existente.
- `GET /v1/companies/me/incidents` — bandeja de encargos de la empresa.
- `POST /v1/incidents/{id}/accept`, `POST /v1/incidents/{id}/decline`
  (motivo obligatorio) — aceptar/rechazar encargo.
- `POST /v1/companies/{id}/ratings` (`{ score, comment }`) — valorar al
  cerrar.

Tablas: `incidents`, `incident_comments`, `incident_attachments`,
`incident_events`, `incident_followers`, `ratings`.

### Requisitos legalmente relevantes
- Ninguno de art. 17/Art. 19.4 (eso es junta/voto, bloque M7). Lo único con
  base legal explícita en las pantallas es la mención "Art. 10.1 LPH" en la
  categoría de obras, que el PRD ya recoge como categoría de incidencia
  ("obras obligatorias art. 10 LPH", §5.3).

### Observaciones / brechas de diseño
- "Encargo en curso" muestra "Presupuesto máximo autorizado 600,00 € (IVA no
  incluido)", que coincide con `incidents.max_budget`. Pero también muestra
  un plazo de urgencia contractual ("Urgencia 4h") que **no existe** como
  columna en `incidents` ni en ninguna tabla del §7.3: es un dato de diseño
  sin modelo de datos que lo respalde todavía.
- El toggle "Visible para el vecino" del parte de trabajo corresponde a
  `incident_comments.is_internal` invertido. Pero el PRD también define
  `tasks`/`task_reports` como entidad separada (con `tasks.incident_id`
  opcional) para partes de trabajo de empresa. La pantalla vive dentro del
  detalle de la incidencia, no de una pantalla de "tarea" independiente, así
  que lo más fiel al diseño es mapear el parte a `incident_comments`; pero
  es una ambigüedad real entre dos tablas candidatas del propio PRD, y debe
  resolverse antes de implementar.

---

## M3 — Avisos, documentos, directorio

### Pantallas
| Pantalla | Screen ID |
|---|---|
| Detalle de aviso | `66d20dbae241462ea83010b9bdab1d6d` |
| Documentos | `50f9cf9be68c434cbef834f30cb8df39` |
| Directorio de empresas | `ca158c32343e46b2ac30d6d652be2fc5` |

### Datos mostrados
- Detalle de aviso: título, categoría ("Aviso urgente"), autor
  (despacho), fecha y hora, cuerpo del texto, un adjunto en PDF con su
  tamaño, estado de lectura ("Leído el 12 sep, 10:14"), referencia
  (`AV-2026-084`).
- Documentos: carpetas de la comunidad con contador (Actas 12, Estatutos 1,
  Presupuestos 4, Seguros 2, Contratos 3, Otros 5) y una descarga compuesta
  "Expediente completo 2024" en ZIP.
- Directorio de empresas: filtros por provincia y gremio (Fontanería,
  Ascensores, Cerrajería, Electricidad), ficha de empresa con nombre,
  gremio + provincia, insignia "Verificada", valoración media y número de
  reseñas, texto destacado por empresa (contrato de mantenimiento vigente,
  urgencias 24 h, presupuesto sin compromiso).

### Acciones
- Descargar adjunto del aviso, contactar con administración (llamada).
- Abrir carpeta de documentos, descargar expediente completo en ZIP.
- Buscar/filtrar empresas por gremio o provincia, pedir presupuesto,
  "Sugerir empresa".

### Backend implicado
- `GET /v1/announcements/{id}`, `POST /v1/announcements/{id}/read`.
- Descarga de adjunto de aviso: el PRD no lista un endpoint explícito para
  `announcement_attachments` (sí lo hace para `documents`) — **(provisional)**
  `GET /v1/announcements/{id}/attachments/{attId}/download`, mismo patrón
  de URL prefirmada que usa `GET /v1/documents/{id}/download`.
- `GET /v1/communities/{id}/documents` (agrupado por `document_folders`),
  `GET /v1/documents/{id}/download`.
- `GET /v1/companies` (filtros `trade`, `province`, `q`) — directorio.
- `POST /v1/companies/{id}/service-requests` — pedir presupuesto.
- `POST /v1/companies/{id}/ratings` — también aplica aquí, sobre
  `service_requests` en vez de `incidents`.

Tablas: `announcements`, `announcement_attachments`, `announcement_reads`,
`document_folders`, `documents`, `companies`, `service_requests`, `ratings`.

### Observaciones / brechas de diseño
- La descarga "Expediente completo 2024" (ZIP con 27 documentos) no está
  cubierta por el PRD, que solo define descarga de documento individual vía
  URL prefirmada de 15 minutos (§5.5). Endpoint **(provisional)**: `GET
  /v1/communities/{id}/documents/export?year=`. Si se implementa, hay que
  decidir cómo interactúa con el registro en `audit_log` por descarga que
  exige el PRD para actas y documentos con datos personales — una descarga
  ZIP agregada complica ese registro por documento individual.
- "Sugerir empresa" en el directorio no tiene tabla ni endpoint en el PRD.
  Podría reutilizar `service_requests` con un tipo distinto o ser una tabla
  nueva — el diseño no lo aclara; queda **sin especificar**.
- El badge "Art. 10.1 LPH" en el detalle del aviso no corresponde a ninguna
  columna de `announcements` (que no tiene `legal_basis`); es una referencia
  decorativa del diseño que probablemente debería ir en la categoría de la
  incidencia relacionada, no en el aviso.

---

## M7 — Juntas y voto online

### Delegar mi voto
- **Propósito**: formalizar la delegación de voto de una vivienda para una
  junta convocada, conforme al art. 15.1 LPH.
- **Datos mostrados**: junta y fecha ("Junta ordinaria · 24 de septiembre"),
  identidad del poderdante (vivienda, coeficiente, estado "Al corriente"),
  dos modalidades de delegación con radio buttons: "En otro propietario"
  (con selector de un comunero concreto, mostrando su vivienda, si tiene
  deudas vencidas y si está habilitado para votar) y "En otra persona"
  (nombre y apellidos + DNI/NIE, para representante no propietario), aviso
  de revocabilidad hasta el inicio de la junta.
- **Acciones**: elegir modalidad y delegado, "Firmar delegación con código
  SMS", cancelar.
- **Backend implicado**: `POST /v1/meetings/{id}/delegations`.
  Tablas: `vote_delegations` (`meeting_id`, `from_unit_id`, `from_user_id`,
  `to_user_id` nullable, `to_person_name` nullable,
  `to_person_id_document_encrypted` nullable, `document_file_key` nullable,
  `signature_evidence_id` FK nullable, `revoked_at`), `signature_evidence`,
  `meeting_voters` (para el estado de elegibilidad del delegado mostrado en
  pantalla).
- **Hito**: M7 ("Juntas y voto online").

### Requisitos legalmente relevantes
- Delegación conforme al **art. 15.1 LPH**: a cualquier persona (no solo
  propietarios), revocable hasta el inicio de la junta — la pantalla lo cita
  literalmente y coincide con el PRD §5.8.
- La delegación requiere firma electrónica avanzada (OTP), lo que implica
  un registro en `signature_evidence` con el canal usado, tal como exige el
  PRD para que la fuerza probatoria del voto/delegación sea distinguible a
  posteriori (§5.8, puerta de seguridad M7).
- **Discrepancia a corregir**: el botón dice "Firmar delegación con código
  **SMS**", pero `PRD_go.md` §11 decidió explícitamente no usar SMS para el
  OTP de voto y firma (solo email o TOTP), precisamente para eliminar el
  proveedor de SMS y su superficie de riesgo (coste, rotación de
  credenciales, *SMS pumping*). El copy de esta pantalla contradice esa
  decisión de arquitectura ya tomada; hay que corregir el texto del diseño
  o reabrir la decisión antes de construir la pantalla.
- El desplegable de delegado muestra su elegibilidad ("Sin deudas vencidas
  · Habilitada para votar"), lo que implica validar que el delegado
  (`to_user_id`) no esté a su vez privado de voto por morosidad antes de
  aceptar la delegación — esto no está explícito como regla en el PRD
  aunque se deduce de `meeting_voters.is_debtor`; conviene dejarlo escrito
  como requisito antes de implementar.

---

## M8 — Empresa: tareas, registro de facturas, fichaje

Aquí solo entran las pantallas que son exclusivas de la operación interna de
la empresa (no las de gestión de una incidencia concreta, que están en M2).

### Pantallas
| Pantalla | Screen ID |
|---|---|
| Documentación | `327487c1310c4625a2260f033df51c16` |
| Equipo | `6edbceb8550b4ced86f1879c76a63c4c` |
| Facturas | `07085aad4a32453fa6be9b9175826d70` |
| Fichaje | `822f2027033e4b7fa6ec28d16517c14c` |
| Fichaje (Modo oscuro) | `8ed16677275c46389c32e8fefcbc4385` |
| Informe mensual | `e11b8406ceb74688b79bb2b9bd4b286e` |

("Fichaje" y "Fichaje (Modo oscuro)" son la misma pantalla en claro y en
oscuro, contenido idéntico: se cuentan como una sola funcionalidad.)

### Datos mostrados
- Documentación (de la propia empresa, no de la comunidad): semáforo de
  vigencia (vigentes / por vencer / acción requerida), seguro de
  responsabilidad civil con póliza y fecha de caducidad, certificado TGSS,
  certificado AEAT, plan de prevención (caducado), RNT (pendiente de
  subir), nota sobre validez ante el Colegio de Administradores de Fincas.
- Equipo: lista de miembros con nombre, rol (Técnico / Responsable /
  Administrador), estado ("En servicio hoy"), hora de fichaje y partes
  asignados; formulario de invitación por email con selector de rol
  ("Técnico Campo" con acceso a tareas/partes/fichaje, "Responsable
  Oficina" con acceso completo a facturas/bandeja/equipo).
- Facturas: resumen (pendiente, facturado del mes, vencidas, al día),
  filtros (Todas/Pendientes/Cobradas/Rectificativas), lista con serie y
  número, comunidad, importe, estado (Enviada/Pagada/Aceptada/
  Rectificativa) y referencia al encargo.
- Fichaje: entrada/salida con cronómetro de jornada en curso, ubicación GPS
  opcional, resumen semanal por día con horas ordinarias y exceso de
  jornada señalado.
- Informe mensual: navegación por mes, cómputo individual por trabajador
  (días efectivos, horas ordinarias, extraordinarias, total, estado
  "Conforme"), exportación en PDF ("modelo normalizado para la ITSS con
  diligencia de firma por ambas partes") y en CSV con telemetría GPS.

### Acciones
- Actualizar/subir documento de la empresa.
- Invitar trabajador por email con rol.
- Registrar factura (botón `+`), filtrar por estado.
- Fichar salida, adjuntar ubicación, revisar exceso de jornada.
- Navegar entre meses, descargar informe en PDF o CSV.

### Backend implicado
- `POST /v1/companies/me/documents` — ya documentado en el PRD (§7.4).
  El `GET` equivalente para pintar el semáforo no está listado
  explícitamente — **(provisional)** `GET /v1/companies/me/documents`.
- Equipo: el PRD solo define el simétrico para despachos (`GET/POST
  /v1/offices/me/members`), no hay nada para empresas aunque sí existe la
  tabla `company_members`. **(provisional)** `GET
  /v1/companies/me/members`, `POST /v1/companies/me/members` (invitar
  trabajador).
- Facturas: el PRD define las tablas `invoices`/`invoice_lines` en §7.3
  pero no lista ningún endpoint REST para ellas en §7.4.
  **(provisional)** `GET /v1/companies/me/invoices?status=`, `POST
  /v1/companies/me/invoices` (registrar factura + subir PDF, coherente con
  "la empresa las emite con su software y aquí registra los datos y sube
  el PDF", §5.10).
- Fichaje: mismo caso, tabla `time_entries` sin endpoints REST listados.
  **(provisional)** `POST /v1/companies/me/time-entries` (fichar entrada),
  `PATCH /v1/companies/me/time-entries/{id}` (fichar salida o corregir
  dentro de las 24 h siguientes, con motivo).
- Informe mensual: `GET /v1/companies/me/time-reports?year=&month=` — este
  sí está documentado explícitamente en el PRD.

Tablas: `company_documents`, `company_members`, `invoices`,
`invoice_lines`, `time_entries`, `time_reports`.

### Requisitos legalmente relevantes
- Fichaje conforme al **art. 34.9 del Estatuto de los Trabajadores** (no es
  LPH, pero es igual de vinculante): registro horario obligatorio,
  conservación 4 años, inalterabilidad; el trabajador solo puede corregir
  un fichaje en las 24 h siguientes y con motivo justificado — coincide
  exactamente con `time_entries.edited_at/edited_by/edit_reason` y con la
  puerta de seguridad de M8 ("Fichajes y facturas append-only... corrección
  solo por nuevo registro con motivo").
- El informe mensual queda "congelado" (coherente con `time_reports.
  report_hash`) y debe conservarse a disposición de la Inspección de
  Trabajo y Seguridad Social — mismo artículo 34.9 ET.
- La plataforma **no emite** facturas, solo registra los datos que la
  empresa ya emitió con su propio software — decisión explícita del PRD
  (§5.10) para no quedar sujeta a Verifactu. La pantalla de Facturas es
  coherente con esto: no hay flujo de "emitir", solo de "registrar" y
  gestionar el estado.

### Observaciones / brechas de diseño
- El texto "Facturación electrónica sujeta a la Ley Crea y Crece y RGPD" en
  la pantalla de Facturas cita una ley que el PRD no menciona por nombre
  (el PRD solo habla de "Verifactu, factura electrónica B2B" como riesgo
  abierto en §11). No es una contradicción, pero conviene que el texto legal
  final del producto y el del PRD usen la misma referencia normativa antes
  de publicarse.
- Los roles mostrados en "Equipo" ("Técnico Campo" y "Responsable Oficina",
  con permisos distintos sobre facturas y gestión de equipo) son más
  granulares que el enum `company_members.role (company | worker)` del
  PRD, que solo tiene dos valores. Es una discrepancia de modelo real: o se
  amplía el enum, o los dos roles de la pantalla se resuelven con un flag
  adicional sobre `worker`. Debe decidirse antes de implementar permisos.

---

## Transversal, sin hito claro

### Eliminar cuenta
Dos variantes casi idénticas de la misma pantalla (mismo contenido y
acciones; probablemente una es una iteración de diseño de la otra, no dos
funcionalidades distintas). Se documentan como una sola funcionalidad.

- **Propósito**: baja definitiva de la cuenta con anonimización, no borrado
  físico, de los datos que la ley obliga a conservar.
- **Datos mostrados**: aviso legal vinculante citando el **art. 19 LPH** y
  el **art. 6.1.c RGPD** como base para anonimizar en vez de borrar (actas,
  votos y recibos se conservan de forma anónima "para garantizar la
  trazabilidad histórica de los acuerdos adoptados en junta"); lista de
  efectos inmediatos (pérdida de acceso, cese de notificaciones,
  desvinculación del coeficiente de voto y de la titularidad catastral
  registrada).
- **Acciones**: introducir contraseña actual, marcar "entiendo que no se
  puede deshacer", eliminar cuenta o mantenerla.
- **Backend implicado**: `DELETE /v1/me` — ya está listado en el PRD junto
  a los endpoints base de `/v1/me` (§7.4), fuera de cualquier fase
  numerada del plan de entregas (§10). Tablas afectadas por anonimización
  (no borrado): `users` y, de forma indirecta y sin especificar en el PRD,
  cualquier tabla con `user_id` que participe en actas/votos/recibos
  (`votes`, `receipts`, `unit_members`, etc.).
- **Hito**: no encaja limpiamente en ninguna fila de la tabla de M0–M8. El
  PRD lista el endpoint como parte del bloque base de `/v1/me` (junto con
  login y perfil), no como entregable de una fase concreta; el propio
  contenido de la pantalla (coeficiente de voto, actas) presupone que ya
  existen comunidades, juntas y recibos, es decir, funcionalidad de fases
  posteriores. Se deja fuera de la tabla de hitos y se señala aquí en vez
  de forzarlo a M0 o M1.
- **Brecha de especificación**: el PRD no detalla, más allá de nombrar el
  endpoint y la exportación RGPD (`GET /v1/me/export`), qué se anonimiza
  exactamente y qué se conserva íntegro tras la baja. Esta pantalla aporta
  contenido de producto (la lista de "efectos inmediatos") que todavía no
  está desarrollado en el PRD.

---

## Pantallas cuya finalidad no pude determinar

Ninguna. Las 20 pantallas del bloque tienen un propósito identificable a
partir de su contenido. Las únicas dudas son las señaladas arriba como
discrepancias o brechas de diseño frente al PRD, no dudas sobre qué hace
cada pantalla.

## Duplicados detectados en el bloque

- **Eliminar cuenta** (`b1201339da944b97af85835c74eba2e1` y
  `dd9498b7b9b243dc82f14e53aad006ef`): mismo contenido y acciones.
- **Fichaje** / **Fichaje (Modo oscuro)** (`822f2027033e4b7fa6ec28d16517c14c`
  y `8ed16677275c46389c32e8fefcbc4385`): mismo contenido, variante de tema.
