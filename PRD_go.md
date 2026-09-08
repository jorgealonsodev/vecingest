# PRD — Vecingest

Plataforma de gestión de comunidades de propietarios (propiedad horizontal) para vecinos, administradores de fincas y empresas de servicios.

Versión 3.1 · Documento de requisitos de producto y técnicos. Cambios mayores: backend en Go y arquitectura de escala por fases (sección 7.10).

Este documento es la fuente de verdad para el desarrollo. Cualquier asistente de código debe leerlo completo antes de generar código y respetar el glosario de la sección 12 para nombrar entidades.

**Principio de escalabilidad:** la plataforma se diseña para soportar millones de usuarios (objetivo de diseño: 5 millones de usuarios, 250.000 comunidades, 20.000 despachos, 100.000 empresas) sin reescribir código. Eso se consigue con tres reglas desde el primer commit: (1) ningún proceso guarda estado (todo estado vive en Postgres, Valkey u object storage), (2) toda tabla de negocio lleva su clave de tenant y toda consulta la usa, (3) todo lo que puede ser asíncrono lo es. La infraestructura crece por fases (7.10) según métricas, no por adelantado.

**Principio de eficiencia:** la plataforma debe ser rápida y consumir los mínimos recursos posibles. Objetivos medibles: la API responde en menos de 50 ms (p95) las peticiones que no tocan disco y en menos de 150 ms (p95) las que consultan Postgres; los procesos `api` y `worker` usan menos de 128 MB de memoria cada uno en reposo y menos de 256 MB bajo carga; el consumo real del stack completo en fase A (sin ClamAV) se mantiene por debajo de 1 GB de RAM aunque los límites de los contenedores sumen más; la imagen de la API pesa menos de 30 MB. Cada decisión técnica se justifica frente a estos números.

**Principio rector: la seguridad no es una fase, es una puerta en cada fase.** Ningún hito se da por cerrado sin superar su puerta de seguridad (sección 10.1), ningún PR se fusiona sin el punto 10 de la Definition of Done (sección 9) y ninguna funcionalidad nueva se diseña sin registrar sus amenazas en la tabla de la sección 6.1. Si hay que elegir entre entregar a tiempo y entregar seguro, se retrasa la entrega.

> **Nota legal**: la sección 4 recoge la normativa española aplicable. Los artículos de la LPH se han contrastado con el texto consolidado del BOE (última actualización publicada el 21/03/2026, consultado el 04/09/2026) y los plazos de Verifactu con el RDL 15/2025. Aun así, la LPH está en proceso de reforma (ver 4.7) y el documento no sustituye asesoramiento legal.

**Índice**: 1 Visión · 2 Alcance · 3 Roles y permisos · 4 Marco legal · 5 Requisitos funcionales · 6 Requisitos no funcionales (6.1 seguridad: amenazas, controles y acciones) · 7 Arquitectura (7.7 decisiones de implementación, 7.8 jobs, 7.9 diseño visual, 7.10 arquitectura de escala) · 8 Infraestructura · 9 Convenciones y Definition of Done · 10 Plan · 11 Riesgos · 12 Glosario

---

## 1. Visión

Plataforma que conecta a los tres actores de una comunidad de propietarios en un único sistema:

- **Vecino / propietario**: consulta lo que pasa en su comunidad, avisa de incidencias, vota en juntas, reserva zonas comunes y accede a sus recibos y documentos desde el móvil.
- **Administrador de fincas**: gestiona varias comunidades desde un panel web, comunica con los vecinos, asigna incidencias a empresas y contabiliza facturas.
- **Empresa de servicios**: recibe encargos, gestiona tareas y trabajadores, factura y registra jornada.

Objetivo del MVP: que el flujo completo de una incidencia (vecino → administrador → empresa → resolución → cierre) funcione de extremo a extremo en móvil y web. La factura de la empresa se incorpora en fase 2.

## 2. Alcance

### 2.1 Dentro del MVP (fase 1)
- Autenticación y roles.
- Gestión de comunidades, viviendas y miembros.
- Incidencias con fotos, estados, asignación y comentarios.
- Comunicaciones (tablón de anuncios) con notificaciones push y email.
- Documentos de la comunidad.
- Directorio de empresas.
- App web (Expo web) y apps móviles (Expo Android/iOS).
- Web pública estática (landing, páginas por perfil, contacto, formulario de alta de empresas, aviso legal y privacidad).

### 2.2 Fase 2 (en este orden; las juntas dependen de los recibos para la lista de morosos)
1. Recibos y saldo del propietario.
2. Reserva de zonas comunes.
3. Juntas y voto online con delegación.
4. Tareas y trabajadores para empresas.
5. Registro de facturas de empresas hacia comunidades (la plataforma no emite facturas).
6. Control horario de empresas.

### 2.3 Fase 3 (previsto, sin compromiso)
- Firma cualificada con prestador externo y sello de tiempo cualificado.
- Web push (VAPID).
- Comunicaciones certificadas con acuse.
- Emisión de facturas conforme a Verifactu (solo tras verificación normativa).

### 2.4 Fuera de alcance
- Contabilidad completa del administrador.
- Integración con software de fincas de terceros (se ofrece import/export CSV).
- Apertura remota de puertas (requiere hardware).
- Tasación de propiedades.
- Pasarela de pagos.

## 3. Roles y permisos

| Rol | Ámbito | Descripción |
|---|---|---|
| `superadmin` | Global | Operador de la plataforma. Da de alta despachos, verifica empresas, soporte. Se crea por seed, no por UI. |
| `admin` | Despacho (`office`) | Administrador de fincas. Gestiona todas las comunidades de su despacho. |
| `admin_staff` | Despacho | Empleado del despacho con permisos limitados (sin borrar comunidades, sin gestionar miembros del despacho). |
| `owner` | Vivienda | Propietario. Puede votar y ver recibos. |
| `tenant` | Vivienda | Inquilino. Como `owner` pero sin voto ni recibos. |
| `company` | Empresa | Responsable de la empresa. Gestiona tareas, trabajadores y facturas. |
| `worker` | Empresa | Trabajador. Ve sus tareas asignadas y ficha jornada. |

**Cargos de la junta directiva** (no son roles, son un atributo `board_role` sobre la membresía de vivienda, solo asignable a `owner` por el art. 13.2 LPH): `president`, `vice_president`, `secretary`. El presidente puede además ver todas las incidencias de zonas comunes, publicar avisos, proponer y convocar juntas y firmar actas. Solo el `admin` asigna cargos. Un cargo tiene fecha de inicio y fin (un año salvo estatutos, art. 13.7) y la app avisa 30 días antes del vencimiento.

Reglas:
- Un usuario puede tener varias membresías (p. ej. propietario en dos comunidades, o vecino y a la vez trabajador de una empresa). El rol se evalúa por membresía, no por usuario.
- Toda consulta a datos de una comunidad se filtra por la membresía del usuario. Nunca se confía en ids que vengan del cliente sin comprobar pertenencia.
- El `admin` de un despacho tiene acceso a todas las comunidades donde `community.office_id` coincide con alguna de sus `office_members`.
- Un despacho puede tener varios `admin`. Un usuario puede pertenecer a varios despachos (poco frecuente pero permitido).
- **Alta de despachos**: solo `superadmin`. Es una relación comercial, no un registro abierto.
- **Alta de empresas**: registro público en la web con email, CIF y datos de contacto. Quedan en `status = pending` hasta que `superadmin` las verifica (`status = verified`). Hasta entonces no aparecen en el directorio pero sí pueden recibir asignaciones de un admin que las haya añadido como proveedor habitual.
- **Cambio de despacho**: cuando una comunidad cambia de administrador, `superadmin` (o el despacho saliente) transfiere la comunidad. Todo el histórico se conserva; el despacho saliente pierde el acceso.

Matriz de permisos resumida (C = crear, R = leer, U = editar, D = borrar). `superadmin` no aparece: tiene acceso de lectura a todo para soporte y escritura solo sobre despachos, empresas y transferencias de comunidades; toda acción suya queda en `audit_log`.

| Recurso | admin | admin_staff | presidente | owner | tenant | company | worker |
|---|---|---|---|---|---|---|---|
| Comunidad | CRUD | RU | R | R | R | — | — |
| Viviendas y miembros | CRUD | CRU | R | R (la suya) | R (la suya) | — | — |
| Incidencias de zonas comunes | CRUD | CRU | CR | CR | CR | RU (asignadas) | RU (asignadas) |
| Incidencias de vivienda | CRUD | CRU | — | CR (la suya) | CR (la suya) | RU (asignadas) | RU (asignadas) |
| Avisos | CRUD | CRU | CR | R | R | — | — |
| Documentos | CRUD | CRU | R | R | R (según visibilidad) | — | — |
| Reservas | CRUD | CRU | CR (la suya) | CR (la suya) | CR (la suya) | — | — |
| Juntas y votos | CRUD | CRU | CR + votar + firmar acta | R + votar | R | — | — |
| Propuestas de orden del día | RU | RU | RU | CR (las suyas) | — | — | — |
| Solicitudes de presupuesto y valoraciones | R | R | — | CR (las suyas) | CR (las suyas) | RU (las recibidas) | — |
| Recibos | CRUD | CRU | R (los suyos) | R (los suyos) | — | — | — |
| Tareas y fichajes | — | — | — | — | — | CRUD | RU (los suyos) |
| Facturas | R | R | — | — | — | CRUD | — |

## 4. Marco legal español que condiciona el diseño

Esta sección resume la normativa que afecta a cada actor y qué exige de la plataforma. Cada punto tiene su reflejo en los requisitos funcionales y en el modelo de datos.

### 4.1 Comunidad de propietarios — Ley de Propiedad Horizontal (Ley 49/1960, LPH)

Texto consolidado del BOE, última actualización publicada el 21/03/2026 (RDL 7/2026). Cada regla indica el artículo exacto.

| Regla LPH | Qué implica para la plataforma |
|---|---|
| **Cuota de participación** (art. 3 y 5): cada piso o local tiene una cuota referida a centésimas del valor total del inmueble, fijada en el título constitutivo; solo se altera conforme a los arts. 10 y 17. | `units.participation_coefficient` obligatorio; se avisa si la suma no es 100. Solo lo edita el admin y cada cambio queda en `audit_log`. |
| **Órganos** (art. 13): junta, presidente (nombrado entre los propietarios; cargo obligatorio), vicepresidentes opcionales, secretario y administrador. Secretario y administrador los ejerce el presidente salvo que estatutos o junta los provean aparte; pueden acumularse en una persona, ser un propietario, un profesional o una persona jurídica. Mandato de un año salvo estatutos; removibles en junta extraordinaria (art. 13.7). Edificios de hasta 4 propietarios pueden acogerse al art. 398 CC (art. 13.8). | `board_role` solo asignable a miembros `owner`. Fecha de fin de mandato con recordatorio al admin. El secretario puede ser el despacho (`communities.secretary_is_office`). Si no hay presidente en vigor la app lo señala. |
| **Junta ordinaria** al menos una vez al año para aprobar presupuestos y cuentas; extraordinarias cuando lo decida el presidente o lo pidan la cuarta parte de los propietarios o el 25 % de las cuotas (art. 16.1). Cualquier propietario puede pedir que un asunto se incluya en el orden del día de la siguiente junta (art. 16.2). Junta universal válida sin convocatoria si concurren todos (art. 16.3). | Aviso al admin si una comunidad lleva más de 11 meses sin junta ordinaria. Función "proponer punto del orden del día" (`agenda_requests`) que el presidente debe incluir en la siguiente convocatoria. |
| **Convocatoria** (art. 16.2 y 16.3): la hace el presidente o, en su defecto, los promotores; indica asuntos, lugar, día y hora de primera y, en su caso, segunda convocatoria; contiene la relación de propietarios que no están al corriente de pago y advierte de la privación de voto. Citaciones conforme al art. 9. **Antelación mínima: 6 días para la ordinaria anual** (art. 16.3); para las extraordinarias, "la que sea posible". | `meetings` con `first_call_at`, `second_call_at`; validación de los 6 días para `type = ordinary`; generación automática de la relación de deudores desde `receipts` al publicar. |
| **Quórum y segunda convocatoria** (art. 16.2): primera convocatoria exige mayoría de propietarios que representen mayoría de cuotas; segunda sin quórum. La segunda puede celebrarse el mismo día pasada media hora; si no, nueva convocatoria dentro de los 8 días naturales siguientes con 3 días de antelación mínima. | `second_call_at` por defecto +30 min. Si el admin fija la segunda convocatoria otro día, validación: ≤ 8 días naturales y ≥ 3 días de antelación. `meetings.held_on_call` registra en cuál se celebró. |
| **Citación y notificaciones** (art. 9.1.h): al domicilio en España comunicado por el propietario por medio que deje constancia de recepción; en su defecto, el piso o local; si resulta imposible, tablón de anuncios con diligencia firmada por secretario con VºBº del presidente, con efectos a los 3 días naturales. La LPH no regula la notificación electrónica: se admite con consentimiento del propietario. | `unit_members.notification_address` y `electronic_notifications_consent_at`. Sin consentimiento, la convocatoria queda "pendiente de envío postal" y la app genera el PDF. Registro de intentos fallidos y de la diligencia de tablón (`meeting_notifications.channel = board`, con fecha de efectos +3 días). |
| **Cambio de titularidad** (art. 9.1.i) debe comunicarse al secretario; el transmitente que no lo hace responde solidariamente. **Certificado de deudas para la venta** (art. 9.1.e): lo emite el secretario con VºBº del presidente en un máximo de **7 días naturales**. | Flujo `unit_transfers`. Exportación del certificado de deudas para transmisión con plazo visible de 7 días desde la solicitud. |
| **Privación de voto a deudores** (art. 15.2): quienes **en el momento de iniciarse la junta** no estén al corriente de las deudas vencidas y no las hayan impugnado judicialmente ni consignado, pueden deliberar pero no votar. El acta debe reflejarlos y su persona y cuota no se computan para las mayorías. | La relación de la convocatoria es informativa; `meeting_voters.is_debtor` se **recalcula al abrir la junta** (o al abrir la ventana de voto) y el admin puede marcar `debt_challenged` / `debt_deposited`. El moroso ve la junta y puede intervenir pero el botón de voto está bloqueado. Los resultados excluyen a los privados de voto del denominador. |
| **Representación** (art. 15.1): personal o por representación con escrito firmado por el propietario. Los pisos en proindiviso deben nombrar **un** representante. En usufructo vota el nudo propietario, representado por el usufructuario salvo manifestación en contrario (delegación expresa para acuerdos del art. 17.1 y obras extraordinarias). | `vote_delegations` con documento o firma en app. Para viviendas con varios `owner`, la app exige designar un `voting_representative` por junta antes de votar. Campo `unit_members.tenure (full_owner | bare_owner | usufructuary)` para el caso de usufructo. |
| **Mayorías** (art. 17), computadas sobre el **total** de propietarios y cuotas salvo indicación: · **1/3** de propietarios y cuotas (17.1): infraestructuras de telecomunicaciones, energías renovables incluidas aerotermia y geotermia, nuevos suministros energéticos colectivos; el coste no se repercute a quien no votó a favor. · **Mayoría** de propietarios y cuotas (17.2): supresión de barreras, ascensor (aunque modifique título), obras de eficiencia energética y su financiación si no superan 12 mensualidades. · **3/5** (17.3, 17.4, 17.12): portería y servicios de interés general, arrendamiento de elementos comunes, equipos de eficiencia energética/hídrica, innovaciones no necesarias, división o agregación de pisos, alteraciones de estructura, viviendas de uso turístico y sus cuotas especiales. · **Unanimidad** (17.6): modificación del título o estatutos no regulada en otro apartado. · **Resto** (17.7): mayoría del total; **en segunda convocatoria, mayoría de asistentes que represente más de la mitad de las cuotas presentes**. · Punto de recarga privado (17.5): solo comunicación previa. | `meeting_items.majority_type`: `one_third`, `majority`, `three_fifths`, `unanimity`. La regla de segunda convocatoria (asistentes) solo se aplica a `majority` cuando el punto es de los "no regulados expresamente" (`meeting_items.legal_basis = art_17_7`); para 17.2 se computa sobre el total. `legal_basis` es un campo del punto con la lista de apartados del art. 17 y un texto de ayuda. La app muestra qué apartado aplica y si el coste es repercutible a los disidentes. Función "comunicación de punto de recarga" fuera del circuito de votación. |
| **Voto presunto de ausentes** (art. 17.8): los ausentes debidamente citados que, informados del acuerdo conforme al art. 9, no manifiesten discrepancia al secretario en **30 días naturales** por medio que deje constancia, se computan como favorables. No aplica cuando el coste no puede repercutirse a quien no votó a favor ni a reformas para aprovechamiento privativo. | Tras cerrar cada punto con `majority_type` sobre el total, el sistema notifica a los ausentes, abre `absent_dissent_deadline` (30 días naturales desde la notificación de cada uno) y recalcula al vencer. Se desactiva automáticamente para puntos con `legal_basis = art_17_1` (coste no repercutible) o marcados como aprovechamiento privativo. |
| **Junta telemática**: la LPH vigente **no la regula**. El art. 15 habla de asistencia personal o por representación y el art. 16 de "lugar, día y hora". La habilitación de la pandemia (RDL 8/2021) fue temporal y decayó el 31/12/2021. La doctrina mayoritaria admite la junta telemática o híbrida si los estatutos lo prevén o todos los propietarios lo aceptan, y siempre que se garanticen identidad, participación en tiempo real y voto. Hay un proyecto de reforma de la LPH en tramitación en el Congreso (expediente 122/000240, en fase de enmiendas desde junio de 2026) que previsiblemente la regulará. En Cataluña el Código Civil catalán tiene régimen propio. | `meetings.mode`: `in_person` (por defecto), `hybrid`, `remote_only`. Al elegir `hybrid` o `remote_only` la app exige indicar la base: `bylaws` (estatutos que lo prevén, con documento) o `unanimous_consent` (aceptación registrada de todos los propietarios del censo). Sin una de las dos, solo se permite `in_person` con voto delegado. `remote_only` queda **desactivado por defecto** en `legal_rules` hasta que exista habilitación legal. |
| **Acta** (art. 19.2): fecha y lugar; autor de la convocatoria y promotores; carácter ordinario/extraordinario y si se celebró en primera o segunda convocatoria; relación de asistentes con sus cargos y de representados, con sus cuotas; orden del día; acuerdos, indicando nombres de quienes votaron a favor y en contra y sus cuotas cuando sea relevante para la validez. **Firmas** del presidente y secretario al terminar o dentro de los **10 días naturales** siguientes (19.3); desde el cierre los acuerdos son ejecutivos. Se remite a los propietarios conforme al art. 9 (sin plazo legal). Defectos subsanables antes de la siguiente junta, que debe ratificar la subsanación. Libro de actas diligenciado por el Registrador de la Propiedad (19.1). El secretario custodia los libros y conserva **5 años** convocatorias, comunicaciones, apoderamientos y demás documentos (19.4). | Acta en PDF con todos los campos de 19.2; el nombre de los votantes a favor/en contra se incluye siempre en el registro y en el acta cuando `majority_type ≠ majority` o cuando el admin lo marque como relevante. Recordatorio de firma a los 10 días naturales. Estado `minutes_signed` desbloquea la ejecución de acuerdos. Registro de la fecha de remisión a cada propietario. Función "subsanación de acta" que genera anexo y lo añade al orden del día de la siguiente junta. Retención mínima de 5 años de convocatorias, acuses y delegaciones (`meeting_notifications`, `vote_delegations`). El acta digital no sustituye al libro diligenciado: la app lo indica y ofrece la versión imprimible. |
| **Impugnación** (art. 18): legitimados los que salvaron su voto, los ausentes y los indebidamente privados de voto; deben estar al corriente de pago (salvo impugnaciones sobre cuotas). Caducidad: 3 meses; 1 año si el acuerdo es contrario a la ley o estatutos. Para ausentes, desde la comunicación del acuerdo conforme al art. 9. | La app permite "salvar el voto" (constancia de voto en contra) y muestra a cada propietario la fecha de comunicación del acta, que arranca sus plazos. |
| **Fondo de reserva** (art. 9.1.f): titularidad de la comunidad, mínimo del **10 %** del último presupuesto ordinario; nunca por debajo del mínimo durante el ejercicio (DA 1.ª). | `communities.reserve_fund` y `annual_budget`; aviso si el fondo no llega al 10 %. |
| **Reclamación de deudas** (art. 21): la junta puede acordar medidas disuasorias (intereses superiores al legal, **privación temporal de uso de servicios o instalaciones** no esenciales) sin retroactividad (21.1). Monitorio con certificado de liquidación de deuda emitido por el secretario con VºBº del presidente (no necesario si es secretario-administrador profesional que no interviene en la reclamación), con importe y desglose, y acreditación de notificación al deudor o tablón durante 3 días (21.3). | Certificado de deuda en PDF con desglose de recibos y registro de la notificación previa al deudor. Ajuste `settings.debtors_lose_bookings` (por defecto `false`): si la junta lo acordó, los deudores no pueden reservar zonas comunes; se guarda el acuerdo que lo habilita. |
| **Funciones del administrador** (art. 20): velar por el régimen de la casa, preparar el plan de gastos, atender la conservación y disponer reparaciones urgentes **dando inmediata cuenta al presidente**, ejecutar acuerdos y pagos, actuar como secretario y custodiar la documentación. | Las incidencias de prioridad `urgent` gestionadas por el admin notifican automáticamente al presidente. |
| **Obras obligatorias sin acuerdo** (art. 10.1): conservación, accesibilidad a instancia de personas con discapacidad o mayores de 70 años hasta 12 mensualidades, etc. | Categoría de incidencia `mandatory_works` para que el admin las tramite sin junta y quede constancia de la base legal. |

