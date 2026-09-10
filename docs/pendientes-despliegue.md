# Pendientes tras el primer despliegue

Estado a 2026-09-08. El stack está desplegado en Portainer, construido en el
propio servidor desde GitHub, y los tres dominios responden con certificado
válido:

```
https://vecingest.xdev.es/                    HTTP 200
https://app.vecingest.xdev.es/                HTTP 200
https://api.vecingest.xdev.es/v1/health/live  HTTP 200  {"ok":true}
```

Lo que sigue no bloquea el funcionamiento, pero sí queda por hacer.

## Antes de que entre ningún vecino real

### 1. Los puertos publicados dependen solo del cortafuegos de Oracle

`site` (24221), `web` (38043) y `api` (34246) se publican en `0.0.0.0`, pero
**no son alcanzables desde internet**. Comprobado desde fuera: los tres dan
tiempo de espera agotado, mientras 22 y 443 responden.

Quien los bloquea es la *security list* de la VCN, en la consola de Oracle
Cloud. No es el cortafuegos de la máquina, y la diferencia importa:

```
-A INPUT -p tcp --dport 22 -j ACCEPT
-A INPUT -j REJECT --reject-with icmp-host-prohibited
```

Ese `REJECT` no protege un puerto publicado por Docker. El tráfico hacia un
puerto publicado se redirige en `nat/PREROUTING` y continúa por `FORWARD`, sin
pasar nunca por `INPUT` -- por eso el 443 del proxy responde pese a esa regla.
Añadir reglas de ufw (que además no está instalado) tendría el mismo efecto
nulo: es el fallo clásico de ufw con Docker, y deja reglas escritas que no
bloquean nada.

Consecuencia práctica: la única barrera está fuera del servidor y fuera de este
repositorio. Abrir esos puertos en la consola de Oracle los expone al mundo sin
que nada en la máquina lo impida, y desde ahí se llega a la API saltándose
nginx, el certificado, Cloudflare y el `return 404` del punto 4.

Si en algún momento se quiere defensa en profundidad en la propia máquina, no
sirve `INPUT`: hay que escribir las reglas en la cadena `DOCKER-USER` (hoy
vacía), o publicar los puertos en una dirección concreta en vez de en
`0.0.0.0`. Ojo con lo segundo: `127.0.0.1` **rompería el proxy**, porque NPM
corre en redes bridge (172.18.0.2 y 172.19.0.3) y para él `127.0.0.1` es su
propio contenedor, no el host.

### 2. `SMTP_URL` real

Ahora mismo vale `smtps://localhost:465`, un valor inerte que solo sirve para
que la API arranque. El parser (`api/internal/mail/mail.go:220-270`) exige
esquema, host y puerto numérico, pero el usuario y la contraseña son
opcionales, y la conexión no se abre hasta el envío
(`DialAndSendWithContext`), así que un valor sintácticamente válido arranca sin
protestar y falla en silencio al enviar.

Con este valor están rotos: el restablecimiento de contraseña, el aviso de
bloqueo por intentos fallidos y, cuando llegue, el OTP de voto y firma. El
correo no es un canal de avisos en este proyecto, es un canal crítico.

Si la contraseña lleva `@`, `/`, `:`, `#` o algo fuera de ASCII, hay que
codificarla en porcentaje o la URL se parte.

### 3. `PROXY_IP=172.30.0.1`

Medido en el despliegue real: es la pasarela de la red del contenedor `api`,
no la `172.17.0.1` que se puso para el primer arranque. Con el valor
equivocado la API no confía en la cabecera del proxy y registra la dirección
de Docker para todas las peticiones, con lo que el bloqueo por IP tras cinco
intentos fallidos deja de distinguir entre un atacante y un vecino.

Se vuelve a medir así:

```bash
docker inspect -f '{{range .NetworkSettings.Networks}}{{.Gateway}} {{end}}' vecingest-api-1
```

## Endurecimiento

### 4. ~~No exponer `/v1/health/ready` hacia fuera~~ — HECHO (2026-09-08)

Resuelto en el bloque *Advanced* del host `api.vecingest.xdev.es` en Nginx Proxy
Manager. Verificado desde fuera: `live` sigue devolviendo 200 y `ready` devuelve
404, con y sin barra final. Los cinco contenedores siguen sanos: el healthcheck
de Docker consulta `ready` dentro del contenedor, sin pasar por nginx.

