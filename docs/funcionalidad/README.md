# Inventario de funcionalidad, reconstruido desde los diseños

Qué hace falta construir, extraído pantalla por pantalla de los dos proyectos
de Stitch y contrastado con `PRD_go.md`.

Se hizo en septiembre de 2026, con el proyecto en M0: la API tiene doce
endpoints (autenticación, salud y perfil) y la base de datos seis tablas
(`users`, `sessions`, `audit_log`, `password_reset_tokens`, `user_mfa`,
`otp_challenges`). Todo lo que aparece en estos ficheros está sin construir
salvo donde se indique lo contrario.

## Los cinco ficheros

Cada uno cubre un tramo y está ordenado **por hito del PRD**, no por pantalla:
lo que interesa es qué cuesta M1, no qué hace la pantalla 47.

| Fichero | Cubre | Pantallas |
| --- | --- | --- |
| [`app-movil-1.md`](./app-movil-1.md) | *Acta* → *Delegación activa* | 19 |
| [`app-movil-2.md`](./app-movil-2.md) | *Delegar mi voto* → *Informe mensual* | 20 |
| [`app-movil-3.md`](./app-movil-3.md) | *Iniciar sesión* → pista de pádel | 18 |
| [`app-movil-4.md`](./app-movil-4.md) | *Proponer un punto* → *Zonas comunes* | 18 |
| [`consola-web.md`](./consola-web.md) | Los 46 diseños de escritorio | 46 |

**121 pantallas reales** (46 de escritorio + 75 de móvil), contando **filas
del índice** de `stitch-screens-web.md` y `stitch-screens.md` (lo que
devuelve `mcp__stitch__list_screens`). Las seis restantes de las 127 filas
combinadas de esos dos índices son fotografías de banco, logotipos y la
copia que Stitch guarda del propio `design.md`.

Estas cifras no van a coincidir con lo que devuelva `mcp__stitch__get_project`
para el proyecto de escritorio: esa llamada trabaja con `screenInstances`, no
con el índice de pantallas, y ahora mismo devuelve 51 entradas en vez de 48.
La diferencia es que Stitch representa un borrado hecho desde su interfaz
poniendo `"hidden": true` en la entrada existente, no eliminándola — así
siguen apareciendo los dos ids vacíos que el usuario borró el 2026-09-15
("Recibos" y "Deudores y certificados" originales), y también una entrada de
canvas del propio design system que no es una fila del índice. Ver
`consola-web.md` para el detalle de esos dos ids y los que los sustituyen.

Los proyectos de origen están documentados en
[`../design/README.md`](../design/README.md): "Vecingest APP" para móvil,
"VecinGest WEB" para escritorio.

## El resultado que más importa

**El PRD ya nombraba casi todo.** Los cinco bloques, trabajando por separado,
llegaron a la misma conclusión: las secciones 7.3 y 7.4 de `PRD_go.md` ya
contienen los nombres de tablas y endpoints que las pantallas necesitan
— `incidents.transition`, `service-requests`, `agenda-requests`,
`meeting-items/{id}/votes`, `bookings`, `invoices`, `task_reports`.

Así que esto no inventó el dominio: lo reconstruyó desde la interfaz y
comprobó que coincide con lo que ya estaba escrito. Los pocos casos donde el
PRD describe un comportamiento sin nombrar su ruta van marcados como
**provisionales**.

## Contradicciones y huecos encontrados

No son fallos del inventario: son decisiones pendientes.

1. **Los diseños siguen diciendo SMS.** La pantalla "Delegar mi voto" ofrece
   *"Firmar delegación con código SMS"*. El PRD decidió lo contrario (línea
   423: "Sin proveedor de SMS"; el porqué y el precio aceptado, en la 1644).
   Los diseños son anteriores a esa decisión. Hay que actualizarlos en Stitch
   o asumir que el texto miente.

2. **La pantalla "Recibos" tiene un botón de pagar.** El PRD dice que "el
   cobro se gestiona fuera de la plataforma" (línea 296). No se inventó un
   endpoint de pago para sostenerlo.