### 4.2 Administrador de fincas

- No existe reserva de actividad por ley estatal: puede ser un profesional colegiado (Colegios de Administradores de Fincas) o no. La plataforma guarda `offices.collegiate_number` opcional y lo muestra en el perfil si existe.
- Como secretario custodia los libros de actas y conserva durante **5 años** las convocatorias, comunicaciones, apoderamientos y demás documentos de las juntas (art. 19.4 LPH); como administrador custodia la documentación de la comunidad a disposición de los titulares (art. 20.e). Los documentos de comunidad no se borran físicamente antes de ese plazo.
- **Protección de datos**: cuando gestiona datos de vecinos por cuenta de la comunidad actúa como **encargado del tratamiento** (art. 28 RGPD); la comunidad es la **responsable**. Necesita contrato de encargo con cada comunidad. La plataforma facilita una plantilla y guarda la fecha de firma (`communities.dpa_signed_at`).
- **Obligaciones fiscales de la comunidad que ejecuta el administrador**:
  - **Modelo 347**: las comunidades de propietarios declaran operaciones con terceros que superen 3.005,06 € anuales. La plataforma exporta por comunidad y año el total facturado por cada empresa (`invoices` aceptadas/pagadas) en formato CSV compatible.
  - **Retenciones de IRPF**: cuando la comunidad paga a un profesional (autónomo en actividad profesional), debe practicar retención (modelos 111/190). La factura debe reflejarla; ver 4.3.
  - **Modelo 184** solo si la comunidad obtiene rentas (alquiler de elementos comunes); fuera de alcance, se documenta.
- **Blanqueo de capitales**: los administradores de fincas no son sujetos obligados con carácter general, pero sí si intermedian en compraventas o alquileres. No aplica a esta plataforma; se deja constancia.

### 4.3 Empresas de servicios y autónomos

| Norma | Exigencia | Reflejo en la plataforma |
|---|---|---|
| **Registro de jornada** (art. 34.9 Estatuto de los Trabajadores, RDL 8/2019) | Registro diario con hora de inicio y fin de cada trabajador, conservado **4 años**, accesible a trabajadores, representantes legales e Inspección de Trabajo. Debe ser fiable e inalterable. | `time_entries` inmutable salvo corrección registrada en `audit_log` con motivo; exportación mensual firmada (hash) en PDF/CSV; el trabajador descarga su propio registro. Aviso si un fichaje supera 9 h o falta la salida. Solo aplica a trabajadores por cuenta ajena; el autónomo sin empleados no está obligado (la función es opcional). |
| **Coordinación de actividades empresariales** (art. 24 Ley 31/1995 PRL y RD 171/2004) | Cuando una empresa externa trabaja en la finca, la comunidad (como titular del centro) debe informar de riesgos y la empresa acreditar su prevención. | Perfil de empresa con documentos PRL opcionales (plan de prevención, TC2/RNT, seguro de RC, certificado de estar al corriente con AEAT y Seguridad Social) con fecha de caducidad; el admin ve el estado de la documentación antes de asignar. La plataforma no valida los documentos, solo su existencia y vigencia. |
| **Responsabilidad en contratas** (art. 42 ET, art. 43 LGSS) | Quien contrata obras o servicios de su propia actividad puede responder solidariamente de deudas laborales y de Seguridad Social; el certificado de la TGSS exonera. | Campo `company_documents.type = social_security_certificate` con caducidad; aviso al admin. Se marca como recomendación, no bloqueo. |
| **Facturación** (RD 1619/2012, Reglamento de facturación) | Contenido obligatorio: número y serie correlativos, fecha de expedición y de operación si difiere, NIF, nombre y domicilio de emisor y destinatario, descripción, base imponible, tipo y cuota de IVA (o mención de exención/inversión), retención de IRPF si procede. Facturas rectificativas en serie específica. Facturas simplificadas solo hasta 400 € (o 3.000 € en ciertos sectores). | Modelo `invoices` con `series`, `year`, `number` correlativo sin huecos por serie, `operation_date`, `tax_rate` por línea (21/10/4/0 con `tax_exempt_reason`), `withholding_rate` (15 % general, 7 % nuevos profesionales), `rectifies_invoice_id`. Los datos fiscales de la comunidad (CIF, domicilio) se rellenan automáticamente. |
| **Conservación de facturas** | 4 años a efectos fiscales (prescripción LGT) y **6 años** por el Código de Comercio (art. 30). | Retención mínima de 6 años; no se permite borrado físico. |
| **Verifactu / sistemas informáticos de facturación** (Ley 11/2021 antifraude, RD 1007/2023, Orden HAC/1177/2024, aplazado por RDL 15/2025) | Los programas de facturación deben generar registros con hash encadenado y código QR, con remisión voluntaria a la AEAT. Obligatorio desde el **1 de enero de 2027** para contribuyentes del Impuesto sobre Sociedades y desde el **1 de julio de 2027** para el resto de empresas y autónomos. Los desarrolladores de software de facturación están obligados desde el 29/07/2025. | **Decisión**: la plataforma no es un sistema de facturación: registra facturas emitidas con el software de la empresa (datos + PDF) y no genera numeración ni documentos con efectos fiscales. Así no queda sujeta al RD 1007/2023. Cualquier función futura de emisión se trata como fase 3 y exige cumplir Verifactu desde el primer día. |
| **Factura electrónica B2B** (Ley 18/2022 "Crea y Crece", reglamento pendiente) | Obligación progresiva de facturación electrónica entre empresas. Las comunidades de propietarios no son empresarios, por lo que en principio no les afecta como receptoras, pero sí a las empresas entre ellas. | Sin impacto directo; se monitoriza. |
| **IVA en obras en comunidades** | Tipo reducido del 10 % para obras de renovación y reparación en viviendas si se cumplen requisitos (materiales < 40 % del coste). | El campo `tax_rate` por línea admite 10 %; la app muestra un aviso informativo, sin validar los requisitos. |
| **Retención IRPF por comunidades** (art. 76 RIRPF) | Las comunidades de propietarios están obligadas a retener cuando pagan rendimientos de actividades **profesionales** (no empresariales). | `companies.activity_type = professional | business`; si es `professional` y autónomo, la factura exige `withholding_rate`. |
| **Ley de Servicios de la Sociedad de la Información (LSSI, Ley 34/2002)** | La empresa que se anuncia en el directorio debe identificarse (denominación, NIF, domicilio, contacto). | Perfil público con estos datos obligatorios antes de ser verificada. |

### 4.4 Protección de datos y firma electrónica (transversal)

- **RGPD y LOPDGDD (LO 3/2018)**. Reparto de papeles:
  - La **comunidad de propietarios** es responsable del tratamiento de los datos de sus vecinos.
  - El **administrador** es encargado (o corresponsable en lo que gestione por cuenta propia).
  - La **plataforma** es encargada del tratamiento de la comunidad y del despacho, y **subencargados**: Cloudflare (R2, con ubicación EU y cláusulas contractuales tipo), el proveedor de email, Expo (push, solo tokens) y Sentry. Deben figurar en los términos del servicio y en el contrato de encargo.
  - Las **empresas** que reciben datos para ejecutar un servicio (nombre, teléfono y dirección del vecino o de la comunidad) los tratan como **responsables independientes** para su propia finalidad (ejecución del encargo), no como encargadas. Por eso la plataforma aplica minimización: solo se les entrega el contacto estrictamente necesario, y en `service_requests` únicamente con el consentimiento explícito del vecino. Los términos del directorio obligan a la empresa a usar los datos solo para el servicio.
- **Base jurídica**: ejecución de la relación de comunidad (LPH) para vecinos y administrador; consentimiento para funcionalidades no necesarias (directorio, valoraciones, contacto con empresas, geolocalización en fichajes).
- **Registro de actividades de tratamiento** (art. 30 RGPD): la plataforma entrega a cada comunidad una ficha exportable con las actividades que realiza en su nombre.
- **Listas de morosos**: la AEPD y la LPH permiten incluirlas en la convocatoria (que solo reciben los propietarios), pero **no** exponerlas en zonas visibles a terceros. La lista se muestra únicamente dentro de la convocatoria y del acta, nunca en el tablón general ni en avisos.
- **Fotos de incidencias**: pueden captar personas o matrículas. La app avisa antes de subir y el admin puede pedir el borrado; se elimina el EXIF (GPS) en cliente antes de subir.
- **Datos de menores o de salud** (por ejemplo, justificación de obras de accesibilidad por discapacidad): se tratan como categorías especiales; solo el admin puede adjuntarlos y quedan en carpeta `board`.
- **Videovigilancia y control de accesos**: fuera de alcance; si se integra en el futuro aplica la Instrucción 1/2006 y el art. 22 LOPDGDD.
- **Derechos**: acceso, rectificación, supresión, portabilidad, oposición y limitación desde el perfil (`/me/export`, `/me`). La exportación se genera en el worker y se entrega por enlace de un solo uso que caduca en 24 h; tanto exportar como borrar la cuenta exigen reintroducir la contraseña (y el 2FA si está activo). Plazo de respuesta 1 mes. La supresión anonimiza pero no borra actas ni votos (obligación legal de conservación prevalece).
- **Brechas de seguridad**: notificación a la AEPD en 72 h por parte del responsable; la plataforma como encargada notifica a los responsables (despachos) sin dilación. Procedimiento documentado en el runbook.
- **Delegado de protección de datos**: no obligatorio para comunidades ni para la mayoría de despachos, pero sí recomendable para la plataforma si el volumen de tratamiento es grande (art. 34.1 LOPDGDD). Se evalúa al superar 100 despachos.
- **Cookies y aviso legal** (LSSI): la web pública lleva banner de cookies solo si usa cookies no técnicas; aviso legal con datos del titular; términos de uso y política de privacidad enlazados desde el registro y el pie.
- **Firma electrónica** (Reglamento eIDAS 910/2014 y Ley 6/2020):
  - **Firma simple** con identificación de usuario, IP, hora y aceptación explícita: suficiente para delegaciones de voto, aceptación de tareas, partes de trabajo y acuse de recibo.
  - **Firma avanzada** (OTP por email o código TOTP de la app de autenticación + sello de tiempo + hash del documento): la que se exige en la plataforma para **votar a distancia en juntas `hybrid` o `remote_only`** y para **firmar actas** (presidente y secretario). Se conserva la evidencia (`signature_evidence`).
  - **Firma cualificada** (certificado digital, DNIe): no se exige; se prevé integración con un prestador cualificado (fase 3) para comunidades que lo acuerden en estatutos.
- **Identidad del votante**: para dar validez al voto telemático se exige, además de la cuenta creada por invitación del administrador, verificación por OTP en el momento de votar (al email de la cuenta, o TOTP si el propietario lo tiene activo) y que el propietario haya sido dado de alta por el admin con su DNI/NIE (`users.id_document_encrypted`). No se guarda copia del documento, solo el número cifrado, y se muestra enmascarado.

### 4.5 Accesibilidad y consumidores

- Las apps para comunidades no están en el ámbito directo del RD 1112/2018 (sector público), pero la **Ley 11/2023** (accesibilidad de productos y servicios, en vigor desde junio de 2025) puede aplicar a servicios de comercio electrónico y a los términos de contratación con empresas. Se adopta WCAG 2.1 AA como objetivo.
- Si la plataforma cobra a despachos o empresas (SaaS), aplica la normativa de contratación electrónica: confirmación de contratación, condiciones accesibles y conservables, e información precontractual. Los vecinos no pagan a la plataforma, por lo que no hay relación de consumo con ellos.

### 4.6 Checklist de verificación legal antes de cada fase

- Fase 1: contrato de encargo plataforma–despacho; política de privacidad; consentimiento de notificaciones electrónicas; cláusulas del directorio para empresas.
- Fase 2: revisión con abogado de las modalidades `hybrid` y `remote_only` a la luz del estado de la reforma de la LPH; texto de la delegación de voto; plantilla de acta conforme al art. 19 LPH; textos de consentimiento de notificación electrónica.
- Continuo: seguimiento del proyecto de reforma de la LPH (Congreso, expediente 122/000240) y de las modificaciones puntuales (en 2025-2026 se ha tocado el art. 17 tres veces).

### 4.7 Fuentes verificadas y fecha de consulta

| Norma | Fuente | Verificado | Estado |
|---|---|---|---|
| LPH, Ley 49/1960 | BOE texto consolidado, actualización publicada 21/03/2026 (RDL 7/2026 modifica art. 17.1) | 04/09/2026 | Vigente; reforma en tramitación en el Congreso |
| Verifactu, RD 1007/2023 | RDL 15/2025 (BOE 03/12/2025) | 04/09/2026 | Obligatorio 01/01/2027 (IS) y 01/07/2027 (resto) |
| Registro de jornada, art. 34.9 ET (RDL 8/2019) | Texto refundido ET | Conocimiento previo, no reconsultado | Vigente; conservación 4 años |
| Reglamento de facturación, RD 1619/2012 | BOE | Conocimiento previo, no reconsultado | Vigente |
| RGPD y LOPDGDD | DOUE / BOE | Conocimiento previo | Vigente |
| Ley 11/2023 accesibilidad | BOE | Conocimiento previo | Aplicable desde 28/06/2025 |

Las filas marcadas "conocimiento previo" no han cambiado en años, pero conviene reconsultarlas antes de construir el módulo correspondiente.

## 5. Requisitos funcionales

### 5.1 Autenticación
- Registro por invitación: el administrador crea la vivienda e invita por email (enlace con token) o genera un código de 8 caracteres para entregar en papel. Nadie se registra libremente en una comunidad. La invitación caduca a los 14 días y puede reenviarse.
- Login con email + contraseña. Recuperación por email. Longitud mínima de contraseña condicionada al segundo factor: **15 caracteres** en cuentas sin 2FA y **12 caracteres** con TOTP activo (NIST SP 800-63B rev. 4, de 26-08-2025: 15 es el mínimo para autenticación solo con contraseña, 8 el mínimo cuando la contraseña es un factor de MFA). El mensaje de error indica qué regla aplica. Comprobadas contra filtraciones conocidas (API k-anonymity de Have I Been Pwned, prefijo de 5 caracteres del hash SHA-1, cabecera `Add-Padding: true` y descarte de las entradas con recuento 0) y hasheadas con Argon2id (m=19456 KiB, t=2, p=1, sal de 16 bytes, tag de 32 bytes; perfil de OWASP, elegido sobre los 64 MiB del RFC 9106 porque la fase A sitúa base de datos, cola y API en la misma máquina). Las respuestas de login, recuperación e invitación no revelan si el email existe.
- Verificación por OTP de 6 dígitos (5 min, 5 intentos) necesaria antes de poder votar a distancia o firmar actas. El código se envía **al email de la cuenta**, o se toma de TOTP si el propietario lo tiene activo. El OTP se guarda hasheado y nunca se registra en logs.
- 2FA con TOTP (`otplib`): **obligatorio para `superadmin` desde M0 y para `admin`/`admin_staff` desde M1**; opcional para el resto. Códigos de recuperación de un solo uso.
- Tokens JWT: access (15 min) + refresh (30 días con rotación; un refresh usado dos veces invalida toda la familia de tokens).
  - Móvil: ambos tokens en `expo-secure-store`. Se envían como `Authorization: Bearer`.
  - Web: el access token vive solo en memoria; el refresh va en cookie `httpOnly; Secure; SameSite=Strict; Path=/v1/auth/refresh; Domain=api.DOMAIN` (no se comparte con `DOMAIN` ni con otros subdominios; `app.DOMAIN` y `api.DOMAIN` son *same-site*, así que `Strict` funciona con `fetch` + `credentials: 'include'`). CORS con `credentials: true` solo para `https://app.DOMAIN`; la web pública llama a la API sin credenciales. La API valida además la cabecera `Origin` en `/auth/refresh` y exige en esa petición un **token CSRF**: `SameSite` y `Origin` no bastan por sí solos como defensa CSRF, porque la cabecera `Origin` falta en un 1-2 % del tráfico real y `SameSite` no protege frente a un subdominio comprometido (OWASP CSRF Prevention Cheat Sheet). Una petición con cookie válida pero sin token CSRF, o con uno inválido, se rechaza.
  - El endpoint `/auth/refresh` acepta el refresh token por cookie (web) o por body (móvil), nunca ambos en la misma petición.
  - Los tokens de refresh, invitación, reset de contraseña y códigos cortos se guardan **hasheados** (SHA-256) y son de un solo uso; el reset caduca en 1 h.
- Sesiones: tabla `sessions` con dispositivo y última actividad. El usuario puede cerrar sesiones remotas desde su perfil.
- Bloqueo progresivo tras 5 intentos fallidos en 15 minutos (por email y por IP) y aviso al usuario por email; CAPTCHA (Cloudflare Turnstile) en login web tras el tercer fallo y siempre en los formularios públicos (alta de empresa, contacto, recuperación).
- Opcional fase 2: login biométrico local en móvil, OAuth Google/Apple.
- Tras el login, la app carga las membresías y redirige al portal según el rol. Si tiene varias, muestra selector de comunidad/empresa. La última selección se recuerda en el dispositivo.
- Deep links: esquema `vecingest://` y universal links en `app.DOMAIN` para invitaciones (`/invite/:token`), reset de contraseña y apertura de una incidencia desde una push (`/incidents/:id`). La app solo navega a rutas de una lista blanca; el campo `url` de una push o de un enlace nunca se abre tal cual (evita open redirect).
- Usuario `superadmin` inicial: creado por `vecingest bootstrap-superadmin`, subcomando idempotente e independiente de `APP_ENV`, separado de las fixtures de desarrollo de `vecingest seed`. Se ejecuta a mano una sola vez contra el entorno de destino y las credenciales se le pasan en la invocación, de modo que nunca queda una contraseña de administrador en las variables del stack. Ejecutarlo dos veces sobre la misma base de datos no duplica el usuario ni falla.

### 5.2 Comunidades y viviendas
- Comunidad: nombre, dirección, CIF, despacho gestor, presidente vigente, número de viviendas, zonas comunes (fase 2). Subcomunidades y complejos inmobiliarios (art. 2 y 24 LPH) se modelan como comunidades independientes vinculadas por `parent_community_id`; la comunidad agrupada tiene como votantes a los presidentes de las integradas (fuera del MVP, pero el campo existe desde M1 para no migrar después).
- Viviendas: portal, piso, puerta, tipo, coeficiente de participación (%), referencia catastral (opcional). La suma de coeficientes de una comunidad debe ser 100 ± 0,01; se avisa al admin si no cuadra pero no se bloquea.
- Cada vivienda tiene 0..n miembros (owner/tenant). Una vivienda puede tener varios propietarios (cotitulares); a efectos de voto cuenta una vez.
- Importación masiva de viviendas y propietarios por CSV con plantilla descargable, validación previa (se muestran errores línea a línea) y confirmación antes de escribir.
- Datos legales de la comunidad: CIF, presupuesto anual, fondo de reserva, fecha de la última junta ordinaria, si el secretario es el despacho, fecha del contrato de encargo de tratamiento.
- Consentimiento de notificaciones electrónicas por miembro (fecha, versión del texto aceptado). Sin él, las convocatorias se marcan como pendientes de envío en papel.
- Cambio de titularidad de una vivienda: fecha de transmisión; los recibos anteriores siguen vinculados al titular anterior y el nuevo titular no ve el histórico previo salvo actas y documentos.
- `settings` de comunidad (jsonb validado contra un esquema JSON Schema en `packages/shared`): `incident_categories_enabled`, `tenants_can_create_incidents` (por defecto `true`), `auto_close_days` (por defecto 7), `booking_cancellation_hours`, `announcements_require_admin_approval` (si el presidente publica, por defecto `false`), `show_individual_votes_in_minutes` (por defecto `false`), `meeting_second_call_minutes` (por defecto 30), `debtors_lose_bookings` (por defecto `false`, requiere acuerdo de junta adjunto), `remote_meetings_basis` (`none` | `bylaws` | `unanimous_consent`; es el valor por defecto que copia cada junta en `meetings.mode_basis`).

### 5.3 Incidencias (núcleo del MVP)
Estados y transiciones permitidas:

| De | A | Quién |
|---|---|---|
| `open` | `assigned` | admin (al asignar empresa) |
| `open` | `in_progress` | admin (lo resuelve el despacho sin empresa) |
| `open` | `rejected` | admin (motivo obligatorio) |
| `assigned` | `in_progress` | empresa (al aceptar) |
| `assigned` | `open` | empresa (al rechazar, motivo obligatorio; se notifica al admin y se desasigna) |
| `assigned` | `assigned` | admin (reasignar a otra empresa) |
| `in_progress` | `resolved` | empresa o admin |
| `resolved` | `closed` | admin, creador, o job automático a los `auto_close_days` |
| `resolved` | `in_progress` | creador o admin (reabrir porque no está bien resuelto, máximo 2 veces) |
| `closed` / `rejected` | — | estado final; solo admin puede reabrir a `open` |