El PRD (§9, "Endpoints de salud separados") dice que hacia fuera solo se expone
`live`; `ready` es el que consulta base de datos y almacenamiento y lo usa
Docker internamente. Estuvo respondiendo desde internet hasta este cambio.
Devolvía únicamente `{"ok":true}`, así que no llegó a filtrar nada, pero era un
mapa gratuito de las dependencias en cuanto alguien le añadiera detalle, y una
palanca para forzar consultas a la base de datos desde fuera.

```nginx
location = /v1/health/ready {
    return 404;
}
```

El `=` no es opcional: hace coincidencia exacta y en nginx eso tiene prioridad
sobre el `location /` que genera Nginx Proxy Manager para el `proxy_pass`. Sin
el `=` sería un prefijo y competiría con la regla general en vez de imponerse.

### 5. Bucket lock de 30 días sobre `vecingest-backups`

R2 no ofrece permiso de solo escritura: sus presets son Admin Read & Write,
Admin Read only, Object Read & Write y Object Read only. El token de copias es
por tanto Object Read & Write acotado a ese bucket, y **puede borrar**. La
inmutabilidad frente a un borrado malicioso la tiene que dar un bucket lock con
retención de 30 días, no el permiso del token. Ver §11 y la lista de puertas
de §10.1.

### 6. Rotar las credenciales de R2

Las claves secretas de los tokens `vecingest-api` y `vecingest-backups` se
pegaron en una conversación y deben considerarse comprometidas. Borrar ambos
tokens y recrearlos con la misma configuración: Object Read & Write, acotado
cada uno a su bucket, TTL siempre, sin filtro de IP.

El filtro de IP es tentador para el token de la API, pero rompería las subidas:
las URL prefirmadas se firman con esas credenciales y quien las usa es el
navegador del vecino, desde su propia dirección.

### 7. CORS del bucket `vecingest`

El cliente sube los ficheros directamente a R2, no a través de la API (§7.7),
así que el navegador hace un PUT a otro dominio y sin política CORS el bucket
lo rechaza. No está documentado en el PRD: es un hueco de la especificación.

```json
[
  {
    "AllowedOrigins": ["https://app.vecingest.xdev.es"],
    "AllowedMethods": ["PUT", "GET"],
    "AllowedHeaders": ["content-type", "content-length"],
    "ExposeHeaders": ["etag"],
    "MaxAgeSeconds": 3600
  }
]
```

Solo `PUT` y `GET`. El borrado lo hace la API con sus credenciales, nunca el
cliente. El bucket `vecingest-backups` no necesita CORS: ningún navegador lo
toca.

## Deuda técnica

### 8. ~~Quitar `MIGRATIONS_DATABASE_URL` del contrato de configuración~~ — HECHO (2026-09-10)

La variable ya no existe en ningún sitio: ni en `config.go` (ni la constante
`envMigrationsDatabaseURL` ni la entrada en el conjunto de requisitos de
`CommandMigrate`), ni en `docker-compose.yml`, ni en `env.example`, ni en
`PRD_go.md` (§8.1, §8.2, §1107). El binario deriva la conexión del esquema en
tiempo de ejecución a partir de `BOOTSTRAP_DATABASE_URL`
(`api/internal/platform/migrate.SchemaDSN`), añadiendo el mismo parámetro de
arranque de libpq que antes ponía el compose (`options=-c%20role%3Dvecingest_owner`,
ahora construido dentro del binario en vez de tecleado en el YAML) para que
**toda** conexión que abre el pool del set de esquema asuma `vecingest_owner`
desde el momento de conectar, no solo la primera.

Esa distinción importó de verdad: una primera versión de `SchemaDSN` montaba el
parámetro con `url.Values.Encode()`, que codifica el espacio como `+`, y pgconn
no traduce `+` de vuelta a espacio en `options` (a diferencia de `net/url` al
decodificar) — el intento de migrar fallaba con
`unrecognized configuration parameter "+role"`. El arreglo fue construir el
parámetro ya porcentaje-codificado (`%20`, `%3D`), igual que el literal que ya
funcionaba en `docker-compose.yml`.

`env.example` y `PRD_go.md` §8.2 se comprobaron idénticos byte a byte con
`diff` tras el cambio.