3. **M6 no tiene ninguna pantalla de escritorio.** Las reservas de zonas
   comunes solo están diseñadas para móvil. Es un hueco de diseño, no una
   omisión del inventario.

4. **Tres funciones no encajan en ningún hito**: tareas del despacho, remesas
   y liquidaciones, y reclamaciones monitorias, todas visibles en "Comunidad,
   resumen". O falta un hito, o falta decidir dónde van.

5. **Los roles de empresa que dibujan las pantallas son más finos** que los
   dos valores del enum `company_members.role` del PRD ("Técnico Campo",
   "Responsable Oficina").

6. **"Emitir recibos / Exportar SEPA" es ambiguo frente al PRD.** La nueva
   pantalla "Recibos" (`10faad887d6b4a69bb0975315a2a1a83`) ofrece ese botón
   más una sincronización bancaria CSB 19/58, pero el PRD (§5.9, línea 321)
   dice que "los recibos se generan fuera (software del despacho) y se
   importan; la plataforma no calcula cuotas en esta fase". Si "emitir"
   solo exporta a SEPA lo ya importado, no hay conflicto; si genera recibos
   nuevos, sí lo hay. No se resolvió la ambigüedad ni se inventó el
   endpoint que faltaría para el segundo caso. La propia pantalla, en
   cambio, **no** tiene botón de cobrar — "Marcar pagado" es una anotación
   manual, coherente con la línea 296 del PRD — así que el problema de la
   contradicción 2 (móvil) no se repite en el escritorio.

**Nota resuelta:** "Recibos" y "Deudores y certificados" llegaron vacías
desde Stitch; el 2026-09-15 se generaron ids nuevos con contenido completo y
el usuario borró los vacíos desde la interfaz de Stitch. Ya no es una
contradicción ni un hueco — detalle en `consola-web.md` (M5) y
`docs/design/stitch-screens-web.md`.

## Reasignaciones de hito

Tres pantallas estaban mentalmente en el hito equivocado:

- **"Encargo en curso" y "Encargo, detalle"** → M2, no M8. El criterio de M2
  dice literalmente "flujo vecino → admin → empresa → cierre": son la
  continuación del mismo estado de `incidents`, no una función de empresa
  aparte. Que la interfaz diga "encargo" no crea un concepto nuevo.
- **"Parte de trabajo"** → M8. La tabla `task_reports (task_id, user_id, body,
  hours, file_keys)` coincide campo por campo con la pantalla.
- **"Inicio" y "Más"** no tienen hito propio: agregan y navegan.

## Lo que tiene peso legal

Los ficheros lo marcan como requisito, no como descripción de interfaz. El
grupo más denso está en M7:

- Quórum de doble convocatoria y regímenes de mayoría por punto del orden del
  día (art. 17 LPH), que varían según el acuerdo.
- Exclusión de voto a deudores (art. 15.2), con sus excepciones: impugnación
  judicial y deuda consignada.
- Cómputo del voto presunto de ausentes (art. 17.8) y sus plazos, que obliga a
  un resultado **provisional** en `meeting_item_results` hasta que venza.
- Doble firma obligatoria del acta (art. 19.2 y 19.3) y plazos del 19.4.
- Cadena de hash del libro de actas y sellado RFC 3161.
- Validez de la notificación electrónica (art. 9).
- Aislamiento entre comunidades en toda consulta.

Fuera de M7: la restricción de exclusión de PostgreSQL que impide que dos
vecinos reserven la misma franja (M6, criterio de aceptación explícito), y los
fichajes append-only del RD-ley 8/2019 (M8).

## Cómo usarlo

Para planificar un hito, lee su sección en los cinco ficheros: cada uno trae
los endpoints y las tablas que ese tramo de pantallas necesita. La
deduplicación está hecha dentro de cada fichero, no entre ellos — un endpoint
que aparezca en tres ficheros es el mismo endpoint.

Para M1, que es lo siguiente, las secciones relevantes son `app-movil-1.md`,
`app-movil-2.md`, `app-movil-3.md` y `consola-web.md`.