- Creación por vecino, presidente o admin: título, descripción, categoría (ascensor, fontanería, electricidad, limpieza, cerrajería, jardinería, obras, obras obligatorias art. 10 LPH, ruidos y convivencia, otros), ámbito (zona común o vivienda propia), hasta 5 fotos, prioridad sugerida. La prioridad definitiva la fija el admin.
- Los inquilinos pueden crear incidencias si `tenants_can_create_incidents` está activo.
- El administrador ve las incidencias de todas sus comunidades, puede cambiar prioridad, rechazar, o asignar a una empresa (del directorio o de sus proveedores habituales). Al asignar puede añadir una nota interna con instrucciones y un presupuesto máximo.
- La empresa recibe la asignación, la acepta o rechaza, la pasa a `in_progress`, añade comentarios y fotos, y la marca `resolved`. Si no responde en 48 h, el admin recibe un aviso.
- Hilo de comentarios por incidencia. Los comentarios pueden ser internos (solo admin y empresa) o públicos (también el vecino). El vecino nunca ve comentarios internos ni presupuestos.
- Cada cambio de estado se registra en `incident_events` y genera notificación a los implicados.
- Cierre automático: un job periódico del worker (River, cada hora) cierra las `resolved` que superan `auto_close_days`.
- Visibilidad:
  - Incidencia de zona común: la ven todos los miembros de la comunidad, el admin y la empresa asignada.
  - Incidencia de vivienda: solo el creador, otros miembros de esa vivienda, el admin y la empresa asignada. El presidente no la ve.
  - La empresa ve solo los campos necesarios (título, descripción, fotos, ubicación, contacto del admin). No ve datos de otros vecinos ni el hilo público completo salvo lo que el admin marque como visible.
- Duplicados: al crear, se muestran incidencias abiertas de la misma categoría en la comunidad para evitar duplicados; el vecino puede "sumarse" a una existente (contador `affected_count`).

### 5.4 Comunicaciones
- El admin o el presidente publica avisos: título, cuerpo (markdown básico), adjuntos, destinatarios, fecha de publicación (permite programar) y fecha de caducidad.
- Destinatarios (`target` + `target_filter`): `all` (todos los miembros), `owners` (solo propietarios), `block` (`{ blocks: [...] }`), `units` (`{ unit_ids: [...] }`).
- Avisos fijados (`is_pinned`) aparecen siempre arriba hasta que caducan.
- Estados: `draft` → `pending_approval` (solo si publica el presidente y `announcements_require_admin_approval` está activo) → `published` → `expired`. El admin aprueba o devuelve con comentario.
- Marcado como leído por usuario. El admin ve el porcentaje de lectura calculado sobre los usuarios destinatarios en el momento de publicar (se guarda `recipient_count`).
- Notificación push + email al publicar (configurable por el usuario). Se envía una sola vez por usuario aunque tenga varias viviendas en la comunidad.
- Los avisos no admiten comentarios de vecinos en el MVP.
- Fase 2: comunicaciones certificadas con acuse.

### 5.5 Documentos
- Carpetas por comunidad: actas, estatutos, presupuestos, seguros, contratos, otros.
- Subida por admin. Visibilidad por carpeta: `all` (todos los miembros), `owners` (solo propietarios), `board` (junta directiva y admin).
- Versionado simple: subir un documento con el mismo nombre en la misma carpeta crea versión nueva y conserva la anterior. Solo se lista la última; las anteriores se ven en el detalle.
- Límites: 25 MB por documento. Tipos permitidos: PDF, JPEG, PNG, WebP, HEIC, DOCX/XLSX/PPTX. **No** se admiten SVG, HTML, ZIP ni macros (DOCM/XLSM). El tipo se valida por *magic bytes* tras la subida, no por extensión ni por la cabecera del cliente.
- Los documentos se escanean con ClamAV en el worker antes de ser visibles (`scan_status`); mientras están `pending` solo los ve quien los subió. Las descargas usan `response-content-disposition: attachment` y `response-content-type` fijados en la URL prefirmada.
- Almacenamiento en Cloudflare R2, bucket privado, acceso por URL prefirmada de 15 minutos. Cada descarga se registra en `audit_log` (necesario para actas y documentos con datos personales).

### 5.6 Directorio de empresas
- Perfil público de empresa: nombre comercial, razón social, NIF, domicilio (obligatorios por LSSI), logo, gremios, zonas de trabajo (provincias), descripción, teléfono, web, horario de atención, valoración media.
- Búsqueda por gremio y provincia.
- Solo aparecen empresas con `status = verified`. Las `suspended` desaparecen del directorio y no pueden recibir nuevas asignaciones, pero conservan las abiertas.
- Un vecino puede solicitar presupuesto para su vivienda (crea una `service_request` privada, no una incidencia). La solicitud lleva descripción, fotos, dirección de la vivienda y teléfono de contacto que el vecino autoriza compartir explícitamente.
- Valoración 1–5 con comentario al cerrar una incidencia o solicitud. Solo puede valorar quien creó la incidencia o solicitud, una vez. La empresa puede responder públicamente a la valoración.
- `rating_avg` y `rating_count` se recalculan en cada valoración nueva (no se calculan al vuelo).

### 5.7 Reserva de zonas comunes (fase 2)
- Zonas definidas por el admin: nombre, aforo, horario, duración mínima/máxima, antelación máxima, límite de reservas por vivienda y mes, precio opcional.
- Calendario con disponibilidad. Reserva por franjas. Cancelación hasta `booking_cancellation_hours` antes.
- El admin puede bloquear franjas (mantenimiento, eventos de la comunidad) mediante `common_area_blocks`; un bloqueo cancela las reservas afectadas y notifica.
- Evitar solapamientos con bloqueo a nivel de base de datos: constraint de exclusión sobre `tstzrange` (requiere extensión `btree_gist`).
- Si la zona tiene precio, la reserva queda `pending` hasta que el admin la confirma; el cobro se gestiona fuera de la plataforma (se anota en recibos).
- Si `debtors_lose_bookings` está activo (solo con acuerdo de junta registrado, art. 21.1 LPH), las viviendas con deuda vencida no pueden reservar; el mensaje indica el acuerdo que lo habilita.
- Zonas horarias: toda la plataforma opera en `Europe/Madrid`; los timestamps se guardan en UTC y se convierten en cliente.

### 5.8 Juntas y voto online (fase 2)
- Convocatoria conforme al art. 16 LPH: convocante (presidente o promotores), carácter, lugar, primera y segunda convocatoria, orden del día con puntos y su base legal del art. 17, documentación adjunta, relación de propietarios no al corriente de pago con advertencia de privación de voto. Validaciones: 6 días mínimos para la ordinaria; segunda convocatoria el mismo día (≥ 30 min) o dentro de 8 días naturales con ≥ 3 días de antelación.
- Propuestas de puntos por cualquier propietario (`agenda_requests`); el presidente las ve al preparar la siguiente convocatoria y debe incluirlas (art. 16.2).
- Envío de la convocatoria por app y email a quien tenga consentimiento electrónico; para el resto se genera el PDF y el admin registra el envío postal, el intento fallido y, en su caso, la diligencia de tablón (efectos a los 3 días naturales).
- Modalidad: `in_person` por defecto. `hybrid` o `remote_only` solo si la comunidad tiene base registrada (estatutos o aceptación unánime). `remote_only` deshabilitado globalmente hasta que la LPH lo regule (`legal_rules.remote_only_enabled`). En modalidad híbrida la plataforma aporta identificación (OTP), asistencia registrada, intervención en tiempo real (enlace de videoconferencia externo) y voto; la sesión presencial la conduce el presidente y el secretario levanta el acta única.
- Snapshot del censo al publicar (`meeting_voters`): viviendas, cuotas, propietarios, tenencia (pleno dominio / nuda propiedad / usufructo) y representante designado en proindiviso. Los votos se validan contra este censo.
- Deudores: la relación de la convocatoria es informativa. **Al abrir la junta** se recalcula `is_debtor` con los recibos vencidos en ese momento; el admin puede marcar impugnación judicial o consignación para restituir el voto. Los privados de voto pueden asistir y comentar pero no votar, y no cuentan en el denominador de ninguna mayoría.
- Voto por punto: a favor, en contra, abstención. Resultados por cuotas y por cabezas. Solo votan `owner`; una vivienda emite un solo voto (representante designado si hay cotitulares; nudo propietario o usufructuario según delegación). Cambio de voto permitido hasta el cierre; se guarda como nuevo registro en la cadena y cuenta el último.
- Delegación conforme al art. 15.1: a cualquier persona (no solo propietarios) con escrito firmado o firma en app; revocable hasta el inicio.
- Mayorías según art. 17 con `legal_basis` por punto; cálculo automático incluido el régimen de segunda convocatoria para los acuerdos del art. 17.7. Visualización de si el coste es repercutible a los disidentes (17.1, 17.4).
- Voto presunto de ausentes (art. 17.8): para los puntos donde aplica, notificación a los ausentes, plazo de 30 días naturales desde la notificación de cada uno, registro de discrepancias y recálculo final con anexo al acta.
- Salvar el voto: cualquier propietario presente puede dejar constancia de su voto en contra o de que salva su voto, a efectos del art. 18.2.
- Acta con el contenido del art. 19.2, firma de presidente y secretario con firma avanzada (OTP + sello de tiempo) al terminar o en los 10 días naturales siguientes; desde el cierre los acuerdos pasan a `executable`. Remisión a los propietarios con registro de fecha por cada uno (arranca los plazos de impugnación de los ausentes). Subsanación de errores mediante anexo que se ratifica en la siguiente junta.
- Registro inmutable: hash SHA-256 encadenado en `votes`, `signature_evidence` y timestamp del servidor. Resultados congelados en `meeting_item_results`.
- Retención: convocatorias, acuses, delegaciones y actas al menos 5 años (art. 19.4); en la práctica, indefinida para actas.

### 5.9 Recibos y saldo (fase 2)
- El admin sube recibos por vivienda (import CSV o manual): concepto, importe, fecha, estado (pendiente, pagado, devuelto).
- El propietario ve su saldo y descarga los recibos en PDF.
- El propietario pagador (`unit_members.is_payer`) puede actualizar su IBAN (se notifica al admin para validación). El IBAN se cifra en aplicación con AES-256-GCM usando `ENCRYPTION_KEY`; nunca se devuelve completo por API, solo los 4 últimos dígitos.
- Morosidad: una vivienda es morosa si tiene recibos `pending` con `due_date` vencida. Este estado alimenta la relación de deudores de las convocatorias, el recálculo al abrir la junta (art. 15.2), el bloqueo opcional de reservas (art. 21.1) y dos certificados en PDF: el de liquidación de deuda para el monitorio (art. 21.3, con desglose y constancia de notificación previa al deudor) y el de estado de deudas para la venta de la vivienda (art. 9.1.e, plazo de 7 días naturales desde la solicitud).
- Los recibos se generan fuera (software del despacho) y se importan; la plataforma no calcula cuotas en esta fase.

### 5.10 Empresas: tareas, facturas y control horario (fase 2)
- Tarea: derivada de una incidencia o creada manualmente. Asignada a uno o varios trabajadores, con fecha prevista, partes de trabajo y fotos.
- Factura: serie + número correlativo por empresa y año (sin huecos, asignado al pasar de borrador a enviada), fecha, líneas, tipo de IVA por línea, retención IRPF opcional (autónomos), total, PDF generado con los datos fiscales obligatorios (NIF de ambas partes, dirección, desglose de impuestos). Vinculada a comunidad y opcionalmente a incidencia. Estados: borrador, enviada, aceptada, pagada, disputada. Una factura enviada no se edita: se emite rectificativa.
- El admin ve las facturas recibidas de sus comunidades y las marca como aceptadas/pagadas. Exportación CSV para contabilidad y exportación anual por proveedor para el modelo 347 (proveedores que superan 3.005,06 €).
- Documentación de la empresa (PRL, seguro de RC, certificados AEAT y TGSS) con caducidad; el admin ve un semáforo de vigencia al asignar. Ver 4.3.
- Fichaje: entrada/salida con geolocalización opcional. El trabajador solo puede corregir un fichaje en las 24 h siguientes y queda registrado en `audit_log`; después solo la empresa. Informe mensual exportable por trabajador en PDF y CSV. Debe cumplir registro horario legal en España (conservación 4 años, inalterabilidad).
- Decisión de alcance: la plataforma **no emite** facturas; la empresa las emite con su software y aquí registra los datos y sube el PDF. Así se evita quedar sujeta a Verifactu. Ver 4.3.

### 5.11 Notificaciones
- Canales: push (Expo Push), email (SMTP), in-app (bandeja).
- Preferencias por usuario y por tipo de evento. Las notificaciones de seguridad (reset de contraseña, nueva sesión, cambio de 2FA) no se pueden desactivar.
- El cuerpo de las push y de los emails nunca incluye datos sensibles (importes de deuda, sentido de votos, IBAN, DNI): títulos genéricos y enlace a la app. La pantalla de bloqueo del móvil no debe mostrar contenido privado.
- Los eventos se encolan en Postgres (River) y se envían de forma asíncrona con 3 reintentos y backoff exponencial. Los tokens push que Expo marca como `DeviceNotRegistered` se eliminan.
- Deduplicación: un mismo evento no genera más de una notificación por usuario aunque tenga varias membresías afectadas.
- Emails con plantillas HTML en `html/template` de Go (con MJML precompilado a HTML en el repositorio para el diseño responsive), remitente configurable por despacho (`Nombre del despacho vía Vecingest <no-reply@mail.DOMAIN>`), enlace de gestión de preferencias en el pie.
- Push mediante la API HTTP de Expo (`exp.host/--/api/v2/push/send`) en lotes de 100; se guarda el `ticket` y se consultan los `receipts` a las 15 min para detectar errores.

## 6. Requisitos no funcionales

- **Idioma**: español por defecto. Preparado para i18n (euskera, catalán, gallego, inglés) con `i18next`; los textos legales (convocatorias, actas, certificados) se generan siempre en castellano y, opcionalmente, en la lengua cooficial configurada por la comunidad.
- **Rendimiento**: API < 50 ms (p95) sin acceso a disco y < 150 ms (p95) con consulta a Postgres; listados paginados por cursor en todos los casos; ninguna consulta N+1 (se comprueba con el log de consultas por petición en desarrollo). Presupuesto de memoria: `api` y `worker` < 128 MB en reposo. Arranque del proceso < 1 s.
- **Objetivos de nivel de servicio (SLO)** que se miden desde M0 y disparan el paso de fase (7.10): disponibilidad de la API 99,9 % mensual; p95 de lectura < 150 ms y de escritura < 300 ms; entrega de push < 30 s desde el evento en el p95; cierre de una votación con 5.000 votantes en menos de 5 s; cero pérdida de jobs (cola transaccional).
- **Capacidad de diseño**: 5 M usuarios, 250 k comunidades, 100 M incidencias acumuladas, 500 M notificaciones al año, 50 M ficheros en R2 (~50 TB), pico de 2.000 peticiones/s en tardes de junta (los martes y jueves de septiembre-octubre y marzo-abril concentran las juntas ordinarias). Cada tabla del modelo tiene definida su estrategia de crecimiento en 7.10.
- **Seguridad**: contraseñas con Argon2id (m=19456 KiB, t=2, p=1; parámetros y política de longitud en 5.1). Rate limiting: 10 req/min en login y reset, 300 req/min por usuario autenticado, 60 req/min por IP en endpoints públicos. Cabeceras de seguridad (middleware `secure` de chi). Validación de todo input con las etiquetas de `huma` y `DisallowUnknownFields`. Auditoría de acciones sensibles (`audit_log`). Dependencias auditadas en CI. 2FA (TOTP) obligatorio para `superadmin` (M0) y `admin`/`admin_staff` (M1), opcional para el resto. Secretos solo en variables de entorno; nunca en el repositorio.
- **Ficheros**: imágenes máximo 10 MB, comprimidas en cliente a 1920 px de lado mayor y calidad 80 antes de subir; documentos máximo 25 MB. Lista blanca de MIME. Las claves de R2 siguen el patrón `{community_id}/{entity}/{entity_id}/{uuid}.{ext}` para poder borrar por comunidad.
- **RGPD**: exportación de datos del usuario en JSON, borrado con anonimización (se conserva el histórico de incidencias y votos con el usuario sustituido por "Usuario eliminado"), registro de consentimientos, ubicación de datos en la UE (servidor propio + R2 con jurisdicción EU). Retención: fichajes 4 años, facturas 6 años, recibos y certificados de deuda 6 años (prescripción de 5 años de las cuotas, art. 1966 CC, más margen), convocatorias, delegaciones y acuses 5 años (art. 19.4 LPH), actas indefinido, incidencias cerradas 3 años y luego se anonimizan las fotos, logs de acceso 1 año.
- **Observabilidad**: logs estructurados con `slog` (JSON), `request_id` en cada petición, Sentry en API y app, métricas básicas de cola (jobs fallidos).
- **Disponibilidad**: backups diarios de Postgres (retención 30 días) con prueba de restauración mensual documentada. Healthcheck en `/health` (API, BD, R2). Objetivo RPO 24 h, RTO 4 h.
- **Accesibilidad**: contraste AA, tamaños de fuente escalables, etiquetas en todos los controles.
- **Offline**: la app móvil cachea las últimas lecturas y permite redactar una incidencia sin conexión y enviarla al recuperar red.


### 6.1 Seguridad: modelo de amenazas, controles y acciones

Objetivo: **OWASP ASVS nivel 2** para la API y **OWASP MASVS-L1 + R** (resiliencia básica) para la app. La plataforma custodia datos de identidad, deudas, votos y cuentas bancarias de miles de personas; el atacante realista no es un Estado, es un vecino descontento, un ex-empleado de un despacho, un bot de credential stuffing o un ransomware que entra por el servidor.

**Activos críticos** (por orden): base de datos (identidad + deudas + IBAN), evidencias de voto y actas, backups, claves (`ENCRYPTION_KEY`, JWT, R2, Expo), cuenta de superadmin, cuenta de Expo/EAS (puede empujar código a todos los móviles), cuenta de Cloudflare, servidor y NPM.

**Amenazas y controles**

| Amenaza | Control |
|---|---|
| Credential stuffing y fuerza bruta | Argon2id, contraseñas comprobadas contra HIBP, bloqueo progresivo, Turnstile, 2FA obligatorio en admins, alertas de inicio de sesión nuevo |
| Robo de sesión (XSS, dispositivo perdido) | Access token de 15 min en memoria, refresh en cookie `Strict` + `Path` acotado, rotación con detección de reutilización, cierre remoto de sesiones, CSP sin inline |
| IDOR entre comunidades o despachos | `MembershipGuard` en todos los endpoints, repositorios con ámbito obligatorio, **test automático de matriz de permisos** que llama a cada endpoint con cada rol y con recursos de otra comunidad y espera 403/404 |
| Mass assignment | Structs de entrada y de salida separados, decodificación JSON con `DisallowUnknownFields`; nunca se decodifica el cuerpo sobre el modelo de base de datos; los structs de salida no tienen campos `*_hash` ni `*_encrypted` |
| Ficheros maliciosos (malware, SVG con script, polyglots) | Lista blanca por *magic bytes*, sin SVG/HTML/ZIP, ClamAV en worker, `Content-Disposition: attachment`, URLs de descarga de 5 min, miniaturas generadas en cliente y re-codificadas |
| Inyección (SQL, comandos, plantillas) | Consultas generadas por `sqlc` (siempre parametrizadas; `fmt.Sprintf` en SQL prohibido por linter); `html/template` con escapado automático; plantillas de PDF sin evaluación de código; markdown sanitizado con `bluemonday` |
| Manipulación de votos o actas por un operador con acceso a la BD | Cadena de hash por punto + `signature_evidence` + **anclaje externo**: al cerrar cada junta se sella el `head_hash` con un servidor de sellado de tiempo RFC 3161 y se envía por email al presidente y al secretario. Una alteración posterior es detectable por cualquiera de ellos |
| Alteración del registro de jornada o del `audit_log` | Tablas append-only: el rol de BD de la aplicación no tiene `UPDATE`/`DELETE` sobre `audit_log`, `votes`, `time_entries`, `signature_evidence`; hash encadenado y anclaje diario del `audit_log` |
| Fuga por logs o errores | Redacción de PII en el handler de `slog`, errores genéricos hacia el cliente (`code` estable, detalle solo en servidor), Sentry sin PII, OpenAPI y Swagger UI desactivados en producción |
| Spoofing de IP para evadir límites | `trust proxy` restringido a la IP de NPM |
| **Compromiso del buzón de correo del propietario** (riesgo asumido, ver 11) | El OTP de voto y firma viaja por el mismo canal que la recuperación de contraseña, así que quien controla el buzón puede suplantar al votante por completo. Mitigación parcial: límite de 3 envíos/hora y 10/día por cuenta, OTP de un solo uso hasheado en BD, alerta al propietario por cada emisión, y `signature_evidence` registra el canal usado para que una impugnación pueda distinguir un voto firmado con TOTP de uno firmado por email. Se ofrece TOTP como alternativa más fuerte y se recomienda activarlo a quien vaya a votar |
| Phishing con emails de la plataforma | SPF, DKIM (2048) y DMARC `p=reject` en el dominio de envío (`mail.DOMAIN`), BIMI opcional; los emails nunca piden contraseña ni incluyen enlaces a dominios de terceros; nombre del despacho como remitente visible pero dominio de la plataforma |
| Compromiso del servidor (ransomware) | Backups cifrados con clave distinta (`age`) hacia un bucket R2 **separado con credenciales de solo escritura** y *object lock*/versionado; la API no tiene credenciales para borrar backups; restauración probada mensualmente; el servidor solo expone 80/443 (NPM) y SSH con clave |
| Compromiso de un contenedor vecino en la red de NPM (red compartida con otros proyectos) | `db` solo en red interna; `api` valida `Origin`/`Host`; contenedores `read_only`, `no-new-privileges`, `cap_drop: ALL`, usuario no root, límites de memoria |
| Cadena de suministro (dependencias, imágenes, OTA) | Renovate, `govulncheck` y `pnpm audit` (app) y Trivy en CI (falla con CVE alta), imagen final `FROM scratch` o `distroless/static` con el binario estático, `go.sum` verificado con `GOFLAGS=-mod=mod` desactivado y `GONOSUMDB` vacío, code signing de EAS Update, 2FA obligatorio en GitHub, Expo, Cloudflare y el registro de dominios |
| Abuso de formularios públicos (spam de altas de empresa, contacto) | Turnstile, límite por IP, verificación de email antes de crear el registro `pending` |
| Exposición de morosos o datos de terceros | Lista de deudores solo dentro de la convocatoria y del acta; descargas registradas; empresas reciben el mínimo; push sin contenido sensible |
| Acceso indebido del personal de la plataforma | `superadmin` con 2FA desde M0, login en ruta separada, sesiones de 8 h, todas sus acciones en `audit_log`, sin acceso directo a producción salvo por *bastion* con clave y registro de sesión |

