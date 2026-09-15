# Pendientes de funcionalidad frente al diseño

Estado a 2026-09-15. El rediseño del login (móvil + escritorio, Stitch
proyectos `11075381530582947267` y `14138416730329310203`), el flujo
completo de recuperación de contraseña, y la primera pantalla posterior al
login (`app/src/screens/PortalScreen.tsx`, ruta `/portal`) ya están
implementados y con backend real donde lo hay (`GET /v1/me`,
`POST /v1/auth/logout`). Lo que sigue documenta los elementos visuales del
diseño que se construyeron fieles al Stitch pero deliberadamente
deshabilitados o sustituidos por un estado vacío honesto, y por qué, para
que nadie los confunda con un olvido.

## 1. Selector de perfil de usuario en el login -- eliminado, movido a `PortalScreen`

Pantalla Stitch "Login web - Vecingest"
(`projects/14138416730329310203/screens/5f83c21f55d9493da67cc330fa8528d4`).

Lo que se veía hasta el 2026-09-15: un control segmentado de tres opciones
(Vecino / Administrador / Empresa) en el panel derecho del login de
escritorio, `app/src/screens/LoginScreen.tsx`,
`testID="login-profile-selector"`, completamente deshabilitado. Se retiró
por completo de `LoginScreen`: un usuario reportó que, aun deshabilitado, un
selector de tres opciones sin ninguna seleccionada se leía como un control
roto, no como un "todavía no disponible". `POST /v1/auth/login`
(`api/openapi/openapi.yaml:422`, tipo `LoginRequest` en
`packages/shared/src/schemas/index.ts`) nunca aceptó un parámetro de rol, así
que el control nunca pudo afectar al inicio de sesión.

Lo que falta: elegir un perfil/contexto real sigue siendo competencia de la
pantalla **posterior** al login, `PortalScreen`
(`app/src/screens/PortalScreen.tsx`, ruta `/portal`; Stitch "Elegir dónde
entrar", proyecto APP `11075381530582947267`, pantalla
`95e97f53757e419bb498acc29cf049e9`; equivalente de escritorio, "Selector de
contexto", proyecto WEB, pantalla `537dd208d9d649b6a56afa3f29f51de1`) --
construida el 2026-09-15, ver punto 4 -- que necesita comunidades,
viviendas, roles y membresías para tener algo que ofrecer, ninguno de los
cuales existe todavía (sin tablas de dominio, sin endpoints).

Hito: `PRD_go.md` §10, fila M1 -- "Comunidades, viviendas, invitaciones,
portales vacíos por rol" / "Un admin crea una comunidad e invita a un vecino
que entra en su portal".

## 2. Código de invitación

Pantallas Stitch: enlace de pie "Tengo un código de invitación" en "Iniciar
sesión" (móvil, proyecto `11075381530582947267`) y su equivalente en "Login
web - Vecingest" (escritorio, proyecto `14138416730329310203`, pantalla
`5f83c21f55d9493da67cc330fa8528d4`); y, desde el 2026-09-15, el enlace
"Añadir otra comunidad o empresa con código" de "Elegir dónde entrar" /
"Selector de contexto" en `PortalScreen`
(`app/src/screens/PortalScreen.tsx`).

Lo que falta: ningún endpoint de `api/openapi/openapi.yaml` acepta un
código de invitación todavía. Las invitaciones son un concepto de M1 (misma
fila de `PRD_go.md` §10 citada arriba: "invitaciones").

Comportamiento actual en `LoginScreen`: `testID="login-invitation-link"`, no
pulsable (`Pressable disabled`, `accessibilityState={{disabled: true}}`,
`accessibilityLabel="Código de invitación, no disponible todavía"`),
estilo apagado, con la leyenda "Próximamente" junto al texto. En escritorio
se renderiza además con el tratamiento de píldora con borde y la leyenda
"¿Primera convocatoria o registro?" que trae el propio diseño; en móvil se
mantiene el tratamiento de texto simple. Ambos comparten el mismo
`testID` y el mismo estado deshabilitado.

Comportamiento actual en `PortalScreen`: mismo patrón, `testID` propio
(`testID="portal-invitation-link"`), `Pressable disabled`,
`accessibilityState={{disabled: true}}`, leyenda "Próximamente" -- idéntico
tratamiento en móvil y escritorio, ya que ninguno de los dos Stitch de
"Elegir dónde entrar" distingue el enlace por plataforma.

## 3. Recorte deliberado en el flujo de recuperación de contraseña