Único punto que no se pudo tocar en esta sesión: el campo `MigrationsDatabaseURL`
y su accessor en `api/internal/config/secrets/holder.go` siguen declarados
(ya no los rellena nadie, así que quedan siempre vacíos y sin uso real). El
sandbox del agente que hizo este cambio deniega lectura y escritura de
cualquier ruta bajo `**/secrets/*`, así que ese borrado concreto lo tiene que
hacer una persona con acceso directo al fichero.

Verificado de punta a punta contra un Postgres 17 real levantado solo para
esta comprobación: `go run ./cmd/vecingest migrate` con únicamente las
variables que hoy declara `CommandMigrate` (sin `MIGRATIONS_DATABASE_URL` en
ningún sitio) termina con `migrate: applied`, y el catálogo confirma las 24
tablas del esquema a nombre de `vecingest_owner`, `goose_db_version_bootstrap`
a nombre del superusuario, y `vecingest_owner` con `rolcanlogin = f`.

### 9. ~~Arreglar el falso verde de `migrate_test.go`~~ — HECHO (2026-09-10)

`TestRunMigrate_AppliesBootstrapAndSchemaSets` ya no fija
`MIGRATIONS_DATABASE_URL` en absoluto (la variable no existe); solo pone
`BOOTSTRAP_DATABASE_URL`, y `runMigrate` deriva la conexión del esquema él
mismo, exactamente como en producción.

Se añadió `TestRunMigrate_SchemaSetOwnershipIsRestricted`, que hace lo que
pedía este punto: tras un `runMigrate` real contra un contenedor
Testcontainers, consulta el catálogo y comprueba que:

- `vecingest_owner` conserva `rolcanlogin = false`, y
- toda tabla de `public` (`pg_class.relowner` contra `pg_roles`) pertenece a
  `vecingest_owner`, **salvo** `goose_db_version_bootstrap`, para la que se
  afirma explícitamente que su dueño es el superusuario — no se excluye en
  silencio, se comprueba el valor exacto.