**Gestión de claves y secretos**
- `ENCRYPTION_KEY` con versión (`v1:` como prefijo del cifrado) para poder rotar re-cifrando en segundo plano. Al desplegar con Portainer sobre Docker standalone no hay `secrets` de Docker, así que llega como variable de entorno; mitigación: la app la lee al arrancar y la borra de `process.env`, el acceso a Portainer exige 2FA y rol de administrador solo para dos personas, y el `docker.sock` no se expone a ningún otro contenedor. Si el servidor pasa a Swarm o se añade un gestor de secretos (Infisical/Vault), se migra a `secrets:`.
- Credenciales de R2 por bucket y por uso: una para la API (lectura/escritura en el bucket principal), otra de solo escritura para backups, ninguna con permiso de borrar el bucket de backups.
- Rotación anual como mínimo de JWT (con `kid`), R2 y SMTP; rotación inmediata ante cualquier sospecha. Procedimiento en el runbook.
- Ningún secreto en el repositorio ni en la imagen Docker (comprobado con `gitleaks` en CI).

**Endurecimiento del servidor** (runbook, no código)
- Ubuntu LTS con `unattended-upgrades`, `ufw` (solo 22, 80, 443), SSH solo con clave y sin root, `fail2ban`, Docker con `live-restore`, journald persistente, alertas de disco y de reinicios.
- NPM: TLS 1.2+ únicamente, HSTS con `includeSubDomains; preload`, `X-Content-Type-Options`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy` mínima; bloqueo de rutas `.env`, `.git`, `/migrations`. El panel de NPM no expuesto a Internet (VPN o `ufw` por IP).

**Verificación continua**
- CI: `govulncheck`, `gosec`, `pnpm audit` (app), `gitleaks`, Trivy, Semgrep (reglas OWASP para Go y TypeScript), test de matriz de permisos, test de `DisallowUnknownFields` en todos los decoders.
- Antes del lanzamiento público (M4) y anualmente: **pentest externo** de API, web y app; DAST con OWASP ZAP baseline contra `staging` en cada release.
- `security.txt` en `DOMAIN/.well-known/` y política de divulgación responsable.
- Plan de respuesta a incidentes en el runbook: quién decide, cómo se aísla el servidor, cómo se rotan claves, plantilla de notificación a la AEPD (72 h) y a los despachos.

**Acciones por hito**: cada hito tiene una puerta de seguridad con checklist verificable y evidencia en la sección 10.1. Ningún hito se cierra sin superarla.

## 7. Arquitectura técnica

### 7.1 Stack

| Capa | Tecnología |
|---|---|
| Frontend (web + iOS + Android) | React Native con la última Expo SDK estable en el momento de iniciar (fijar la versión en M0 y no cambiarla hasta M4), Expo Router, TypeScript |
| Estado y datos | TanStack Query, Zustand |
| UI | React Native Paper con tema propio (madurez, accesibilidad y soporte web sin compilador). Tamagui descartado para no añadir curva de aprendizaje en M0 |
| Formularios | React Hook Form + Zod (los esquemas Zod se generan desde el OpenAPI) |
| Backend | **Go 1.26+** con `chi` (router), `huma` (OpenAPI 3.1 con validación de entrada generada desde los structs) y `pgx` (driver Postgres). Binario estático, imagen `distroless/static` (< 30 MB) |
| Acceso a datos | `sqlc` (SQL escrito a mano, tipos generados) + `goose` para migraciones. Sin ORM |
| Base de datos | PostgreSQL 17 (`btree_gist`, `pg_partman`, `pg_stat_statements`). Fase A: un nodo. Fase B: primaria + réplicas de lectura con PgBouncer. Fase C: Citus (Postgres distribuido por `community_id`) o particionado por rango de tenants |
| Cola y jobs | **River** v0.47.0 (su `go.mod` declara `go 1.26.0`, y desde Go 1.21 esa directiva es un mínimo duro para quien la consume: de ahí el suelo de Go 1.26+ del backend. Sigue en 0.x, sin garantía de estabilidad de API). Cola sobre Postgres, transaccional: el job se inserta en la misma transacción que el cambio) para todo lo que debe ser exactamente-una-vez (eventos de dominio, cierre de votaciones, facturas). **NATS JetStream** desde la fase B para el *fan-out* masivo (notificaciones, emails, push) donde importa el caudal más que la transacción |
| Caché y límites | **Valkey** (fork libre de Redis) desde la fase B: caché de membresías, `legal_rules`, ajustes de comunidad, rate limiting distribuido, sesiones calientes. En fase A todo esto vive en memoria del proceso y en Postgres |
| Ficheros | Cloudflare R2 (escala sin límite práctico; sin coste de salida). Descargas y miniaturas servidas por URL prefirmada directa, nunca a través de la API |
| Email | `net/smtp` + `go-mail`, plantillas `html/template` |
| Push | API HTTP de Expo Push desde Go (cliente propio de 100 líneas) |
| PDF | `maroto` v2 (sobre `gofpdf`) para actas, convocatorias, certificados e informes. Sin Chromium |
| Despliegue | Fase A: Docker Compose (Portainer) en servidor propio tras Nginx Proxy Manager. Fase B: varios servidores con Docker Swarm gestionado desde el mismo Portainer, Postgres HA con Patroni. Fase C: Kubernetes (k3s o gestionado) con CloudNativePG, autoescalado de `api` y `worker` |
| Apps nativas | EAS Build + EAS Update |
| OTP de voto y firma | Se envía por email con el mismo `internal/mail` que el resto del correo; TOTP (`otplib`) como alternativa más fuerte para quien lo active. **Sin proveedor de SMS**: decisión de fase 1, ver 11 |
| Antivirus | ClamAV en contenedor aparte, opcional por perfil de compose (es el mayor consumidor de memoria del stack; ver 7.7) |
| Observabilidad | OpenTelemetry en Go (trazas y métricas) → Prometheus + Grafana + Loki; alertas sobre los SLO. Fase A en el mismo servidor con retención corta; fase B en nodo aparte |
| CDN y borde | Cloudflare delante de todo: caché de `site` y `web` (estáticos), WAF, protección DDoS, Turnstile. Los estáticos pueden servirse desde Cloudflare Pages sin contenedor |
| Búsqueda | Postgres FTS (fase A). Meilisearch o Typesense (fase C) si el p95 de búsqueda supera el SLO |

### 7.2 Estructura del repositorio (monorepo)

```
/
├── app/                    # Expo (TypeScript)
│   ├── app/                # rutas Expo Router
│   │   ├── (auth)/         # login, recuperar, aceptar invitación
│   │   ├── (owner)/        # portal vecino
│   │   ├── (admin)/        # portal administrador
│   │   ├── (company)/      # portal empresa
│   │   ├── (superadmin)/   # operador de la plataforma (solo web)
│   │   └── _layout.tsx     # redirección por rol
│   ├── src/
│   │   ├── api/            # hooks de TanStack Query sobre el cliente generado en packages/shared
│   │   ├── components/
│   │   ├── store/          # Zustand
│   │   ├── i18n/
│   │   └── utils/
│   ├── Dockerfile.web
│   ├── app.json
│   └── eas.json
├── site/                   # web pública estática (Astro) + Dockerfile con nginx
├── api/                    # Go (módulo github.com/tu-org/vecingest)
│   ├── cmd/
│   │   └── vecingest/      # un solo binario con subcomandos: serve, worker, migrate, seed, verify-chain
│   ├── internal/
│   │   ├── http/           # router chi, middlewares (auth, membership, ratelimit, idempotency), handlers huma
│   │   ├── domain/         # servicios de dominio por módulo (incidents, meetings, receipts, ...)
│   │   ├── db/             # consultas .sql y código generado por sqlc
│   │   ├── jobs/           # workers de River y jobs periódicos
│   │   ├── legal/          # LegalRulesService y cálculo de mayorías
│   │   ├── mail/, push/, storage/ (R2), clamav/, tsa/, pdf/
│   │   └── config/         # carga y validación de variables de entorno
│   ├── migrations/         # SQL con goose (incluye la creación del rol app_rw)
│   ├── openapi/            # openapi.yaml generado por huma en build (fuente del cliente TS)
│   ├── templates/          # email (html/template) y PDF
│   ├── test/               # e2e con Postgres en Testcontainers
│   ├── Dockerfile          # multi-stage: golang:1.23 → distroless/static
│   └── go.mod
├── packages/
│   └── shared/             # cliente TS, tipos y esquemas Zod GENERADOS desde api/openapi/openapi.yaml; códigos de error
├── docs/
│   ├── runbook.md          # despliegue, backups, brechas de seguridad
│   ├── security/           # threat-model.md, gates/M0..M8.md, pentests/
│   └── legal/              # plantillas: convocatoria, acta, delegación, contrato de encargo
├── .github/
│   ├── PULL_REQUEST_TEMPLATE.md   # incluye el checklist de seguridad del punto 10 de la DoD
│   └── workflows/          # ci.yml, security.yml, deploy.yml, release-dast.yml
├── deploy/
│   └── docker-compose.dev.yml   # stack local (solo db); Portainer nunca lo toca
├── docker-compose.yml      # stack de Portainer (copia de referencia en PRD 8.1); vive en la
│                           #   raíz para que el "compose path" de Portainer se quede en su
│                           #   valor por defecto y `docker compose` cargue `.env` sin flags
├── env.example             # plantilla junto al compose; sin punto inicial a propósito
│                           #   copiándolo a `.env` sin `--env-file`
├── Makefile                # make dev, make test, make gen (sqlc + openapi + cliente TS), make lint
└── PRD.md
```

Gestor de paquetes: pnpm con workspaces para `app`, `site` y `packages/shared`; módulos de Go para `api`. El contrato entre ambos mundos es `api/openapi/openapi.yaml`: lo genera Go y lo consume TypeScript. Nadie escribe tipos a mano en los dos lados.

### 7.3 Modelo de datos

Convenciones: tablas en snake_case plural, ids UUID v7 generados en la aplicación (`github.com/google/uuid` v7; Postgres 17 no lo trae nativo; v7 es ordenable por tiempo, lo que mantiene los índices B-tree compactos a escala), `created_at` y `updated_at` en todas las tablas, borrado lógico con `deleted_at` en entidades de negocio; todas las consultas `sqlc` de lectura incluyen `deleted_at IS NULL` salvo las marcadas `_including_deleted`.

**Clave de tenant obligatoria**: toda tabla de negocio lleva `community_id` (o `office_id` / `company_id` según su dueño) aunque sea derivable por una relación, y todos los índices compuestos empiezan por esa columna. Es lo que permite en fase C distribuir por tenant (Citus) sin tocar el esquema. Las tablas de alto volumen (`notifications`, `audit_log`, `product_events`, `time_entries`, `job_runs`, `idempotency_keys`) se crean **particionadas por mes** desde M0 con `pg_partman`; `incidents`, `incident_comments`, `votes` y `receipts` se crean con `community_id` como primera columna de la clave primaria compuesta para poder particionarlas por hash de tenant en fase C.

```
users
  id, email (unique), password_hash, name, phone, phone_verified_at, locale, avatar_key,
  id_document_encrypted nullable, notification_prefs jsonb, is_superadmin bool,
  last_login_at, deleted_at

sessions
  id, user_id FK, refresh_token_hash, family_id, device_name, platform (ios | android | web),
  ip, last_used_at, expires_at, revoked_at

push_tokens
  id, user_id FK, token (unique), platform, device_id, last_seen_at

offices                         -- despachos de administración de fincas
  id, name, cif, email, phone, address, logo_key, collegiate_number nullable

office_members
  id, office_id FK, user_id FK, role (admin | admin_staff)
  unique(office_id, user_id)

communities
  id, office_id FK, parent_community_id FK nullable, name, cif, address, city, province, postal_code,
  settings jsonb, annual_budget numeric(12,2), reserve_fund numeric(12,2),
  secretary_is_office bool, last_ordinary_meeting_at, dpa_signed_at,
  transferred_from_office_id FK nullable, transferred_at

units                           -- viviendas / locales / garajes
  id, community_id FK, block, floor, door, type (flat | premises | garage | storage),
  participation_coefficient numeric(6,4), cadastral_ref
  unique(community_id, block, floor, door)

unit_members
  id, unit_id FK, user_id FK, role (owner | tenant),
  tenure (full_owner | bare_owner | usufructuary) default full_owner,
  board_role (president | vice_president | secretary) nullable, board_from, board_to,
  is_payer bool default false, iban_encrypted nullable (solo si is_payer),
  notification_address, electronic_notifications_consent_at nullable, consent_text_version,
  valid_from, valid_to
  unique(unit_id, user_id, role)
  -- como máximo un president vigente por comunidad (índice parcial único)

unit_transfers                  -- cambios de titularidad
  id, unit_id FK, from_user_id FK, to_user_id FK, transfer_date, notified_at, created_by FK

invitations
  id, community_id FK, unit_id FK, email nullable, role (owner | tenant),
  token_hash (unique), short_code_hash (unique) nullable, expires_at, accepted_at, sent_count,
  failed_attempts int

password_reset_tokens
  id, user_id FK, token_hash (unique), expires_at (1 h), used_at, requested_ip

otp_challenges                  -- verificación por OTP, firma avanzada
  id, user_id FK, purpose (phone_verify | vote | sign_minutes | sensitive_action),
  code_hash, channel (email | totp), expires_at, attempts int, verified_at

user_mfa
  user_id PK FK, totp_secret_encrypted, enabled_at, recovery_codes_hashed text[]

companies
  id, name, cif (unique), legal_name, email, phone, website, address, province, description, logo_key,
  opening_hours jsonb,
  activity_type (business | professional), is_freelancer bool, has_employees bool,
  trades text[], service_provinces text[], rating_avg numeric(3,2), rating_count int,
  status (pending | verified | suspended), verified_at

company_documents               -- PRL, seguros, certificados
  id, company_id FK, type (liability_insurance | prevention_plan | tgss_certificate |
  aeat_certificate | rnt | other), file_key, issued_at, expires_at, uploaded_by FK

company_members
  id, company_id FK, user_id FK, role (company | worker)

office_providers                -- proveedores habituales de un despacho
  id, office_id FK, company_id FK, notes
  unique(office_id, company_id)

incidents
  id, community_id FK, unit_id FK nullable, created_by FK users,
  title, description, category, priority (low | normal | high | urgent),
  status (open | assigned | in_progress | resolved | closed | rejected),
  scope (common | unit), location_text, affected_count int default 1,
  assigned_company_id FK nullable, assigned_at, accepted_at, resolved_at, closed_at,
  reopen_count int default 0, rejection_reason, max_budget numeric(10,2) nullable

incident_comments
  id, incident_id FK, user_id FK, body, is_internal bool

incident_attachments
  id, incident_id FK, comment_id FK nullable, file_key, thumb_key nullable, mime, size,
  sha256, scan_status (pending | clean | infected | skipped), uploaded_by FK

incident_events                 -- historial de cambios de estado
  id, incident_id FK, user_id FK nullable (null = sistema), from_status, to_status, note

incident_followers              -- vecinos que se suman a una incidencia
  incident_id FK, user_id FK
  primary key(incident_id, user_id)

announcements
  id, community_id FK, created_by FK, title, body, is_pinned bool,
  status (draft | pending_approval | published | expired), approved_by FK nullable,
  published_at, expires_at, recipient_count int,
  target (all | owners | block | units), target_filter jsonb

announcement_reads
  announcement_id FK, user_id FK, read_at
  primary key(announcement_id, user_id)

announcement_attachments
  id, announcement_id FK, file_key, name, mime, size

document_folders
  id, community_id FK, name, visibility (all | owners | board)

documents
  id, folder_id FK, name, file_key, mime, size, sha256, version int,
  scan_status (pending | clean | infected), uploaded_by FK

service_requests                -- peticiones de presupuesto de vecinos a empresas
  id, company_id FK, user_id FK, unit_id FK, description, address, contact_phone,
  share_contact_consent_at, status (pending | answered | accepted | rejected | done),
  quote_amount, quote_notes, answered_at, closed_at

service_request_attachments
  id, service_request_id FK, file_key, mime, size

ratings
  id, company_id FK, user_id FK, incident_id FK nullable, service_request_id FK nullable,
  score smallint check 1..5, comment, company_reply, replied_at
  unique(user_id, incident_id), unique(user_id, service_request_id)

common_areas
  id, community_id FK, name, capacity, opens_at time, closes_at time,
  slot_minutes int, max_duration_minutes int, max_advance_days int,
  max_bookings_per_unit_month int, price numeric(8,2), rules text

bookings
  id, common_area_id FK, unit_id FK, user_id FK, starts_at, ends_at,
  status (pending | confirmed | cancelled), cancelled_at, cancel_reason
  exclude using gist (common_area_id with =, tstzrange(starts_at, ends_at) with &&)
    where (status in ('pending','confirmed'))

common_area_blocks              -- franjas bloqueadas por el admin
  id, common_area_id FK, starts_at, ends_at, reason, created_by FK

agenda_requests                 -- propuestas de puntos por propietarios (art. 16.2)
  id, community_id FK, unit_member_id FK, title, description, status
  (pending | included | dismissed), meeting_id FK nullable, created_at

meetings
  id, community_id FK, title, type (ordinary | extraordinary),
  mode (in_person | hybrid | remote_only), mode_basis (none | bylaws | unanimous_consent),
  mode_basis_document_key nullable, convened_by FK,
  first_call_at, second_call_at, location, video_url nullable,
  voting_opens_at, voting_closes_at,
  status (draft | published | open | voting | closed | minutes_signed | executable),
  held_on_call (first | second) nullable, published_at, opened_at, debtors_recomputed_at,
  minutes_file_key, minutes_signed_at, minutes_amendment_file_key nullable

meeting_notifications           -- citación y remisión del acta por propietario (art. 9.1.h)
  id, meeting_id FK, unit_member_id FK, kind (call | minutes | absent_agreement),
  channel (app | email | postal | board), sent_at, delivered_at, acknowledged_at,
  failed_attempt_at nullable, board_posted_at nullable, effective_at

meeting_voters                  -- snapshot del censo al publicar; deudores recalculados al abrir
  meeting_id FK, unit_id FK, coefficient numeric(6,4), owner_user_ids uuid[],
  voting_representative_user_id FK nullable (obligatorio si hay cotitulares),
  is_debtor bool, debt_amount numeric(10,2), debt_challenged bool, debt_deposited bool,
  attended bool, attendance_mode (in_person | remote | represented) nullable,
  saved_vote bool default false
  primary key(meeting_id, unit_id)

meeting_items                   -- puntos del orden del día
  id, meeting_id FK, position int, title, description, is_votable bool,
  majority_type (one_third | majority | three_fifths | unanimity),
  legal_basis (art_17_1 | art_17_2 | art_17_3 | art_17_4 | art_17_6 | art_17_7 | art_17_12 | other),
  cost_repercutible_to_dissenters bool, private_use bool,
  absent_vote_applies bool (derivado: false si art_17_1 o private_use),
  absent_dissent_deadline nullable

