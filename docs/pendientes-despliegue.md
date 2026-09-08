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

## Bloqueantes antes de que entre ningún vecino real

### 1. Cerrar los tres puertos publicados en el cortafuegos

`site` (24221), `web` (38043) y `api` (34246) se publican en `0.0.0.0`. Hoy se
puede llegar a los tres por la IP del servidor, saltándose nginx, Cloudflare,
el WAF y el certificado. Bajo el montaje anterior de red compartida entre
contenedores esos puertos nunca fueron alcanzables desde internet; ahora sí lo
son, y solo el cortafuegos del host lo impide.

```bash
sudo ufw deny 24221/tcp
sudo ufw deny 38043/tcp
sudo ufw deny 34246/tcp
```

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
Docker internamente. Hoy responde desde internet. Devuelve únicamente
`{"ok":true}`, así que no filtra nada todavía, pero es un mapa gratuito de las
dependencias en cuanto alguien le añada detalle, y una palanca para forzar
consultas a la base de datos desde fuera.

```nginx
location = /v1/health/ready { return 404; }
```

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

### 8. Quitar `MIGRATIONS_DATABASE_URL` del contrato de configuración

Hoy funciona porque el compose la deriva conectándose como superusuario y
asumiendo el rol (`?options=-c%20role%3Dvecingest_owner`), lo cual está
verificado: las 24 tablas del esquema quedan a nombre de `vecingest_owner` y el
rol sigue siendo `NOLOGIN`. Pero sigue siendo una variable en el conjunto de
requisitos de `CommandMigrate` que ningún operador debería tener que conocer.

Lo limpio es derivar la conexión del esquema desde `BOOTSTRAP_DATABASE_URL`
dentro del código y borrar la variable de `config.go` y del `Holder`.

### 9. Arreglar el falso verde de `migrate_test.go`

`api/cmd/vecingest/migrate_test.go:50-51` apunta `BOOTSTRAP_DATABASE_URL` y
`MIGRATIONS_DATABASE_URL` **al mismo DSN de superusuario**. Por eso el test
nunca se conecta como el owner y pasaba en verde mientras producción se caía
con `password authentication failed for user "vecingest_owner"`.

El test que hay que añadir no es "que migre sin error", sino que consulte el
catálogo y compruebe que:

- `vecingest_owner` conserva `rolcanlogin = false`, y
- las tablas creadas por el set de esquema pertenecen a `vecingest_owner`
  (`pg_class.relowner` contra `pg_roles`), no al superusuario.

Sin la segunda comprobación, cualquier arreglo futuro puede degradar en
silencio a "las migraciones corren como superusuario y todo es suyo", que
rompe la puerta del rol restringido de §10.1 sin que nadie se entere.

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