Prueba de que el test realmente detecta la regresión, no solo que pasa: se
rompió `SchemaDSN` a mano (devolviendo el DSN del superusuario sin el
parámetro `options`, simulando "las migraciones corren como superusuario y
todo es suyo") y se ejecutó el test. Falló señalando exactamente
`goose_db_version_schema`, `river_job`, `river_leader`, `river_queue`,
`river_migration` y `river_notification` como propiedad de `postgres` en vez
de `vecingest_owner` — las tablas creadas por los ficheros `.sql` del set de
esquema (`users`, `audit_log`, etc.) no aparecieron en el fallo porque cada
uno de esos ficheros ya emite su propio `SET ROLE vecingest_owner;`, pero ni
la tabla de versión que crea `goose` internamente ni las migraciones de River
(vía `rivermigrate`, que no emite ese `SET ROLE`) tienen esa protección — de
ahí que la comprobación 2 de este punto sea imprescindible y no baste con la
del punto 8. Se restauró `SchemaDSN` y se confirmó que ambos tests vuelven a
pasar.

`assertAppRwNotOwnerMember` (`api/migrations/bootstrap/00001_roles.go`) sigue
en verde: comprueba membresía de rol, no propiedad de tabla, y por eso no
habría detectado esta regresión por sí sola.

### 10. Un test que ejecute el contenedor, no solo el binario

Los tres primeros fallos del despliegue comparten una única causa: **la imagen
nunca se había ejecutado**. La batería de tests corre el binario Go
directamente, así que nada ejercitaba el `ENTRYPOINT`, ni la arquitectura de
compilación, ni el entorno resultante del compose.

- `GOARCH=amd64` fijado a mano contra un servidor `aarch64` → `exec format
  error`. La imagen *decía* ser arm64 porque la base distroless es
  multiarquitectura; solo el binario de dentro estaba mal.
- `ENTRYPOINT ["/vecingest"]` más `command: ["/vecingest", "serve"]` →
  `unknown subcommand "/vecingest"`.
- `<<:` de YAML es una fusión **superficial**: el `environment:` propio de un
  servicio sustituye entero el del ancla. `api` y `worker` arrancaron con
  `GOMEMLIMIT` como única variable.

Un test que levante el contenedor y compruebe que responde habría cazado los
tres. Es la misma familia que el hallazgo anterior de que `serve` no tenía
tarea de implementación entre 170 tareas.

### 11. Regla de `lintcompose` contra el `environment` machacado

El punto anterior es detectable estáticamente: ningún servicio que use el ancla
`x-app-image` debe declarar su propia clave `environment:`. Hoy el fichero lo
advierte en un comentario, que es exactamente la clase de protección que se
pierde en el siguiente cambio.

### 12. El linting de TypeScript no analiza nada

`turbo run lint` informa **4/4 correcto** sin revisar una sola línea de
TypeScript. Los tres paquetes de frontend tienen el mismo script:

```
app             lint = echo 'app: lint not yet configured (Phase 11)' && exit 0
packages/shared lint = echo 'packages/shared: lint not yet configured (Phase 11)' && exit 0
site            lint = echo 'site: lint not yet configured' && exit 0
```

El `(Phase 11)` de esos mensajes es una promesa que nunca llegó a ser tarea:
la fase 11 de `tasks.md` solo cablea los trabajos de Go y los escáneres de
seguridad, y no hay ninguna tarea que pida ESLint, Biome ni equivalente. Así
que no es una tarea cerrada en falso, es una que no existe.

El lado Go sí se analiza de verdad (`golangci-lint` desde `api/`). El lado
TypeScript, que es todo el frontend y el cliente generado, no tiene ninguna
comprobación estática más allá de `tsc --noEmit`.

Mismo patrón que los demás falsos verdes de este proyecto: una comprobación
que informa de éxito sin haber mirado nada. Encontrado al arreglar los colores
del tema, cuando la verificación decía "lint 4/4 correcto" sobre un cambio que
tocaba precisamente TypeScript.

## Sin implementar todavía (no son fallos)

Estas variables están declaradas en el PRD y previstas en la configuración,
pero **ninguna línea de Go las lee** a fecha de hoy. Verificado con `grep`:

| Variable | Estado |
| --- | --- |
| `R2_*` | El almacenamiento de ficheros no está implementado en M0 |
| `TSA_URL` | El sellado de tiempo tampoco; además tiene valor por defecto en el compose |
| `SENTRY_DSN` | El SDK no está ni en `api/go.mod` ni en `app/package.json` |
| `TURNSTILE_SECRET` | La API lo lee al `Holder` pero nadie lo usa; la Site Key ni siquiera tiene variable donde aterrizar |
| `EXPO_ACCESS_TOKEN` | Cero apariciones en todo el repositorio; es para compilar con EAS, nunca una variable del stack |

Antes de implementar Turnstile hará falta decidir dónde aterriza la Site Key,
que es pública y tiene que llegar al cliente.

## Lo que queda de la Fase 14 (Checkpoint B) y quién tiene que hacerlo

Estado a 2026-09-08. El bloqueo de infraestructura de la Fase 14
(`openspec/changes/m0-foundation/tasks.md`) queda levantado: el servidor
existe, los cinco contenedores están sanos y 13.2, 14.1, 14.2 y 14.6 quedan
cerrados con evidencia real (`docs/security/evidence/checkpoint-b/2026-09-08-server-verification.md`).
Lo que sigue **no lo puede cerrar un agente**: necesita a una persona.

### 1. ~~Crear la cuenta de superadmin~~ — HECHO (2026-09-10)

Ya no bloquea nada. La base pasó de cero usuarios a dos:

- `admin@vecingest.xdev.es` — superadmin.
- `test@vecingest.xdev.es` — usuario normal, creado con el mismo subcomando
  (para pasar por la validación contra HIBP y el hasheo Argon2id) y degradado
  después con `UPDATE users SET is_superadmin = false`. Su campo `name` dice
  "Superadmin"; es solo cosmético.

Hizo falta un usuario normal porque **un superadmin no puede entrar por
`/v1/auth/login`**: `api/internal/http/handlers/auth_login.go:52` rechaza esas
cuentas a propósito y devuelve el mismo `AUTH_INVALID_CREDENTIALS` que una
contraseña equivocada, para no revelar que la cuenta existe y es privilegiada.
Los superadmins entran por `/v1/auth/superadmin/login`, con TOTP obligatorio.

Tampoco se usó `seed`: se niega a correr fuera de `development`/`staging`, y su
contraseña `vecingest-dev-2024` es una constante pública del repositorio, así
que sembrar un despliegue accesible desde internet habría publicado tres
accesos válidos.

Las dos contraseñas se escribieron en una conversación, así que son
desechables y hay que cambiarlas antes de que el despliegue contenga algo real.

Verificado de punta a punta contra el dominio público: `POST /v1/auth/login`
con `platform: "web"` devuelve 200 con `access_token`, `expires_in: 900` y
`csrf_token`; `GET /v1/me` con ese portador devuelve 200 e `is_superadmin: false`.

### 1b. Lo que sigue haciendo falta para cerrar 14.3 y 14.4

Todavía no existe ningún usuario en el despliegue: nadie ha ejecutado
`vecingest bootstrap-superadmin`. Sin esa cuenta no hay con qué iniciar
sesión (14.3/14.4) ni token con el que autenticar el `k6` de 14.5.

El subcomando lee el email por flag y la contraseña **exclusivamente por
stdin** (nunca por flag ni por variable de entorno; `api/cmd/vecingest/bootstrap_superadmin.go`):

```bash
echo -n 'LA_CONTRASEÑA_QUE_ELIJA_EL_USUARIO' | \
  docker exec -i vecingest-api-1 /vecingest bootstrap-superadmin \
    --email admin@ejemplo.com \
    --password-stdin
```

Puntos importantes, verificados leyendo el código, no supuestos:

- `--password-stdin` es obligatorio y no existe ningún `--password`: si se
  omite, el comando falla explícitamente.
- La contraseña se valida con `PasswordPolicy.Validate(ctx, password, false)`
  — el tercer argumento (`totpActive`) está fijado a `false` en este
  subcomando, así que **siempre exige el suelo de 15 caracteres del PRD
  §5.1** (el de 12 con TOTP no aplica aquí: en el momento del bootstrap
  todavía no hay TOTP configurado). También se comprueba contra HIBP
  (k-anonymity), igual que cualquier alta.
- Es idempotente: ejecutarlo dos veces con el mismo email no duplica el
  usuario ni falla — el propio binario imprime
  `bootstrap-superadmin: already exists, no change`.
- La contraseña la elige la persona que lo ejecuta; nadie debe inventarla
  por ella ni dejarla en ningún fichero de este repositorio.
- El contenedor `api` corre como el usuario `nonroot` (`65532:65532`) y con
  `read_only: true`, así que `docker exec -it ... sh` no sirve (la imagen
  distroless no tiene shell); el `docker exec -i ... /vecingest ...` de
  arriba invoca el binario directamente, sin shell, que es como está
  pensado para funcionar.

### 2. Login manual desde navegador y desde la app (14.3, 14.4)

Con la cuenta ya creada, una persona tiene que:

- Iniciar sesión desde un navegador real contra `https://app.vecingest.xdev.es`
  y confirmar que llega hasta el portal.
- Iniciar sesión desde la app Expo (modo claro y oscuro) contra el mismo
  backend.

Ninguno de los dos tiene sustituto automatizado razonable a este nivel: son
la evidencia del criterio funcional del PRD para M0 ("Login desde web y
móvil contra el servidor desplegado por Portainer").

### 3. `k6` contra `GET /v1/me` (14.5)

Con un token de la cuenta ya creada, hace falta lanzar `k6` contra
`GET /v1/me` en `https://api.vecingest.xdev.es` y comprobar el p95 < 50 ms
del PRD §10.1 (gate item 6). `docker stats --no-stream` ya se capturó como
evidencia de apoyo (ver el fichero de evidencia), pero no sustituye a la
medición de latencia real.

### 4. Confirmar el 2FA de Portainer (14.7, mitad pendiente)

La mitad de `docker.sock` de esta tarea ya está verificada (solo Portainer
lo monta, comprobado en los 16 contenedores del host). El requisito de 2FA
no se puede leer por CLI ni por API sin iniciar sesión, y el puerto HTTPS de
Portainer (9443) no respondía desde fuera del host durante esta comprobación.
Una persona tiene que entrar en la interfaz de Portainer y confirmar en
Settings → Authentication que el 2FA está activo y es obligatorio.

### 5. Cerrar 14.8 una vez lo anterior esté hecho

`docs/security/gates/M0.md` ya refleja todo lo cerrado hasta hoy, pero la
fila del criterio funcional y la mitad de la fila del gate item 6 siguen
abiertas. 14.8 (marcar todas las filas de Checkpoint B en verde) no puede
cerrarse hasta que los puntos 1-4 de arriba estén hechos.