meeting_item_dissents           -- discrepancias de ausentes (art. 17.8 LPH)
  id, meeting_item_id FK, unit_id FK, user_id FK, registered_at, note

votes                           -- append-only; cuenta el último por (item, unit)
  id, meeting_item_id FK, unit_id FK, cast_by FK users, on_behalf_of_delegation_id FK nullable,
  choice (yes | no | abstain), coefficient numeric(6,4), prev_hash, hash, cast_at,
  signature_evidence_id FK nullable, superseded_by FK nullable

meeting_item_tallies            -- contador materializado para resultados en vivo; se actualiza en la transacción del voto
  meeting_item_id PK FK, yes_coef, no_coef, abstain_coef, yes_count, no_count, abstain_count, votes_count, updated_at

meeting_item_results            -- congelado al cerrar (y recalculado tras el plazo de ausentes)
  meeting_item_id PK FK, yes_coef, no_coef, abstain_coef, yes_count, no_count,
  abstain_count, presumed_yes_coef, presumed_yes_count, quorum_coef, quorum_count,
  eligible_coef, eligible_count, outcome (approved | rejected | no_quorum),
  computed_at, final bool

signature_evidence              -- firma avanzada de actas, delegaciones y votos remotos
  id, user_id FK, entity, entity_id, document_hash, otp_challenge_id FK,
  otp_verified_at, ip, user_agent, tsa_token bytea nullable (RFC 3161)

chain_anchors                   -- anclaje externo de las cadenas de hash (votos, fichajes, audit)
  id, chain (votes | time_entries | audit_log), scope_id, head_hash, anchored_at,
  method (tsa_rfc3161 | email_to_board), evidence bytea

vote_delegations                -- art. 15.1: el representante puede no ser propietario
  id, meeting_id FK, from_unit_id FK, from_user_id FK,
  to_user_id FK nullable, to_person_name nullable, to_person_id_document_encrypted nullable,
  document_file_key nullable, signature_evidence_id FK nullable, revoked_at
  unique(meeting_id, from_unit_id)

receipts
  id, community_id FK, unit_id FK, debtor_user_id FK (titular en la fecha de emisión),
  concept, amount numeric(10,2), issue_date, due_date,
  status (pending | paid | returned), paid_at, file_key, external_ref

tasks
  id, company_id FK, incident_id FK nullable, community_id FK nullable,
  title, description, status (pending | in_progress | done | cancelled),
  scheduled_at, completed_at

task_assignments
  task_id FK, user_id FK
  primary key(task_id, user_id)

task_reports                    -- partes de trabajo
  id, task_id FK, user_id FK, body, hours numeric(5,2), file_keys text[]

invoices                        -- registro de facturas emitidas fuera; la plataforma no emite
  id, company_id FK, community_id FK, incident_id FK nullable,
  series, year, number int, rectifies_invoice_id FK nullable,
  issue_date, operation_date, due_date, subtotal, tax_amount,
  withholding_rate numeric(4,2) nullable, withholding_amount, total,
  status (draft | sent | accepted | paid | disputed), file_key, paid_at,
  reported_347 bool
  unique(company_id, series, year, number)

invoice_lines
  id, invoice_id FK, description, quantity, unit_price, tax_rate (21 | 10 | 4 | 0),
  tax_exempt_reason nullable, amount

time_entries                    -- registro de jornada (art. 34.9 ET), conservación 4 años
  id, company_id FK, user_id FK, clock_in, clock_out, lat, lng, note,
  edited_at, edited_by FK nullable, edit_reason nullable, row_hash

time_reports                    -- informes mensuales congelados
  id, company_id FK, user_id FK, year, month, file_key, report_hash, generated_at

notifications
  id, user_id FK, type, title, body, data jsonb, read_at, sent_push_at, sent_email_at

audit_log                       -- append-only: el rol de BD de la API no tiene UPDATE/DELETE sobre esta tabla
  id, user_id FK nullable, action, entity, entity_id, before jsonb, after jsonb, ip, request_id,
  prev_hash, hash

feature_flags
  id, key, enabled bool, scope (global | office | community), scope_id nullable, valid_from, valid_to

product_events                  -- analítica propia, sin terceros; sin PII más allá del user_id
  id, user_id FK nullable, membership_role, event, props jsonb, platform, app_version, created_at

job_runs                        -- trazabilidad de jobs del worker (7.8)
  id, job, started_at, finished_at nullable, status (running | ok | failed), error nullable

idempotency_keys                -- sustituye a Redis para Idempotency-Key; se purga a las 24 h
  user_id FK, key, request_hash, response_status, response_body jsonb, created_at
  primary key(user_id, key)

rate_limits                     -- ventana deslizante por clave (ip | user | phone); solo si hay más de una réplica
  key, window_start, count
  primary key(key, window_start)

-- Las tablas de River (river_job, river_queue, river_leader, river_client) las crea su propia migración.

legal_rules                     -- parámetros legales con vigencia, editables por superadmin
  id, key (ordinary_call_min_days=6 | second_call_min_minutes=30 |
  second_call_reconvene_max_days=8 | second_call_reconvene_min_days=3 |
  board_notice_effective_days=3 | absent_dissent_days=30 | minutes_signature_days=10 |
  debt_certificate_sale_days=7 | reserve_fund_pct=10 | meeting_docs_retention_years=5 |
  remote_only_enabled=false | ...),
  value jsonb, valid_from, valid_to nullable, source (referencia normativa)
```

Índices imprescindibles: `incidents(community_id, status)`, `incidents(assigned_company_id, status)`, `announcements(community_id, published_at desc)`, `bookings(common_area_id, starts_at)`, `notifications(user_id, read_at)`, `unit_members(user_id)`, `sessions(user_id)`, `votes(meeting_item_id, unit_id, cast_at desc)`.

Extensiones de Postgres necesarias: `btree_gist` (reservas), `pgcrypto` (opcional).

### 7.4 API

REST con prefijo `/v1`. JSON. Autenticación con `Authorization: Bearer <access_token>`. Paginación por cursor: `?cursor=&limit=` → `{ data: [], next_cursor }`. Errores con formato `{ code, message, details? }`.

Endpoints principales del MVP:

```
POST   /v1/auth/login
POST   /v1/auth/refresh
POST   /v1/auth/logout
POST   /v1/auth/forgot-password
POST   /v1/auth/reset-password
POST   /v1/auth/accept-invitation          # { token } o { short_code }
POST   /v1/auth/register-company           # alta pública de empresa (queda pending)
GET    /v1/me                              # usuario + membresías
PATCH  /v1/me
POST   /v1/me/push-tokens
DELETE /v1/me/push-tokens/:token
GET    /v1/me/sessions
DELETE /v1/me/sessions/:id
GET    /v1/me/export                       # RGPD
DELETE /v1/me                              # baja con anonimización

GET    /v1/offices/me                      # despacho del admin
GET    /v1/offices/me/members
POST   /v1/offices/me/members              # invitar admin_staff
GET    /v1/offices/me/providers
POST   /v1/offices/me/providers

GET    /v1/communities                     # según membresías
POST   /v1/communities                     # admin
GET    /v1/communities/:id
PATCH  /v1/communities/:id
GET    /v1/communities/:id/units
POST   /v1/communities/:id/units
GET    /v1/communities/:id/units/import/template   # CSV de ejemplo
POST   /v1/communities/:id/units/import    # CSV: ?dry_run=true valida sin escribir
PATCH  /v1/units/:id
GET    /v1/units/:id/members
DELETE /v1/units/:id/members/:memberId
PATCH  /v1/units/:id/members/:memberId     # cambiar rol o cargo de junta
POST   /v1/communities/:id/invitations
GET    /v1/communities/:id/invitations
DELETE /v1/invitations/:id

GET    /v1/communities/:id/incidents       # filtros: status, category, unit
POST   /v1/communities/:id/incidents
GET    /v1/incidents/:id
PATCH  /v1/incidents/:id
POST   /v1/incidents/:id/assign            # { company_id }
POST   /v1/incidents/:id/transition        # { to: status, note? }
GET    /v1/incidents/:id/comments
POST   /v1/incidents/:id/comments
POST   /v1/incidents/:id/attachments       # devuelve URL prefirmada de subida
POST   /v1/incidents/:id/attachments/:attId/confirm
POST   /v1/incidents/:id/follow
GET    /v1/incidents/:id/events            # historial
GET    /v1/companies/me/incidents          # bandeja de la empresa
POST   /v1/incidents/:id/accept            # empresa
POST   /v1/incidents/:id/decline           # empresa, motivo obligatorio

GET    /v1/communities/:id/announcements
POST   /v1/communities/:id/announcements
GET    /v1/announcements/:id
POST   /v1/announcements/:id/read
GET    /v1/announcements/:id/reads         # admin
POST   /v1/announcements/:id/approve       # admin
POST   /v1/announcements/:id/reject        # admin, comentario

GET    /v1/communities/:id/documents
POST   /v1/communities/:id/documents
GET    /v1/documents/:id/download          # URL prefirmada

GET    /v1/companies                       # directorio, filtros: trade, province, q
GET    /v1/companies/:id
GET    /v1/companies/me
PATCH  /v1/companies/me
POST   /v1/companies/:id/service-requests
GET    /v1/companies/me/service-requests
PATCH  /v1/service-requests/:id            # responder, aceptar, cerrar
POST   /v1/companies/:id/ratings
POST   /v1/ratings/:id/reply               # empresa

# fase 2 (resumen)
GET    /v1/communities/:id/meetings
POST   /v1/communities/:id/meetings         # valida art. 16 LPH
POST   /v1/communities/:id/agenda-requests  # propietario propone punto (art. 16.2)
POST   /v1/meetings/:id/publish             # snapshot censo + relación de deudores + citaciones
POST   /v1/meetings/:id/open                # recalcula deudores (art. 15.2), abre asistencia y voto
POST   /v1/meetings/:id/attendance          # admin marca asistentes y representados
POST   /v1/meetings/:id/voters/:unitId/representative   # cotitulares designan representante
POST   /v1/meetings/:id/voters/:unitId/debt-status      # impugnada / consignada
POST   /v1/meeting-items/:id/votes          # OTP en remoto; bloqueado para deudores
POST   /v1/meeting-items/:id/save-vote      # salvar el voto (art. 18.2)
POST   /v1/meeting-items/:id/dissent        # ausentes, 30 días
POST   /v1/meetings/:id/delegations
POST   /v1/meetings/:id/close               # calcula resultados
POST   /v1/meetings/:id/minutes/sign        # presidente y secretario, OTP, ≤ 10 días naturales
POST   /v1/meetings/:id/minutes/amend       # subsanación (art. 19.3)
GET    /v1/communities/:id/debt-certificate/:unitId?purpose=claim|sale   # art. 21.3 / art. 9.1.e
GET    /v1/communities/:id/exports/modelo-347?year=
GET    /v1/companies/me/time-reports?year=&month=
POST   /v1/companies/me/documents

# superadmin
GET    /v1/admin/offices
POST   /v1/admin/offices
GET    /v1/admin/companies?status=pending
POST   /v1/admin/companies/:id/verify
POST   /v1/admin/communities/:id/transfer  # { to_office_id }

GET    /v1/notifications
POST   /v1/notifications/read-all
GET    /v1/health
```

Subida de ficheros: el cliente pide `POST .../attachments` con `{ filename, mime, size }`, la API valida tipo y tamaño y devuelve `{ attachment_id, upload_url, file_key }` (URL prefirmada de PUT, 10 min, con `Content-Length` y `Content-Type` firmados para que no se pueda subir otra cosa). El cliente sube directo a R2 y llama a `.../confirm`; la API comprueba con `HeadObject` que el objeto existe, que el tamaño coincide y lee los primeros bytes para verificar el tipo real; después encola el escaneo antivirus. Los adjuntos sin confirmar se borran a las 24 h por job. La API nunca recibe el binario.

Versionado: cambios incompatibles suben a `/v2`. La app envía cabecera `X-App-Version`; la API puede responder `426 Upgrade Required` con la versión mínima cuando una app antigua deja de estar soportada.

### 7.5 Frontend: pantallas del MVP

**Auth**: login, recuperar contraseña, aceptar invitación (crea cuenta), selector de comunidad/empresa si hay varias membresías.

**Portal vecino** (tabs inferiores): Inicio (últimos avisos y estado de mis incidencias) · Incidencias (lista, detalle, nueva) · Avisos (lista, detalle) · Documentos (carpetas, visor) · Más (directorio de empresas, mi vivienda, perfil, preferencias de notificación).

**Portal administrador** (sidebar en web, drawer en móvil): Dashboard (incidencias abiertas por comunidad, avisos recientes, lectura) · Comunidades (lista, detalle con pestañas: viviendas, miembros, incidencias, avisos, documentos; en fase 2 se añaden zonas, juntas y recibos) · Incidencias (bandeja global con filtros, detalle con asignación y comentarios internos) · Proveedores · Configuración del despacho y usuarios.

**Portal superadmin** (solo web, misma app con guard): Despachos (alta, miembros) · Empresas pendientes de verificación · Transferencias de comunidad · Reglas legales (`legal_rules`) · Auditoría.

**Portal empresa** (tabs): Bandeja (incidencias asignadas y solicitudes de presupuesto) · Detalle de trabajo (fotos, comentarios, cambio de estado) · Perfil público y documentación · Equipo y fichaje (fase 2).

Componentes compartidos: `IncidentCard`, `IncidentStatusBadge`, `AttachmentPicker` (cámara/galería/fichero), `CommentThread`, `EmptyState`, `Paginated list` con pull-to-refresh y carga infinita.

### 7.6 Notificaciones: eventos del MVP

| Evento | Destinatarios |
|---|---|
| `incident.created` | Admins del despacho |
| `incident.assigned` | Empresa asignada, creador |
| `incident.status_changed` | Creador, admin, empresa según estado |
| `incident.commented` | Participantes del hilo (excepto internos → solo admin y empresa) |
| `incident.declined` | Admin (la empresa rechaza) |
| `incident.unanswered` | Admin (48 h sin respuesta de la empresa) |
| `incident.closed_auto` | Creador |
| `announcement.pending_approval` | Admins del despacho |
| `announcement.published` | Destinatarios del aviso |
| `document.uploaded` | Vecinos con visibilidad |
| `invitation.sent` | Email al invitado |
| `service_request.created` / `answered` | Empresa / vecino |
| `rating.created` / `replied` | Empresa / vecino |
| `company.verified` / `suspended` | Responsable de la empresa |
| `security.new_session` / `password_changed` | Usuario (no desactivable) |

### 7.7 Decisiones de implementación

Cosas que, si no se fijan al principio, cada módulo resuelve de forma distinta.

**Autorización y multi-tenant**
- No se usa RLS de Postgres; la autorización vive en la aplicación. Patrón único: middleware `RequireMembership(scope, roles...)` de `chi` que resuelve la membresía a partir del recurso de la ruta (`{communityId}`, `{incidentId}` → comunidad de la incidencia) con una consulta indexada y la inyecta en el `context.Context` como `Membership`. Nunca se toma la comunidad activa de una cabecera ni del body. Los handlers reciben la membresía como argumento explícito; un handler sin ella no compila para rutas con ámbito (interfaz obligatoria).
- Toda consulta `sqlc` sobre una tabla con ámbito lleva `community_id`/`office_id`/`company_id` como parámetro obligatorio; el linter propio (`make lint-scope`, un script sobre los `.sql`) falla si una consulta a esas tablas no filtra por su columna de ámbito.
- Decisión de "membresía activa" en la app: se guarda en Zustand y solo sirve para navegación; la API no la conoce.

**Consistencia y concurrencia**
- Cola transaccional: River inserta los jobs en la misma transacción de Postgres que el cambio de dominio (`river.InsertTx`). El propio job es el evento; se conserva el nombre de los eventos de dominio de la tabla 7.6 como tipos de job. En fase B, el job de River hace una sola cosa: publicar el evento en NATS JetStream; los consumidores `notifier` hacen el *fan-out* a miles de destinatarios en paralelo con *back-pressure*. Así Postgres no paga el coste de 500 M notificaciones al año.
- Idempotencia: los POST que crean recursos desde móvil (incidencias, comentarios, adjuntos, votos, reservas, fichajes) aceptan cabecera `Idempotency-Key` (UUID generado en cliente); la API guarda `(user_id, key) → response` en la tabla `idempotency_keys` (purgada a las 24 h por job) y devuelve la misma respuesta ante reintentos. Se usa `INSERT ... ON CONFLICT DO NOTHING` para que dos reintentos simultáneos no ejecuten la acción dos veces.
- Secciones críticas con `pg_advisory_xact_lock`: cadena de hash de votos (por `meeting_item_id`), numeración de facturas (por `company_id + series + year`), confirmación de adjuntos. Las reservas se protegen con el constraint de exclusión.
- Optimistic locking con `version int` en `incidents`, `meetings`, `bookings`, `announcements`; el cliente envía `If-Match`/`version` en PATCH y recibe `409 CONFLICT_STALE` si cambió (`UPDATE ... WHERE version = $n RETURNING`; si no devuelve fila, conflicto).

**API y contrato**
- OpenAPI 3.1 generado por `huma` a partir de los structs de entrada y salida de Go (con validaciones en etiquetas: `minLength`, `pattern`, `enum`). De ese `openapi.yaml` se generan en CI el cliente TypeScript (`openapi-typescript` + `openapi-fetch`) y los esquemas Zod para los formularios (`openapi-zod-client`), en `packages/shared`. Ningún fetch a mano y ningún tipo duplicado.
- Cursor de paginación: base64url de `(sort_value, id)`; orden estable por `created_at desc, id desc` salvo indicación.
- Endpoints de salud separados: `/health/live` (proceso) y `/health/ready` (BD, R2). Docker usa `ready`; hacia fuera solo se expone `/health/live` con respuesta `200 {"ok":true}` sin detalle de dependencias.
- Cabeceras obligatorias desde la app: `X-App-Version`, `X-Platform`, `X-Request-Id` (UUID generado en cliente, validado por formato y longitud; si falta o no es válido, lo genera el servidor; se usa solo para correlación, nunca para decisiones de seguridad).
- Rate limiting con `httprate` detrás de una interfaz `Limiter`: en fase A, memoria del proceso; en fase B, Valkey (`httprate-redis`), sin tocar los handlers. La IP real se toma de `X-Forwarded-For` **solo** si la petición llega desde la IP del contenedor de NPM (`trust proxy` configurado con esa IP, no con `true`); de lo contrario cualquier cliente podría falsear su IP y saltarse los límites.
- Markdown de avisos y comentarios sanitizado en servidor (`goldmark` para renderizar + `bluemonday` con lista blanca) y renderizado en la app con `react-native-markdown-display`. Nunca HTML crudo.
- Búsqueda: `tsvector` en Postgres con configuración `spanish` para incidencias, avisos, documentos y directorio; sin Elasticsearch.

**Procesos**
- Un solo binario estático (`vecingest`) con subcomandos: `serve` (HTTP), `worker` (River + jobs periódicos), `notifier` (consumidor de NATS para push y email, fase B), `migrate`, `seed`, `verify-chain`. Todos los procesos son **sin estado**: se pueden lanzar N réplicas de `serve` y `notifier` sin coordinación; los jobs periódicos usan la elección de líder de River para ejecutarse una sola vez aunque haya varios `worker`. El worker hace shutdown graceful (termina el job en curso, máximo 30 s). Nunca se ejecutan jobs dentro del proceso HTTP. Presupuesto: cada proceso < 128 MB en reposo (`GOMEMLIMIT` fijado al 80 % del `mem_limit` del contenedor para que el GC respete el límite).
- Validación de variables de entorno al arrancar (`internal/config` con `envconfig` y validación explícita); si falta una, el proceso no arranca y lo dice.
- Migraciones SQL con `goose` (se ejecutan con `vecingest migrate` al arrancar `api`), con estrategia expand/contract para no cortar servicio: añadir columna → desplegar código que escribe en ambas → migrar datos → eliminar columna en el siguiente release. Nunca `DROP` o `RENAME` en la misma release que el código que deja de usarlo.
- Logs con `slog` en JSON y un handler que redacta PII (`email`, `phone`, `iban`, `id_document`, `token`, `authorization`).
- Rotación de claves JWT con `kid` en la cabecera y dos claves activas (`JWT_SECRET` actual y `JWT_SECRET_PREVIOUS`).

**App (Expo)**
- Rutas protegidas: `_layout.tsx` raíz hidrata la sesión desde `expo-secure-store` antes de renderizar; mientras tanto muestra splash. Si llega un deep link sin sesión, se guarda la ruta destino y se restaura tras el login.
- EAS Update con dos canales (`staging`, `production`) y `runtimeVersion` con política `appVersion`; cualquier cambio en dependencias nativas obliga a nuevo build de tienda. **Code signing de `expo-updates` activado** con clave privada fuera de EAS: una cuenta de Expo comprometida no puede empujar código a los usuarios.
- Permiso de notificaciones: se pide tras la primera acción con valor (crear incidencia o abrir un aviso), nunca en el primer arranque. El payload de push lleva siempre `{ type, entity, id, url }` y la app navega con `url`.
- Imágenes: `expo-image` con caché en disco. El cliente genera y sube dos objetos por foto: original comprimido (≤ 1920 px) y miniatura (300 px); R2 no transforma imágenes y no se quiere depender de Cloudflare Images. EXIF eliminado en cliente.
- Descargas: URL prefirmada de 5 minutos → `expo-file-system` → visor nativo o `expo-sharing`. En web, `window.open`. Sin *certificate pinning* (rompe con la rotación de Let's Encrypt); la defensa es TLS + HSTS.
- Sentry con `sendDefaultPii: false`, sin cuerpos de petición y con *scrubbing* de email, teléfono e IBAN. Source maps subidos a Sentry, nunca publicados en `web`.
- Expo web: CSP `default-src 'self'; connect-src 'self' https://api.DOMAIN https://*.r2.cloudflarestorage.com; img-src 'self' data: https://*.r2.cloudflarestorage.com; frame-ancestors 'none'`, sin `unsafe-inline` (Expo export no necesita scripts inline si se configura `web.output = 'static'` y se evita `dangerouslySetInnerHTML`).
- Offline: cola de acciones pendientes (`outbox` local en SQLite vía `expo-sqlite`) para crear incidencias y comentarios; reintento con backoff al recuperar red; conflictos resueltos por el servidor con `Idempotency-Key`.
- Formularios: React Hook Form + `zodResolver` usando los mismos esquemas de `packages/shared`; los mensajes de error se traducen por código, no por texto.
- Tiempo real: no hay WebSockets en el MVP; TanStack Query con `refetchInterval` de 30 s en pantallas de detalle abiertas. En fase 2, SSE (`/v1/meetings/:id/stream`) para resultados de votación en juntas híbridas.
- Tiendas: Apple exige borrado de cuenta dentro de la app (existe en `/me`) y, si en fase 2 se añade login con Google, obliga a ofrecer también Sign in with Apple. Preparar "privacy nutrition labels" y "Data safety" de Google con la lista de datos de la sección 4.4.