A diferencia de los dos puntos anteriores, aquí los endpoints **sí**
existen (`POST /v1/auth/forgot-password`, `api/openapi/openapi.yaml:397`;
`POST /v1/auth/reset-password`, `api/openapi/openapi.yaml:551`) y el flujo
está construido de punta a punta en
`app/src/screens/ForgotPasswordScreen.tsx` y
`app/src/screens/ResetPasswordScreen.tsx`. Dos elementos decorativos del
diseño Stitch no se construyeron porque no hay ninguna señal del cliente
que los pueda alimentar honestamente:

- **Medidor de fortaleza de contraseña y aviso fijo de filtración**
  (pantalla "Nueva contraseña", proyecto `11075381530582947267`, pantalla
  `502fdbc3adb84b7da3ae388ece6da9dd`): ningún endpoint devuelve una
  puntuación de fortaleza antes de enviar el formulario, así que construir
  una barra animada habría sido un indicador inventado. En su lugar, el
  aviso de filtración es un estado real dirigido por el servidor: el código
  `AUTH_PASSWORD_BREACHED` se traduce en
  `resetPasswordErrorMessage` (`app/src/screens/errorMessages.ts`) a
  "Esta contraseña aparece en filtraciones conocidas. Elige otra.", mostrado
  tras el envío real.
- **Cuenta atrás de caducidad del enlace** (pantalla "Recuperar
  contraseña", proyecto `11075381530582947267`, pantalla
  `ba621c2b26b7433f812fd5ea34d0a929`, "Validez restante: 59 min" en el
  diseño): ningún endpoint expone cuánto le queda de validez a un token de
  reseteo. Se sustituye por la confirmación no comprometida que ya devuelve
  la API (`ForgotPasswordResponse = {accepted: boolean}`): "Si el email
  existe, te hemos enviado un enlace."

Ninguna de las dos simplificaciones está bloqueada por ningún hito de
`PRD_go.md` §10 -- es una diferencia entre lo que dibuja el diseño y lo que
expone hoy la API, no una funcionalidad pendiente.

## 4. Primera pantalla posterior al login (`PortalScreen`, ruta `/portal`)

Construida el 2026-09-15: "Elegir dónde entrar" (proyecto APP
`11075381530582947267`, pantalla `95e97f53757e419bb498acc29cf049e9`) y su
equivalente de escritorio "Selector de contexto" (proyecto WEB
`14138416730329310203`, pantalla `537dd208d9d649b6a56afa3f29f51de1`) ya son
una ruta real (`app/app/portal.tsx` -> `app/src/screens/PortalScreen.tsx`).
Un login correcto navega aquí (`LoginScreen.tsx`'s `router.replace("/portal")`)
en vez de dejar al usuario mirando el formulario de login sin más.

Lo que es real: `GET /v1/me` se llama con el token en memoria y su
respuesta (`{id, email, is_superadmin}`) se muestra directamente -- "Conectado
como {email}", más una insignia "Superadministrador" cuando corresponde --
como prueba de que la sesión funciona, que es el objetivo de la pantalla en
este hito. `POST /v1/auth/logout` también es real: el botón "Cerrar sesión"
revoca la sesión en el servidor, limpia el token en memoria y el refresh
token de `expo-secure-store`, y vuelve a `/(auth)/login`. Un acceso a
`/portal` sin token en memoria (carga en frío, enlace directo, sesión
caducada -- `GET /v1/me` devolviendo `AUTH_UNAUTHORIZED`) redirige a
`/(auth)/login` en vez de renderizar en blanco o una pantalla autenticada
rota.

Lo que falta: las filas seleccionables de comunidad/vivienda/empresa del
diseño Stitch, el interruptor "Preguntarme siempre al iniciar sesión" y el
número de colegiación ficticio ("Colegiación Oficial nº 4.192") no se
construyeron -- no hay comunidades, viviendas, roles ni membresías que
alimenten una lista real (ver punto 1), no hay ninguna señal de cliente que
un interruptor de "recordar elección" pudiera pilotar honestamente sin nada
que recordar, y el número de colegiación es texto de una persona de
demostración, no un dato real del usuario. En su lugar, `testID="portal-empty-state"`
muestra honestamente "Todavía no perteneces a ninguna comunidad ni empresa"
y el botón principal ("Acceder" / "Continuar al panel",
`testID="portal-primary-cta"`) se renderiza deshabilitado
(`accessibilityState={{disabled: true}}`) con `accessibilityHint` explicando
el motivo, en vez de fingir una lista o una acción que no lleva a ningún
sitio.

Hito: `PRD_go.md` §10, fila M1 -- misma cita que el punto 1. Cuando existan
comunidades/viviendas/roles/membresías, `portal-empty-state` se sustituye por
la lista real y el botón principal deja de estar deshabilitado.
