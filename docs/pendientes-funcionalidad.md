# Pendientes de funcionalidad frente al diseño

Estado a 2026-09-15. El rediseño del login (móvil + escritorio, Stitch
proyectos `11075381530582947267` y `14138416730329310203`) y el flujo
completo de recuperación de contraseña ya están implementados y con backend
real. Lo que sigue documenta los elementos visuales del diseño que se
construyeron fieles al Stitch pero deliberadamente deshabilitados, y por qué,
para que nadie los confunda con un olvido.

## 1. Selector de perfil de usuario en el login

Pantalla Stitch "Login web - Vecingest"
(`projects/14138416730329310203/screens/5f83c21f55d9493da67cc330fa8528d4`).

Lo que se ve hoy: un control segmentado de tres opciones (Vecino /
Administrador / Empresa) en el panel derecho del login de escritorio,
`app/src/screens/LoginScreen.tsx`, `testID="login-profile-selector"`.
Ninguna opción se muestra como "seleccionada" -- las tres se renderizan con
el mismo estilo neutro -- porque hacerlo implicaría que el control afecta al
inicio de sesión, y no es así.

Lo que falta: `POST /v1/auth/login` (`api/openapi/openapi.yaml:422`, tipo
`LoginRequest` en `packages/shared/src/schemas/index.ts`) solo acepta
`email`, `password`, `platform` y `device_name` -- no hay parámetro de rol.
Elegir un perfil/contexto real es competencia de una pantalla **posterior**
al login ("Elegir dónde entrar", proyecto APP `11075381530582947267`,
pantalla `95e97f53757e419bb498acc29cf049e9`; equivalente de escritorio,
"Selector de contexto", proyecto WEB, pantalla
`537dd208d9d649b6a56afa3f29f51de1`), que necesita comunidades, viviendas,
roles y membresías -- ninguno de los cuales existe todavía (sin tablas de
dominio, sin endpoints).

Hito: `PRD_go.md` §10, fila M1 -- "Comunidades, viviendas, invitaciones,
portales vacíos por rol" / "Un admin crea una comunidad e invita a un vecino
que entra en su portal".

Comportamiento actual del control mientras tanto: completamente
deshabilitado (`accessibilityState={{disabled: true}}`), con
`accessibilityHint` explicando por qué, y sin ningún estado visual de
"seleccionado" que sugiera que hace algo.

## 2. Código de invitación

Pantallas Stitch: enlace de pie "Tengo un código de invitación" en "Iniciar
sesión" (móvil, proyecto `11075381530582947267`) y su equivalente en "Login
web - Vecingest" (escritorio, proyecto `14138416730329310203`, pantalla
`5f83c21f55d9493da67cc330fa8528d4`).

Lo que falta: ningún endpoint de `api/openapi/openapi.yaml` acepta un
código de invitación todavía. Las invitaciones son un concepto de M1 (misma
fila de `PRD_go.md` §10 citada arriba: "invitaciones").

Comportamiento actual: `testID="login-invitation-link"`, no pulsable
(`Pressable disabled`, `accessibilityState={{disabled: true}}`,
`accessibilityLabel="Código de invitación, no disponible todavía"`),
estilo apagado, con la leyenda "Próximamente" junto al texto. En escritorio
se renderiza además con el tratamiento de píldora con borde y la leyenda
"¿Primera convocatoria o registro?" que trae el propio diseño; en móvil se
mantiene el tratamiento de texto simple. Ambos comparten el mismo
`testID` y el mismo estado deshabilitado.

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

## 4. Pantallas posteriores al login: no se construyen todavía

Se decidió explícitamente **no** construir "Elegir dónde entrar" (proyecto
APP `11075381530582947267`, pantalla `95e97f53757e419bb498acc29cf049e9`) ni
su equivalente de escritorio "Selector de contexto" (proyecto WEB, pantalla
`537dd208d9d649b6a56afa3f29f51de1`) como rutas reales de la aplicación.

Motivo: hoy no existe ningún punto de navegación en la app que lleve a esas
pantallas -- un login exitoso resuelve una única sesión, sin elección de
comunidad/rol -- y tampoco existe el modelo de datos que las alimentaría
(comunidades, viviendas, roles, membresías; ver punto 1). Construirlas ahora
sería adelantar trabajo de M1 sin nada que las conecte ni las sostenga. Esta
nota deja constancia de que la ausencia es una decisión, no un olvido.