**Importación de datos**
**Caché**
- Tres niveles, siempre con invalidación explícita y nunca con datos personales en caché compartida sin cifrar: (1) HTTP: `ETag` y `Cache-Control: private, max-age=30` en listados y `immutable` en ficheros de R2; la app usa TanStack Query con `staleTime` por recurso. (2) Proceso: `legal_rules`, ajustes de comunidad y plantillas, invalidados por `LISTEN/NOTIFY`. (3) Valkey (fase B): membresías por usuario (la consulta más repetida de la plataforma, en cada petición), contadores de no leídos, resultados de votación en vivo. TTL corto y clave con versión de tenant para invalidar de golpe.
- Lecturas pesadas (paneles de despacho, exportaciones, modelo 347) van a réplica de lectura desde fase B mediante un `ReadDB` distinto del `WriteDB` en el código desde M0, aunque en fase A apunten al mismo nodo.

**Recursos y ClamAV**
- ClamAV es, con diferencia, el proceso que más memoria consume del stack (1–1,5 GB por las firmas en memoria). Se mantiene por la puerta de seguridad M2, pero como perfil opcional del compose (`--profile av`) y con dos alternativas documentadas si el servidor no lo soporta: (a) `clamd` con `ConcurrentDatabaseReload no` y `--max-memory` para bajar a ~700 MB; (b) escaneo diferido: los documentos quedan `pending` y un job nocturno levanta ClamAV, escanea la cola y lo apaga. En ambos casos las imágenes (JPEG/PNG/WebP/HEIC) se validan por magic bytes y se recodifican en cliente, y no pasan por ClamAV; solo los PDF y Office.
- Postgres: `shared_buffers` 256 MB, `work_mem` 8 MB, `max_connections` 40 con `pgx` pool de 10 por proceso (bajar a 5 si se quiere apurar). `pg_stat_statements` activado para localizar consultas pesadas. Suficiente para miles de comunidades en un solo servidor.
- **PgBouncer por fases.** Fase A (dos procesos, pools persistentes): no aporta y rompería `LISTEN/NOTIFY`. Fase B (varias réplicas de `serve`): PgBouncer en modo *transaction* delante de la primaria y de cada réplica, solo para `serve`, con `pgx` en `QueryExecModeCacheDescribe` (compatible con pooling transaccional); `worker` y `notifier` mantienen conexión directa para River y `LISTEN/NOTIFY`. El código se escribe desde M0 con esa separación (`ServeDB` con pooler, `WorkerDB` directo) para que el cambio sea de configuración.

**Importación de datos**
- Los CSV de los programas de fincas suelen venir en Latin-1 con `;` como separador y decimales con coma. El importador detecta codificación (`chardet`), acepta `;` y `,`, normaliza decimales y muestra una previsualización de 20 filas antes de confirmar.

**Herramientas de repositorio**
- `Makefile` como punto de entrada único (`make dev`, `make gen`, `make test`, `make lint`). Go: `golangci-lint` (con `gosec`, `errcheck`, `sqlclosecheck`), `gofumpt`, `go test -race`. App: pnpm workspaces + Turborepo, ESLint, Prettier, `tsc --noEmit`. `lefthook` para hooks de pre-commit en ambos. Renovate para dependencias con agrupación semanal.
- Tests de la API: unitarios en `internal/domain` (tablas de casos, `testify`), e2e con Postgres real en Testcontainers (`make test-e2e`), y tests de la matriz de permisos generados desde el `openapi.yaml` (cada ruta × cada rol × recurso ajeno).
- Tests E2E de móvil con Maestro sobre development builds para los 5 flujos críticos (login, crear incidencia, aceptar invitación, leer aviso, votar); se ejecutan antes de cada release de tienda, no en cada PR.

### 7.8 Jobs programados (proceso `worker`)

| Job | Frecuencia | Qué hace |
|---|---|---|
| `incidents.auto-close` | cada hora | `resolved` → `closed` pasados `auto_close_days` |
| `incidents.unanswered` | cada hora | Aviso al admin si la empresa no responde en 48 h |
| `attachments.cleanup` | diario 03:00 | Borra adjuntos sin confirmar > 24 h en BD y R2 |
| `invitations.expire` | diario | Marca invitaciones caducadas (14 días) |
| `announcements.expire` | cada hora | `published` → `expired` |
| `announcements.scheduled` | cada minuto | Publica avisos programados |
| `meetings.open` / `meetings.close-voting` | cada minuto | Abre juntas (recalcula deudores) y cierra ventanas de voto |
| `meetings.absent-dissent-expire` | diario | Vence plazos de 30 días y recalcula resultados |
| `meetings.minutes-reminder` | diario | Recuerda firma del acta (10 días) y juntas ordinarias pendientes (11 meses) |
| `board.term-expiry` | diario | Avisa 30 días antes de vencer un cargo |
| `push.receipts` | cada 15 min | Consulta receipts de Expo y purga tokens inválidos |
| `files.scan` | continuo (cola River) | ClamAV sobre PDF y Office recién confirmados (o diferido nocturno si el perfil `av` no está activo) |
| `chains.anchor` | diario 04:00 y al cerrar cada junta | Sella `head_hash` de `audit_log`, `time_entries` y `votes` con TSA RFC 3161 |
| `secrets.expiry-check` | semanal | Avisa de credenciales y certificados con más de 11 meses |
| `partitions.maintain` | diario | `pg_partman` crea particiones futuras y desmonta las que superan la retención; archiva a R2 |
| `slo.report` | diario | Calcula p95, disponibilidad y caudal del día y avisa si un SLO lleva 3 días fuera de objetivo (disparador de fase) |
| `idempotency.purge` | cada hora | Borra claves de `idempotency_keys` con más de 24 h |
| `retention.apply` | semanal | Anonimiza fotos de incidencias > 3 años, purga logs > 1 año |
| `company-docs.expiry` | diario | Avisa de documentos PRL/seguros a 30 días de caducar |
| `backup.postgres` | diario 02:00 (cron del host) | `pg_dump` cifrado a `R2_BACKUP_BUCKET` |

Todos los jobs son idempotentes, se ejecutan como jobs de River (los periódicos con `PeriodicJob`) y registran inicio, fin y errores en `job_runs` (tabla simple: `job, started_at, finished_at, status, error`) para poder ver desde el panel de superadmin si algo lleva días sin ejecutarse.

### 7.9 Diseño visual

Referencia para React Native Paper (tema propio) y para la web pública. Los wireframes de esta sección se han validado en mockups de las pantallas de vecino, administrador, empresa y votación.

**Identidad**
- Nombre: **Vecingest**. Wordmark en minúsculas ("vecingest"), con la "g" como único descendente y sin ligaduras; símbolo: tres líneas horizontales de distinto largo que sugieren plantas de un edificio. Se entrega en SVG monocromo (funciona en azul, blanco y negro).
- Tono de la interfaz: gestión seria, sin adornos. Ninguna ilustración decorativa en pantallas de trabajo; sí en estados vacíos y en la web pública.
- Textos en castellano, frase (no Título), sin signos de exclamación, sin "por favor", verbo primero en botones ("Enviar incidencia", "Asignar", "Fichar salida").

**Color**
- Acento único: azul `#185FA5` (botón primario, tab activa, enlaces). Nunca más de un botón primario por pantalla.
- Neutros cálidos para superficies: fondo `#F1EFE8` (claro) / `#2C2C2A` (oscuro), tarjetas blancas / `#444441`.
- Estados de incidencia, siempre con el mismo color en las tres apps: abierta gris, asignada azul, en curso morado `#534AB7`, resuelta verde `#0F6E56`, cerrada gris oscuro, rechazada rojo `#A32D2D`.
- Prioridades: urgente rojo, alta ámbar `#BA7517`, normal gris, baja gris claro.
- Resultados de votación: sí verde `#1D9E75`, no coral `#D85A30`, abstención gris `#B4B2A9`. Privados de voto en rojo con icono de prohibido.
- Texto sobre fondos de color: siempre el tono 800/900 de la misma familia, nunca negro.
- Modo oscuro obligatorio desde M0; los colores anteriores tienen su par oscuro en el tema.

**Tipografía**
- Sistema (SF en iOS, Roboto en Android, Inter en web). Dos pesos: 400 y 500. Tamaño base 16, secundario 13, metadatos 11. Nada por debajo de 11.
- Cifras legales (cuotas, porcentajes, importes) con tabular-nums para que alineen en tablas.

**Componentes y patrones**
- Navegación: tabs inferiores en móvil (vecino: Inicio, Incidencias, Avisos, Documentos, Más; empresa: Bandeja, Tareas, Fichaje, Facturas, Perfil); barra lateral en web para admin y superadmin.
- Tarjeta de incidencia: título, comunidad/ubicación, tiempo relativo, chip de estado a la derecha. Es el mismo componente en las tres apps.
- Formularios: etiqueta encima del campo, error debajo en rojo, un solo botón primario al final; detección de duplicados como aviso en línea, no como modal.
- Chips de contexto legal en juntas: apartado del art. 17 y mayoría exigida siempre visibles junto al punto; "coste solo a quien vote sí" cuando aplica.
- Resultados de votación: dos barras (cuotas y cabezas) con el umbral escrito, censo desglosado (presentes, ausentes, privados de voto) y plazo del art. 17.8 cuando hay ausentes.
- Fichaje: botón circular grande (verde entrada / rojo salida), contador de jornada y resumen semanal con aviso cuando se superan 9 h.
- Bandeja de empresa: la incidencia urgente sin aceptar lleva borde rojo y botones Aceptar/Rechazar en la propia tarjeta.
- Estados vacíos como invitación ("Crea tu primera incidencia"), nunca "No hay datos".
- Accesibilidad: contraste AA, objetivos táctiles ≥ 44 px, `accessibilityLabel` en iconos, soporte de tamaño de fuente dinámico, sin información transmitida solo por color (los chips llevan texto).

**Web pública** (`site/`)
- Landing con tres entradas de login (vecino, administrador, empresa), una página por perfil, formulario de alta de empresa, blog, contacto, aviso legal, privacidad y cookies. Misma paleta; tipografía Inter; imágenes reales de edificios, no ilustraciones genéricas.

### 7.10 Arquitectura de escala por fases

Tres fases con los mismos binarios y el mismo esquema. El paso de fase lo deciden las métricas de los SLO (sección 6), no el calendario. Lo que cambia entre fases es infraestructura y configuración; nunca el código de dominio.

| | Fase A · arranque | Fase B · crecimiento | Fase C · escala nacional |
|---|---|---|---|
| Usuarios | hasta ~200.000 (≈10.000 comunidades) | hasta ~1,5 M (≈75.000 comunidades) | 5 M+ (250.000+ comunidades) |
| Servidores | 1 (Portainer, Compose) | 3–6 nodos Docker Swarm desde Portainer; Postgres en nodo propio | Kubernetes (k3s propio o gestionado) en la UE; autoescalado |
| API | 1 `serve` | N `serve` tras Traefik o NPM en Swarm; PgBouncer | N `serve` autoescalados por CPU y p95; PgBouncer por réplica |
| Postgres | 1 nodo, 768 MB–4 GB | primaria + 1–2 réplicas de lectura (streaming), Patroni para failover, 16–64 GB | Citus (distribución por `community_id`) o particionado por rango de tenants; CloudNativePG |
| Cola | River en Postgres | River + NATS JetStream (3 nodos) para *fan-out* | Igual, con NATS en clúster dedicado |
| Caché | memoria de proceso | Valkey (1 nodo + réplica) | Valkey en clúster |
| Ficheros | R2 | R2 | R2 (varios buckets por región de tenants si hace falta) |
| Estáticos | nginx en contenedor | Cloudflare Pages / caché de Cloudflare | Igual |
| Observabilidad | Prometheus + Grafana + Loki en el mismo servidor, retención 7 días | nodo aparte, retención 30 días, alertas a guardia | igual + trazas muestreadas (Tempo) |
| Coste orientativo | 0–60 €/mes | 400–900 €/mes | 3.000–8.000 €/mes |
| Disparador para pasar | p95 > SLO de forma sostenida, CPU > 60 % en hora punta, o > 8.000 comunidades | p95 > SLO con réplicas al máximo, primaria > 70 % CPU, o > 60.000 comunidades | — |

**Qué está preparado desde M0 para que el paso de fase no toque código**
1. Procesos sin estado; sesiones, idempotencia y límites fuera del proceso o detrás de una interfaz.
2. `ServeDB` / `WorkerDB` y `ReadDB` / `WriteDB` separados en configuración.
3. Clave de tenant en todas las tablas e índices; tablas de alto volumen particionadas por mes.
4. Eventos de dominio como jobs con nombre estable; el consumidor puede ser River (A) o NATS (B) sin cambiar el productor.
5. Interfaces `Cache`, `Limiter`, `Queue`, `Search` con implementación en memoria/Postgres (A) y distribuida (B/C).
6. Un solo binario con subcomandos: añadir réplicas es repetir un servicio del compose o del manifiesto.
7. Migraciones expand/contract y `goose` con bloqueo: se pueden desplegar con N réplicas vivas.

**Puntos calientes conocidos y cómo se resuelven**
- **Tardes de junta**: miles de comunidades votando a la vez entre las 19:00 y las 21:00. La votación es escritura ligera (un `INSERT` por voto con advisory lock por punto), y el resultado en vivo se calcula con un contador materializado en Valkey (fase B) o en una tabla `meeting_item_tallies` actualizada en la misma transacción (fase A) en vez de contar votos cada vez. El cierre de un punto con 5.000 votantes recorre la cadena de hash una vez y en segundo plano.
- **Publicar un aviso a 250.000 comunidades** (comunicación de la plataforma): *fan-out* por NATS con 50 consumidores; Postgres solo guarda un `announcement` y las lecturas se registran de forma agregada por lotes.
- **Bandeja del administrador con 2.000 comunidades**: índice compuesto `(office_id, status, created_at desc)` y paginación por cursor; nunca `OFFSET`. Réplica de lectura.
- **Fotos**: la API nunca toca los bytes (subida y descarga directas a R2 con URL prefirmada); miniaturas generadas en cliente. A 50 M ficheros el único coste es el índice `(community_id, entity, entity_id)`.
- **Auditoría y notificaciones**: particionadas por mes; las particiones antiguas se desmontan (`DETACH`) y se archivan en R2 como Parquet según la política de retención, en vez de borrar fila a fila.
- **Búsqueda**: FTS en Postgres con índice GIN por comunidad hasta fase B; Meilisearch por tenant en fase C.
- **Reglas legales**: caché en proceso invalidada por `NOTIFY`; una petición nunca consulta `legal_rules`.

**Pruebas de carga** (obligatorias en cada puerta desde M2): `k6` contra `staging` con el escenario "tarde de junta" (1.000 comunidades votando, 50 votos/s sostenidos durante 10 min) y "lunes de incidencias" (200 incidencias/min con fotos). Los resultados se archivan en `docs/security/gates/`. Un PR que empeore el p95 de un endpoint crítico más de un 10 % no se fusiona.

**Lo que no se hace por adelantado**: Kubernetes, Citus, NATS y Valkey en fase A. Añadirlos antes de tiempo multiplica la superficie de ataque, el coste y el tiempo de operación sin ninguna ganancia medible. El diseño garantiza que añadirlos después es un cambio de infraestructura, no de código.

## 8. Infraestructura y despliegue

- **Fase A** (la que describe esta sección): servidor propio con Docker Compose desplegado como **stack de Portainer** (Docker standalone). Las fases B y C están en 7.10; Portainer gestiona también Swarm, así que el paso a fase B no cambia de herramienta. Portainer no construye imágenes: CI las publica en GHCR y el stack solo hace `pull`. Las variables se definen en Portainer (Environment variables); se sustituyen en `${VAR}` y llegan a los contenedores mediante `env_file: stack.env`, que Portainer genera automáticamente. Actualizar = cambiar `TAG` y "Re-pull image and redeploy". Servicios: `web` (nginx con el export de Expo), `site` (nginx con la web pública estática, generada con Astro), `api` (binario Go, `vecingest serve`), `worker` (misma imagen, `vecingest worker`), `db` (Postgres 17) y, opcional por perfil, `clamav`. Sin Redis. Ficheros en Cloudflare R2. Imagen de la API `distroless/static` con binario estático (< 30 MB), usuario no root, `HEALTHCHECK` en `api` y `worker`. Consumo objetivo del stack en reposo: api 40 MB + worker 40 MB + Postgres 300 MB + nginx ×2 10 MB ≈ 400 MB; con ClamAV, +1 GB.
- Proxy inverso existente: nginx corre como **contenedor** en Portainer y hace de proxy inverso de todos los stacks del servidor; se configura **a mano**, apuntando a puertos publicados en el host, no a una red docker compartida. Nuestro stack publica esos puertos para que nginx los use como destino: `DOMAIN → host:SITE_PORT`, `app.DOMAIN → host:WEB_PORT`, `api.DOMAIN → host:API_PORT` (con `client_max_body_size 1m` en el host de la API: la API nunca recibe binarios). **`site` y `web` escuchan en 8080 dentro del contenedor, no en 80**: sus imágenes se construyen sobre `nginxinc/nginx-unprivileged` y corren como usuario no root, porque la imagen oficial de nginx arranca como root y no cumple el punto de contenedores no root de 10.1; por eso el puerto publicado en el host mapea a 8080, no a 80, dentro del contenedor.
- Variables de entorno (plantilla en `env.example`, valores en Portainer, nunca en el repositorio): `DOMAIN`, `APP_URL`, `APP_ENV` (`development` | `staging` | `production`), `CORS_ORIGINS`, `POSTGRES_*` (superusuario, solo para el arranque de roles), `APP_DB_USER`, `APP_DB_PASSWORD`, `BOOTSTRAP_DATABASE_URL`, `MIGRATIONS_DATABASE_URL`, `JWT_SECRET`, `JWT_SECRET_PREVIOUS`, `JWT_REFRESH_SECRET`, `ENCRYPTION_KEY` (32 bytes en base64; en Portainer va como variable de entorno porque los secretos por fichero no están disponibles en Docker standalone; ver mitigación en 6.1), `R2_ACCOUNT_ID`, `R2_BUCKET`, `R2_BACKUP_BUCKET`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `SMTP_URL`, `MAIL_FROM`, `TURNSTILE_SECRET`, `TSA_URL`, `EXPO_ACCESS_TOKEN`, `SENTRY_DSN`, `PROXY_IP` (dirección que la API observa como origen de la petición de nginx, para `trust proxy`), `SITE_PORT`, `WEB_PORT` y `API_PORT` (puertos publicados en el host para que nginx los use como destino). El superadmin inicial no figura aquí: se crea con `vecingest bootstrap-superadmin` y sus credenciales no se guardan como variables del stack (ver 5.1).
- Entornos: `development` (local con `docker compose -f docker-compose.dev.yml`, solo db; API con `air` para recarga y app en el host), `staging` (opcional, mismo compose con otro `.env` y subdominios `staging-*`), `production`.
- Datos de prueba: `vecingest seed` crea un despacho, dos comunidades con viviendas, una empresa verificada y usuarios de cada rol con contraseña conocida (solo si `APP_ENV != production`).
- Migraciones: `vecingest migrate` se ejecuta en el arranque del contenedor `api` (con bloqueo de aviso en Postgres para que `worker` no arranque a la vez). La creación del rol `app_rw` sin `UPDATE`/`DELETE` en tablas append-only es una migración `goose`, no un script de `docker-entrypoint-initdb.d`, para no depender de bind mounts que Portainer no tiene. Las migraciones corren con el rol propietario; `api` y `worker` se conectan como `app_rw`.
- Backups: cron diario con `pg_dump | age -r <clave pública>` a `R2_BACKUP_BUCKET` con credenciales de solo escritura (`R2_BACKUP_ACCESS_KEY_ID`/`SECRET`), versionado activado y retención 30 días. La clave privada de `age` no está en el servidor. Restauración probada el primer lunes de cada mes en `staging`.
- Apps nativas: `eas build --profile production`; actualizaciones JS sin pasar por tienda con `eas update`.
- CI (GitHub Actions): `golangci-lint`, `go test -race`, `make gen` con comprobación de que `openapi.yaml`, el código `sqlc` y el cliente TS están al día (falla si `git diff` no está limpio), lint y typecheck de la app, en cada PR; en merge a `main` construye `vecingest-api`, `vecingest-web` y `vecingest-site`, las escanea con Trivy y las publica en GHCR con etiqueta de versión y `latest`; después llama al webhook de redespliegue del stack de Portainer (Stack → Webhook). La web se construye con `EXPO_PUBLIC_API_URL` como `build-arg` desde un secreto de GitHub, porque Expo lo incrusta en el estático. Las apps nativas se publican manualmente con EAS desde `main` etiquetado.

### 8.1 Stack de Portainer de referencia (`docker-compose.yml`)

Este es el stack que despliega Portainer. Se mantiene en el repositorio en la raíz, `docker-compose.yml`; esta copia es de referencia y debe actualizarse si cambia aquel. El fichero vive en la raíz (no en `deploy/`) por dos motivos: el campo *compose path* de Portainer se queda en su valor por defecto sin nada que configurar, y `docker compose` carga automáticamente un `.env` situado junto al compose, así que copiar `env.example` a `.env` en la raíz basta para la sustitución de `${VAR}` sin `--env-file`. La plantilla se llama `env.example`, sin punto inicial, a propósito: no contiene secretos —solo nombres, valores por defecto y comentarios— y así queda fuera de los guardas que bloquean rutas `.env*`. `deploy/` conserva solo `docker-compose.dev.yml`, el stack local que Portainer nunca toca.

**El stack se crea en Portainer como "Repository"**, apuntando al repositorio de GitHub con `docker-compose.yml` (en la raíz) como *compose path* — el valor por defecto de Portainer, sin nada que configurar —, no pegando el YAML a mano. Así la definición del stack vive versionada en git y no se puede desincronizar de lo que hay en el servidor. El despliegue lo dispara el webhook del stack, que `deploy.yml` llama al final de cada push a `main` que haya pasado CI y la puerta de seguridad; el equivalente manual es "Pull and redeploy". Portainer solo lee el compose del repositorio: **las imágenes las sigue construyendo y publicando CI en GHCR**, y Portainer las descarga (ver 8.1.1).

```yaml
# Stack para Portainer (Docker standalone, no Swarm).
# - Las variables se definen en Portainer > Stack > Environment variables.
#   Portainer las sustituye en ${VAR} y además las vuelca en un fichero `stack.env`
#   que se pasa a los contenedores con `env_file`. No hace falta listar cada variable.
# - El stack se despliega desde el repositorio de GitHub (Portainer > Add stack >
#   Repository, compose path por defecto `docker-compose.yml` en la raíz), no
#   pegando este YAML.
# - Las imágenes se construyen en CI (GitHub Actions) y se publican en GHCR.
#   Portainer solo hace pull; el webhook del stack o "Pull and redeploy" actualiza.
#   Portainer NO construye imágenes: ningún servicio declara `build:`, y hacerlo
#   saltaría el escaneo de Trivy previo a la publicación (ver 8.1.1).
# - Plantilla de variables: ver env.example
#
# Proxy inverso: el servidor corre un único contenedor nginx en Portainer
# que hace de proxy inverso de todos los stacks de la máquina, configurado
# A MANO contra puertos publicados en el host -- no hay ninguna red docker
# compartida a la que unirse. `site`, `web` y `api` publican por eso
# `SITE_PORT`/`WEB_PORT`/`API_PORT` en el host. La publicación se hace en
# todas las interfaces (0.0.0.0), porque el contenedor de nginx llega a
# este stack por la dirección del host, no por una red docker compartida.
# POR ESO EL CORTAFUEGOS DEL HOST DEBE CERRAR SITE_PORT, WEB_PORT Y
# API_PORT DESDE FUERA: con la red compartida anterior nunca eran
# alcanzables desde Internet; publicados así lo son, salvo que ufw los
# bloquee explícitamente.
#
# Los puertos por defecto son 24221 (site), 38043 (web) y 34246 (api),
# elegidos por el operador porque este servidor corre varios stacks
# detrás del mismo nginx y 8080/8081/3000 seguramente ya están ocupados.
# NO son contiguos, así que LA REGLA DE CORTAFUEGOS QUE LOS CIERRA DEBE
# SER TRES REGLAS DE PUERTO SEPARADAS (24221, 38043, 34246) -- una regla
# de rango no cubre las tres, y quien la escriba creerá que dos quedan
# cerradas cuando no es así.
#
# 38043 y 34246 caen dentro del rango efímero por defecto de Linux
# (`net.ipv4.ip_local_port_range`, normalmente 32768-60999): el kernel
# puede asignar cualquiera de los dos como puerto de origen de una
# conexión saliente, y si lo hace antes de que Docker lo reserve, el
# contenedor falla al arrancar con "address already in use" -- de forma
# intermitente, típicamente solo tras un reinicio o un redespliegue.
# Reservar ambos en el host para que solo Docker los use:
#   # /etc/sysctl.d/99-vecingest-reserved-ports.conf
#   net.ipv4.ip_local_reserved_ports = 34246,38043
#   luego: sysctl --system
# 24221 está por debajo de 32768 y no necesita esta reserva.
#
# Antes de desplegar, comprobar que los tres puertos están libres:
#   ss -ltnp | grep -E ':(24221|38043|34246)\b'
# (sin salida significa libre).

x-hardening: &hardening
  read_only: true
  security_opt: [no-new-privileges:true]
  cap_drop: [ALL]
  tmpfs: [/tmp]

x-app-image: &app-image
  image: ${REGISTRY:-ghcr.io/tu-org}/vecingest-api:${TAG:-latest}
  # `stack.env` lo escribe Portainer al desplegar; no existe si este
  # fichero se ejecuta a mano (`docker compose up` en la raíz). Compose
  # trata una entrada `env_file` que falta como error, así que se usa la
  # forma larga con `required: false` -- si no, un `docker compose up`
  # manual falla antes de arrancar nada con "env file ... not found".
  env_file:
    - path: stack.env
      required: false
  environment:
    APP_ENV: production
    PORT: 3000
    APP_URL: https://app.${DOMAIN}
    CORS_ORIGINS: https://app.${DOMAIN},https://${DOMAIN}
    # Tres roles distintos. `${POSTGRES_USER}` es superusuario y solo se usa una vez, en el
    # arranque de roles; `vecingest_owner` aplica las migraciones de goose; `app_rw` es el
    # runtime y no posee ninguna tabla. Conectar la API como superusuario haría imposible
    # el punto del rol restringido de 10.1. `MIGRATIONS_DATABASE_URL` y
    # `BOOTSTRAP_DATABASE_URL` llegan por stack.env.
    DATABASE_URL: postgres://${APP_DB_USER}:${APP_DB_PASSWORD}@db:5432/${POSTGRES_DB}
      CLAMAV_HOST: clamav
    S3_ENDPOINT: https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com
    S3_REGION: auto
    S3_BUCKET: ${R2_BUCKET}
    S3_ACCESS_KEY: ${R2_ACCESS_KEY_ID}
    S3_SECRET_KEY: ${R2_SECRET_ACCESS_KEY}
    TSA_URL: ${TSA_URL:-https://freetsa.org/tsr}

services:
  site:                           # web pública estática (Astro) → DOMAIN, proxiada a mano por nginx
    image: ${REGISTRY:-ghcr.io/tu-org}/vecingest-site:${TAG:-latest}
    restart: unless-stopped
    <<: *hardening
    tmpfs: [/tmp, /var/cache/nginx, /var/run]
    user: "1000:1000"             # La imagen oficial de nginx arranca como root y no cumple el
                                  # punto de contenedores no root de 10.1. Esta imagen se
                                  # construye sobre nginxinc/nginx-unprivileged y escucha en
                                  # 8080, no en 80: el puerto publicado abajo mapea a 8080
                                  # dentro del contenedor, y nginx debe apuntar a host:SITE_PORT.
    mem_limit: 128m
    ports:
      - "${SITE_PORT:-24221}:8080"

  web:                            # app Expo exportada → app.DOMAIN, proxiada a mano por nginx
    image: ${REGISTRY:-ghcr.io/tu-org}/vecingest-web:${TAG:-latest}
    restart: unless-stopped
    <<: *hardening
    tmpfs: [/tmp, /var/cache/nginx, /var/run]
    user: "1000:1000"             # Mismo motivo que `site`: nginx-unprivileged, escucha en
                                  # 8080, no en 80.
    mem_limit: 128m
    ports:
      - "${WEB_PORT:-38043}:8080"

  api:                            # binario Go → api.DOMAIN, proxiada a mano por nginx
    <<: [*app-image, *hardening]
    restart: unless-stopped
    user: "65532:65532"             # nonroot de distroless
    mem_limit: 256m
    environment:
      GOMEMLIMIT: 200MiB
    command: ["/vecingest", "serve", "--migrate"]
    healthcheck:
      test: ["CMD", "/vecingest", "health", "--ready"]   # el binario hace la petición; distroless no tiene wget
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    depends_on:
      db: { condition: service_healthy }
    networks: [internal]          # solo para llegar a db; para que nginx la alcance usa el
                                  # puerto publicado, no una red compartida.
    ports:
      - "${API_PORT:-34246}:3000"

  worker:                         # River + jobs periódicos (misma imagen)
    <<: [*app-image, *hardening]
    restart: unless-stopped
    user: "65532:65532"
    mem_limit: 256m
    environment:
      GOMEMLIMIT: 200MiB
    command: ["/vecingest", "worker"]
    stop_grace_period: 30s
    healthcheck:
      test: ["CMD", "/vecingest", "health", "--worker"]
      interval: 60s
      timeout: 5s
      retries: 3
    depends_on:
      api: { condition: service_healthy }
    networks: [internal]

  db:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - db_data:/var/lib/postgresql/data
    command: postgres -c shared_buffers=256MB -c work_mem=8MB -c max_connections=40
    security_opt: [no-new-privileges:true]
    mem_limit: 768m
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks: [internal]

  clamav:                         # opcional: docker compose --profile av up -d
    image: clamav/clamav:stable_base
    profiles: [av]
    restart: unless-stopped
    volumes:
      - clamav_db:/var/lib/clamav
    mem_limit: 1536m
    healthcheck:
      test: ["CMD", "clamdcheck.sh"]
      interval: 60s
      start_period: 300s
    networks: [internal]

networks:
  internal:                        # bridge sin puertos publicados propios. db y clamav solo son
                                   # alcanzables desde aquí; worker y clamav necesitan salida a
                                   # Internet. api/site/web llegan al exterior por su propio
                                   # `ports:`, no por una red de proxy compartida.

volumes:
  db_data:
  clamav_db:
```

### 8.1.1 Quién construye las imágenes, y por qué no Portainer

Portainer despliega el stack **desde GitHub**, pero **no construye imágenes**: ningún servicio del compose declara `build:`, todos usan `image:` apuntando a GHCR. Construirlas en el servidor es técnicamente posible y rompe cuatro cosas de este mismo documento:

1. **El escaneo previo a la publicación.** `deploy.yml` construye la imagen sin publicarla, la escanea con Trivy bloqueando en HIGH y CRITICAL, y solo entonces publica **esa misma imagen ya escaneada, sin reconstruir**. Si la construye Portainer no hay ningún momento anterior a producción en el que escanearla: la imagen vulnerable ya está corriendo.
2. **El SBOM y la atestación de procedencia**, que genera el paso de publicación de CI. Una imagen construida en el servidor no tiene ni una ni otra, y la cadena de suministro deja de ser auditable.
3. **Las etiquetas inmutables** `sha-<short>` y `v<semver>`. Son las que permiten volver atrás cambiando `TAG` y redesplegando. Construyendo en el servidor solo existe lo último que se compiló.
4. **La memoria del servidor.** La sección 11 pone base de datos, cola y API en la misma máquina. Un build de Go más otro de Expo compiten por RAM con Postgres, y la imagen de la API pesa menos de 30 MB precisamente porque se construye en varias etapas en CI y solo viaja el binario.

Si en algún momento se decide construir en el servidor, hay que reescribir antes el punto de la puerta de seguridad de 10.1 que exige escaneo bloqueante, porque tal como está redactado no se podría cumplir.

### 8.2 Variables del stack (`env.example`)

Valores en Portainer, nunca en el repositorio.

```dotenv
# Copia estas variables en Portainer > Stacks > Environment variables.
#
# Los valores entre [corchetes] no son literales: son la forma de obtenerlos.
# Sustituye el corchete entero por el resultado del comando, o por el valor que
# te dé el panel indicado. Ninguna variable de este fichero debe quedar con el
# corchete puesto.
#
# Dos formatos de secreto, y NO son intercambiables:
#   - `openssl rand -hex 32`     -> para lo que acaba dentro de una URL.
#   - `openssl rand -base64 32`  -> para claves de 32 bytes que no viajan en URL.
# El motivo está explicado junto a las contraseñas de base de datos.

# Dominio y proxy
APP_ENV=production                            # development | staging | production
DOMAIN=tudominio.com
SITE_PORT=24221                              # puerto publicado en el host para `site`; nginx apunta aquí
WEB_PORT=38043                               # puerto publicado en el host para `web`; nginx apunta aquí
API_PORT=34246                               # puerto publicado en el host para `api`; nginx apunta aquí
# PROXY_IP: dirección que la API observa como origen de la petición de nginx, para
# `trust proxy`. Con la red docker compartida anterior era la IP fija del contenedor
# de NPM; ahora nginx llega por el puerto publicado en el host, así que lo que la API
# ve como origen es la puerta de enlace NAT de docker, no la IP del propio nginx. Este
# valor SE DEBE CONFIRMAR TRAS EL PRIMER DESPLIEGUE mirando qué dirección reporta la
# API como origen real; adivinarlo mal no falla de forma ruidosa, rompe en silencio el
# rate limiting de confianza acotada: o no confía en nadie, o confía en el salto
# equivocado.
PROXY_IP=[docker inspect -f '{{range .NetworkSettings.Networks}}{{.Gateway}} {{end}}' $(docker ps -qf name=api) ]

# Imágenes
REGISTRY=[tu organización en GHCR, p. ej. ghcr.io/jorgealonsodev]
TAG=latest                                   # en producción usar una etiqueta de versión, p. ej. v1.4.2

# Base de datos
# POSTGRES_* es el superusuario de la imagen: solo lo usa el arranque de roles.
#
# IMPORTANTE: estas dos contraseñas se interpolan DENTRO de una URL
# (`postgres://usuario:CONTRASEÑA@db:5432/base`), así que generarlas en base64 rompe
# el despliegue: ese alfabeto incluye `/`, `+` y `=`, y un `/` en la contraseña parte
# la URL por la mitad. Por eso aquí es hexadecimal y no base64.
POSTGRES_USER=vecingest
POSTGRES_PASSWORD=[openssl rand -hex 32]
POSTGRES_DB=vecingest
APP_DB_USER=app_rw                           # rol de runtime, sin UPDATE/DELETE/TRUNCATE en tablas append-only
APP_DB_PASSWORD=[openssl rand -hex 32]
# Las dos URLs completas. Compón cada una con los valores de arriba; la contraseña de
# `vecingest_owner` no tiene variable propia porque solo vive dentro de esta URL:
# genérala también con [openssl rand -hex 32] y guárdala donde guardes el resto.
BOOTSTRAP_DATABASE_URL=[postgres://vecingest:$POSTGRES_PASSWORD@db:5432/vecingest]
MIGRATIONS_DATABASE_URL=[postgres://vecingest_owner:CONTRASEÑA_DEL_OWNER@db:5432/vecingest]

# Claves de la aplicación (32 bytes en base64 cada una; no viajan en ninguna URL)
JWT_SECRET=[openssl rand -base64 32]
JWT_REFRESH_SECRET=[openssl rand -base64 32]
JWT_SECRET_PREVIOUS=                         # vacío salvo durante una rotación
ENCRYPTION_KEY=[openssl rand -base64 32]     # cifrado de IBAN, DNI y TOTP. Prefijo de versión lo gestiona la app

# Cloudflare R2  —  panel: Cloudflare > R2 > Manage API Tokens
R2_ACCOUNT_ID=[Cloudflare > R2 > Overview, "Account ID" a la derecha]
R2_BUCKET=vecingest
R2_ACCESS_KEY_ID=[token R2 con permiso de lectura y escritura sobre R2_BUCKET]
R2_SECRET_ACCESS_KEY=[se muestra UNA sola vez al crear el token: cópialo entonces]
R2_BACKUP_BUCKET=vecingest-backups           # lo usa el cron de backups del host, no la API
R2_BACKUP_ACCESS_KEY_ID=[token DISTINTO, con permiso de SOLO escritura sobre el bucket de backups]
R2_BACKUP_SECRET_ACCESS_KEY=[se muestra UNA sola vez al crear el token]

# Email
# No hay proveedor de SMS: el OTP de voto y firma va por email o por TOTP
# (decisión de fase 1, ver 11). El correo pasa a ser un canal crítico, no solo
# de avisos: si el SMTP cae, nadie puede votar a distancia ni firmar un acta.
SMTP_URL=[smtps://usuario:contraseña@smtp.tuproveedor.com:465 — credenciales del proveedor de correo]
MAIL_FROM="Vecingest <no-reply@mail.vecingest.app>"

# Servicios externos
TURNSTILE_SECRET=[Cloudflare > Turnstile > tu widget > "Secret Key"]
TSA_URL=https://freetsa.org/tsr
EXPO_ACCESS_TOKEN=[expo.dev > Account settings > Access tokens > Create token]
SENTRY_DSN=[Sentry > tu proyecto > Settings > Client Keys (DSN)]

# Superadmin inicial: NO se crea por variables de entorno. Se crea una sola vez con el
# subcomando idempotente `vecingest bootstrap-superadmin`, pasándole las credenciales en la
# invocación, para no dejar una contraseña de administrador en las variables del stack. Ver 5.1.
# La contraseña que le pases debe cumplir la política de 5.1: 15 caracteres si la cuenta no
# tiene 2FA, 12 si ya tiene TOTP activo. Para generarla: [openssl rand -base64 24]
```

## 9. Convenciones de código y Definition of Done

- Go idiomático en la API: errores envueltos con contexto (`fmt.Errorf("...: %w", err)`), sin `panic` fuera de `main`, `context.Context` en toda función que haga I/O, sin goroutines sin control de ciclo de vida. `golangci-lint` limpio. TypeScript estricto en la app y en `packages/shared`. Sin `any`.
- `api/openapi/openapi.yaml` (generado por `huma` desde los structs de Go) es la única fuente de verdad del contrato. De él se generan el cliente TS y los esquemas Zod de los formularios. Un PR que cambie un struct de entrada/salida sin regenerar (`make gen`) falla en CI.
- Nombres: inglés en código y base de datos; español en textos de UI (vía i18n).
- Commits: Conventional Commits (`feat:`, `fix:`, `chore:`…).
- Tests: unitarios en `internal/domain` con tablas de casos, e2e de API con Postgres real en Testcontainers, tests de componentes críticos con React Native Testing Library. Cobertura mínima exigida en CI: 80 % en `internal/domain/...` y `internal/legal/...`.
- Errores de API con códigos estables (`AUTH_INVALID_CREDENTIALS`, `FORBIDDEN_MEMBERSHIP`, `INCIDENT_INVALID_TRANSITION`, `MEETING_CALL_TOO_SOON`, `VOTE_DEBTOR_BLOCKED`, `BOOKING_OVERLAP`…) definidos en `packages/shared` y traducidos en la app.
- Nunca exponer `file_key` como URL pública; siempre generar URL prefirmada.
- Toda mutación pasa por un servicio de dominio que comprueba permisos; los controladores no contienen lógica.
- Ninguna regla legal (plazos, porcentajes, mayorías) se escribe como constante: se lee de `legal_rules` a través de `internal/legal` (`Rules.Get(ctx, key, at time.Time)`), que cachea en memoria con invalidación por `LISTEN/NOTIFY` de Postgres y resuelve la vigencia por fecha.
- Fechas: se guardan en UTC (`timestamptz`); la lógica de plazos legales se calcula en `Europe/Madrid` con `time.LoadLocation` (la zona horaria va embebida en el binario con `time/tzdata`); los "días naturales" cuentan desde las 00:00 del día siguiente al hecho.
- Dinero: `numeric` en BD, `shopspring/decimal` en Go y string decimal en API (`"123.45"`); nunca `float64`. Porcentajes de cuota con 4 decimales.
- Documentos legales (convocatoria, acta, certificados) se generan desde plantillas en `docs/legal/` con versión; el PDF guarda la versión de plantilla y de `legal_rules` usada.

**Definition of Done de una funcionalidad** (no se cierra el PR si falta algo):
1. Structs de entrada/salida con validaciones, `make gen` ejecutado y `openapi.yaml`, código `sqlc` y cliente TS regenerados y commiteados.
2. Migración `goose` revisada (expand/contract si toca datos existentes) y consultas `.sql` con filtro de ámbito.
3. Middleware de membresía en cada ruta nueva y test que comprueba que otro rol/comunidad recibe 403.
4. Tests unitarios del servicio y al menos un e2e del flujo principal.
5. Eventos de dominio y notificaciones definidos en la tabla 7.6 y encolados con `river.InsertTx` dentro de la transacción.
6. Textos en `i18n` (es) sin strings en el código; errores por código.
7. Pantalla accesible: etiquetas, foco, contraste; probada en iOS, Android y web.
8. Entrada en `audit_log` si la acción es sensible (dinero, votos, datos personales, permisos).
9. Documentación breve en el README del módulo y, si aplica, en el runbook.
10. **Revisión de seguridad del PR**: checklist rellenado en la plantilla del PR (decoders con `DisallowUnknownFields` y validaciones `huma`, ámbito de membresía comprobado, ningún dato sensible en logs/errores/push, secretos fuera del código, migración sin pérdida de restricciones append-only), CI de seguridad en verde (`gitleaks`, `govulncheck`, `gosec`, `pnpm audit`, Semgrep, Trivy, test de matriz de permisos) y, si el PR toca auth, ficheros, dinero, votos o datos personales, revisión por una segunda persona con la etiqueta `security-review`.

## 10. Plan de entregas

Cada hito tiene dos criterios: el funcional y la **puerta de seguridad**. Los dos son obligatorios. La puerta se verifica con evidencia (resultado de CI, informe, captura, entrada en el runbook), no con una afirmación.

| Hito | Contenido | Criterio funcional | Puerta de seguridad (resumen; detalle en 10.1) |
|---|---|---|---|
| M0 | Monorepo, binario Go con `serve`/`worker`/`migrate`/`seed`, imágenes en GHCR, stack de Portainer, esquema base con `goose` + `sqlc`, auth, `/me`, cliente TS generado | Login desde web y móvil contra el servidor desplegado por Portainer | Base segura: hashing, tokens, cookies, 2FA superadmin, secretos, CI de seguridad, contenedores endurecidos, rol de BD restringido |
| M1 | Comunidades, viviendas, invitaciones, portales vacíos por rol | Un admin crea una comunidad e invita a un vecino que entra en su portal | Aislamiento entre tenants demostrado por tests; 2FA admins; anti-abuso en formularios públicos; correo autenticado |
| M2 | Incidencias completas con fotos, asignación y notificaciones | Flujo vecino → admin → empresa → cierre en móvil | Ficheros seguros de extremo a extremo; nada sensible en logs ni push; alertas de acceso |
| M3 | Avisos, documentos, directorio | Vecino recibe push al publicarse un aviso y descarga un acta | Web endurecida (CSP), telemetría sin PII, backups cifrados y restaurados |
| M4 | Publicación en tiendas, backups, CI de despliegue | Apps en TestFlight y Play internal testing | Pentest externo sin hallazgos altos abiertos, DAST, OTA firmado, runbook de incidentes, cumplimiento de tiendas |
| M5 | Recibos y saldo, morosidad | Propietario ve su saldo; el admin exporta un certificado de deuda | Cifrado de IBAN con clave versionada; reautenticación para cambiarlo; certificados de deuda con acceso registrado |
| M6 | Reservas de zonas comunes | Dos vecinos no pueden reservar la misma franja | Constraint de exclusión probado bajo concurrencia; sin fuga de reservas de otras viviendas |
| M7 | Juntas y voto online | Convocatoria válida según art. 16, votación con OTP, acta firmada y notificada | Integridad probatoria: cadena de hash, anclaje RFC 3161, OTP por email/TOTP verificados con el canal registrado, revisión legal de evidencias, pentest específico del módulo |
| M8 | Empresa: tareas, registro de facturas, fichaje | Informe mensual de jornada exportado y factura vinculada a incidencia | Fichajes y facturas append-only, aislamiento entre empresas, documentación PRL con acceso controlado |

### 10.1 Puertas de seguridad por hito (checklist verificable)

Formato: cada línea es una comprobación con su evidencia. El hito se cierra cuando todas están en verde y el resultado queda archivado en `docs/security/gates/M<n>.md`.

**M0 — Base**
- [ ] Argon2id con los parámetros de 5.1 (m=19456 KiB, t=2, p=1, sal 16 B, tag 32 B) y test que rechaza contraseñas por debajo del mínimo aplicable —15 caracteres sin 2FA, 12 con TOTP activo— o filtradas (HIBP). *Evidencia: test.*
- [ ] Refresh, invitación y reset hasheados en BD; reutilización de refresh invalida la familia. *Evidencia: test e2e.*
- [ ] Cookie `Strict` + `Path` acotado; validación de `Origin` y token CSRF en `/auth/refresh`. *Evidencia: test e2e con origen distinto → 403 y test con cookie válida sin token CSRF → 403.*
- [ ] 2FA TOTP funcionando y obligatorio para `superadmin`; login de superadmin en ruta separada. *Evidencia: test.*
- [ ] `internal/config` valida el entorno: el proceso no arranca con una variable ausente. *Evidencia: test de arranque.*
- [ ] Presupuesto de recursos medido: `api` y `worker` < 128 MB en reposo, imagen < 30 MB, arranque < 1 s, p95 de `GET /v1/me` < 50 ms en el servidor. *Evidencia: `docker stats` y `k6` archivados en `docs/security/gates/M0.md`.*
- [ ] `gitleaks`, `govulncheck`, `gosec`, `pnpm audit --audit-level=high` (app), Trivy y Semgrep en CI y bloqueantes. *Evidencia: workflow en verde y un PR de prueba bloqueado.*
- [ ] Contenedores `read_only`, no root (distroless `nonroot`), `cap_drop: ALL`, `mem_limit` y `GOMEMLIMIT`; Portainer con 2FA y sin exponer `docker.sock` a otros contenedores. *Evidencia: `docker inspect` archivado.*
- [ ] Rol `app_rw` de Postgres sin `UPDATE`, `DELETE` ni `TRUNCATE` en las tablas append-only (`audit_log`, y `votes` y `time_entries` cuando existan) —`TRUNCATE` es un privilegio distinto de `DELETE` y revocar solo los dos primeros deja la tabla vaciable—, sin ser propietario de esas tablas ni miembro del rol propietario; test que intenta las tres operaciones y falla. *Evidencia: test.*
- [ ] Rate limiting (`httprate`) con `trust proxy` acotado a `PROXY_IP`. *Evidencia: test con `X-Forwarded-For` falso desde otra IP → ignorado.*
- [ ] Redacción de PII en el handler de `slog` configurada y probada con un log que contiene email, teléfono e IBAN. *Evidencia: test.*
- [ ] Threat model inicial escrito (`docs/security/threat-model.md`) a partir de la tabla 6.1. *Evidencia: documento.*

**M1 — Aislamiento**
- [ ] Test de matriz de permisos: cada endpoint × cada rol × recurso propio/ajeno; ajeno → 403 o 404. Se ejecuta en cada PR. *Evidencia: informe del test con cobertura del 100 % de rutas.*
- [ ] `make lint-scope`: ninguna consulta `sqlc` sobre tablas con ámbito sin filtrar por `community_id`/`office_id`/`company_id`. *Evidencia: salida del linter en CI.*
- [ ] 2FA obligatorio para `admin` y `admin_staff`; sin 2FA no se puede entrar al portal admin. *Evidencia: test.*
- [ ] Turnstile en alta de empresa, contacto y recuperación; límite por IP. *Evidencia: prueba manual archivada.*
- [ ] SPF, DKIM 2048 y DMARC `p=reject` publicados y verificados con una herramienta externa. *Evidencia: informe.*
- [ ] Invitaciones: caducidad, un solo uso, bloqueo tras 10 códigos fallidos. *Evidencia: test.*
- [ ] Importación CSV: sin inyección de fórmulas (`=`, `+`, `-`, `@` al inicio de celda se escapan en exportaciones) y sin path traversal en nombres. *Evidencia: test.*

**M2 — Ficheros y notificaciones**
- [ ] Subida: URL prefirmada con `Content-Length` y `Content-Type` firmados; `confirm` verifica tamaño y *magic bytes*; SVG/HTML/ZIP rechazados. *Evidencia: tests con ficheros polyglot.*
- [ ] ClamAV operativo (perfil `av` o modo diferido); fichero EICAR queda `infected` y no es visible. *Evidencia: test e2e.*
- [ ] Descargas con `Content-Disposition: attachment`, URL de 5 min, registro en `audit_log`. *Evidencia: test.*
- [ ] EXIF eliminado en cliente; test que sube una foto con GPS y comprueba que R2 no lo contiene. *Evidencia: test Maestro/unitario.*
- [ ] Push y email sin datos sensibles: revisión de todas las plantillas. *Evidencia: checklist firmada.*
- [ ] Alertas de inicio de sesión nuevo y de cambio de contraseña/2FA. *Evidencia: test.*
- [ ] `Idempotency-Key` en POST de incidencias y comentarios; reintento no duplica. *Evidencia: test.*
- [ ] Prueba de carga "lunes de incidencias" con `k6` (200 incidencias/min con fotos durante 10 min) dentro de SLO. *Evidencia: informe en `docs/security/gates/M2.md`.*

**M3 — Web y datos**
- [ ] CSP sin `unsafe-inline` en Expo web; informe de ZAP baseline sin alertas medias. *Evidencia: informe.*
- [ ] Sentry sin PII ni cuerpos de petición. *Evidencia: captura de configuración y evento de prueba.*
- [ ] Backup cifrado con `age` a bucket separado con credenciales de solo escritura y versionado; **restauración completa probada en staging**. *Evidencia: entrada en el runbook con fecha y duración.*
- [ ] Markdown sanitizado: test con `<script>`, `javascript:` y `onerror`. *Evidencia: test.*
- [ ] Exportación RGPD con reautenticación y enlace de un solo uso. *Evidencia: test.*

**M4 — Lanzamiento**
- [ ] Pentest externo de API, web y app; hallazgos críticos y altos cerrados, medios con plan y fecha. *Evidencia: informe y reprueba.*
- [ ] DAST (ZAP full) contra staging en el pipeline de release. *Evidencia: workflow.*
- [ ] Code signing de `expo-updates` con clave fuera de Expo; 2FA en Expo, GitHub, Cloudflare y registrador. *Evidencia: capturas.*
- [ ] `security.txt`, política de divulgación y runbook de incidentes con simulacro realizado (tabletop de 1 h). *Evidencia: acta del simulacro.*
- [ ] Host endurecido según 6.1 (ufw, SSH por clave, fail2ban, unattended-upgrades, panel de NPM no expuesto). Incluye `SITE_PORT`, `WEB_PORT` y `API_PORT` (§8): publicarlos en el host para el proxy inverso manual desplaza el límite de seguridad que antes ponía la red docker compartida al cortafuegos, así que ufw debe cerrarlos explícitamente a Internet o quedan expuestos. *Evidencia: salida de `lynis` o checklist, más regla de `ufw` para esos tres puertos.*
- [ ] Etiquetas de privacidad de App Store y Data Safety de Google coherentes con la sección 4.4. *Evidencia: capturas.*
- [ ] Registro de actividades de tratamiento y contrato de encargo firmados con los despachos piloto. *Evidencia: documentos.*

**M5 — Recibos**
- [ ] IBAN cifrado con AES-256-GCM y prefijo de versión de clave; rotación probada re-cifrando en segundo plano. *Evidencia: test de rotación.*
- [ ] Cambio de IBAN exige reautenticación (contraseña + 2FA si activo) y notifica por email. *Evidencia: test.*
- [ ] Certificados de deuda solo para admin, con registro de cada emisión. *Evidencia: test de permisos.*

**M6 — Reservas**
- [ ] Test de concurrencia (50 reservas simultáneas sobre la misma franja) → exactamente una confirmada. *Evidencia: test.*
- [ ] Un vecino no puede listar ni inferir reservas de otra vivienda (solo ve ocupación anónima). *Evidencia: test de permisos.*

**M7 — Juntas y voto**
- [ ] Cadena de hash verificable con un comando de línea (`vecingest verify-chain <meeting_id>`) que recorre `votes` y comprueba cada eslabón. *Evidencia: test y comando.*
- [ ] Anclaje RFC 3161 del `head_hash` al cerrar cada punto y envío al presidente/secretario. *Evidencia: sello archivado y verificado con `openssl ts -verify`.*
- [ ] OTP solo por email o TOTP; límites de emisión activos (3/hora, 10/día por cuenta); test que intenta 4 envíos en una hora y falla. *Evidencia: test.*
- [ ] `signature_evidence` registra el canal del OTP (`email` o `totp`) en cada voto y cada firma de acta, para que la fuerza probatoria de cada uno sea distinguible a posteriori. *Evidencia: test.*
- [ ] Deudores no pueden votar aunque manipulen la petición; resultados excluyen su cuota. *Evidencia: test.*
- [ ] Voto emitido desde un dispositivo no puede modificarse por otro usuario de la misma vivienda sin nueva OTP. *Evidencia: test.*
- [ ] Pentest específico del módulo de voto (manipulación de resultados, doble voto, delegación falsa). *Evidencia: informe.*
- [ ] Prueba de carga "tarde de junta" con `k6` (1.000 comunidades votando, 50 votos/s durante 10 min, cierre de un punto con 5.000 votantes < 5 s). *Evidencia: informe.*
- [ ] Revisión jurídica de `signature_evidence` y de las plantillas de acta y delegación. *Evidencia: informe del abogado.*

**M8 — Empresa**
- [ ] `time_entries` y `invoices` append-only a nivel de rol de BD; corrección solo por nuevo registro con motivo. *Evidencia: test.*
- [ ] Aislamiento entre empresas (una empresa no ve incidencias, facturas ni fichajes de otra) en el test de matriz. *Evidencia: informe.*
- [ ] Documentos PRL/seguros con las mismas garantías que los documentos de comunidad (escaneo, disposition, registro). *Evidencia: test.*

**Continuo (todos los hitos)**
- Renovate semanal con fusión automática solo de parches; CVE alta o crítica → corrección en 7 días.
- Revisión trimestral de accesos (quién tiene cuenta en Expo, GitHub, Cloudflare, servidor) y baja inmediata al salir alguien del equipo.
- Rotación anual de claves y simulacro anual de restauración y de incidente.
- Cada funcionalidad nueva añade su fila a la tabla de amenazas de 6.1 antes de escribir código.

## 11. Riesgos y decisiones abiertas

- **OTP de voto y firma por email en vez de SMS** (decidido en fase 1, revisable). Se elimina el proveedor de SMS: desaparecen su coste por mensaje, la rotación de sus credenciales y la amenaza de *SMS pumping*. El precio es real y está aceptado a conciencia: **el email es el mismo canal que la recuperación de contraseña**, así que quien controle el buzón de un propietario puede suplantar su voto por completo, y eso debilita la fuerza probatoria de un voto impugnado bajo el art. 17. Mitigaciones: OTP de un solo uso hasheado, límite de emisión, aviso al propietario en cada envío, y `signature_evidence` guarda el canal para poder distinguir después un voto firmado con TOTP de uno firmado por email. **Se recomienda activar TOTP a quien vaya a votar a distancia.** Disparador para revisar la decisión: la primera impugnación que cuestione la identidad del votante, o el primer caso real de buzón comprometido.

- **Panel de administrador en Expo web**: si las tablas y filtros resultan pesados, migrar el portal admin a Next.js compartiendo `packages/shared`. Decidir en M3.
- **Multitenancy**: se opta por base de datos única con filtrado por `community_id`/`office_id`. Revisar si se supera el millar de comunidades.
- **Notificaciones push en web**: Expo Push no cubre navegadores. En el MVP la web solo tiene notificaciones in-app y email; web push (VAPID) queda para fase 2.
- **Expo Go**: en cuanto se añadan módulos nativos (secure store, notificaciones, cámara) hay que trabajar con development builds, no con Expo Go.
- **Coste de EAS**: el plan gratuito limita builds mensuales; valorar builds locales en un Mac o el plan de pago.
- **Cuenta de Expo/EAS como punto único de compromiso**: puede publicar código a todos los móviles. Mitigado con 2FA, code signing de updates con clave fuera de Expo y acceso limitado a dos personas.
- **Escalar antes de tiempo**: la tentación de montar Kubernetes, Citus o NATS en fase A. Cada pieza añade operación, coste y superficie de ataque; se añaden solo cuando un SLO lo pide (7.10).
- **Un único servidor** (fase A): BD y API en la misma máquina sin alta disponibilidad. Aceptado para el MVP; el RTO de 4 h se cubre con backups y un runbook de restauración probado. Si se superan ~200 comunidades activas, separar la base de datos.
- **Dos lenguajes en el monorepo** (Go en la API, TypeScript en app y web): el riesgo es la deriva del contrato. Mitigado con el `openapi.yaml` generado y la comprobación en CI de que el cliente TS está al día; nadie escribe tipos a mano en los dos lados.
- **ClamAV frente al presupuesto de memoria**: es el único servicio que rompe el objetivo de 1 GB. Perfil opcional y escaneo diferido documentados en 7.7; decidir en M2 según la RAM disponible.
- **Panel de administración con tablas grandes en React Native Web**: mitigado con `@tanstack/react-table` + virtualización (`FlashList`); si aun así no rinde, ver el riesgo de migración a Next.js.
- **Facturación electrónica**: normativa española en evolución (Verifactu, factura electrónica B2B). La plataforma no emite facturas precisamente para no quedar sujeta; revisar si se cambia esa decisión.
- **Reformas de la LPH**: se modifica con frecuencia (el art. 17 ha cambiado en enero, junio, julio de 2025 y marzo de 2026) y hay un proyecto de reforma en el Congreso que previsiblemente regulará las juntas telemáticas. Las reglas de mayorías y plazos se implementan como configuración (tabla `legal_rules` con vigencia) y no como constantes en código.
- **Juntas telemáticas sin base legal expresa**: hasta que la reforma entre en vigor, la modalidad híbrida solo se ofrece a comunidades con estatutos que la prevean o aceptación unánime registrada, y la modalidad solo remota queda deshabilitada. Aun así existe riesgo de impugnación; el producto lo comunica al admin al elegir la modalidad.
- **Validez probatoria del voto y del acta electrónicos**: depende de la calidad de la evidencia de firma. Conservar `signature_evidence` de forma inmutable y valorar un sello de tiempo cualificado (TSA) en fase 3.

## 12. Glosario y correspondencia de términos

El dominio es español pero el código está en inglés. Usar siempre esta correspondencia; no inventar sinónimos.

| Español (UI, negocio) | Inglés (código, BD, API) |
|---|---|
| Comunidad de propietarios | `community` |
| Despacho de administración de fincas | `office` |
| Administrador de fincas | `admin` (rol) |
| Vivienda / local / garaje / trastero | `unit` (`flat` / `premises` / `garage` / `storage`) |
| Portal (del edificio) | `block` |
| Propietario / inquilino | `owner` / `tenant` |
| Cotitular | co-owner (varios `unit_members` con rol `owner`) |
| Presidente / vicepresidente / secretario | `board_role`: `president` / `vice_president` / `secretary` |
| Junta directiva | board |
| Cuota o coeficiente de participación | `participation_coefficient` |
| Moroso | `debtor` |
| Empresa de servicios / proveedor | `company` / `provider` (proveedor habitual de un despacho) |
| Autónomo | `freelancer` |
| Trabajador | `worker` |
| Incidencia | `incident` |
| Zona común | `common_area` |
| Reserva | `booking` |
| Aviso / comunicado | `announcement` |
| Documento / acta / estatutos | `document` / `minutes` / `bylaws` |
| Junta (reunión) | `meeting` |
| Convocatoria | `call` (`first_call_at`, `second_call_at`) |
| Orden del día / punto | agenda / `meeting_item` |
| Voto / delegación | `vote` / `vote_delegation` |
| Acta | `minutes` |
| Recibo / cuota | `receipt` |
| Fondo de reserva | `reserve_fund` |
| Factura / rectificativa | `invoice` / `rectifying invoice` |
| Retención (IRPF) | `withholding` |
| Registro de jornada / fichaje | `time_entry` (clock in / clock out) |
| Solicitud de presupuesto | `service_request` |
| Valoración | `rating` |
| Encargado del tratamiento | data processor (`dpa_signed_at`) |
| Salvar el voto | `saved_vote` |
| Nuda propiedad / usufructo | `bare_owner` / `usufructuary` |
| Propuesta de punto del orden del día | `agenda_request` |
| Diligencia de tablón | board notice (`channel = board`) |
