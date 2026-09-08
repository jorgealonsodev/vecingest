---
name: Vecingest
colors:
  surface: '#fbf9f2'
  surface-dim: '#dcdad3'
  surface-bright: '#fbf9f2'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#f5f4ed'
  surface-container: '#f0eee7'
  surface-container-high: '#eae8e1'
  surface-container-highest: '#e4e2dc'
  on-surface: '#1b1c18'
  on-surface-variant: '#424751'
  inverse-surface: '#30312c'
  inverse-on-surface: '#f3f1ea'
  outline: '#727782'
  outline-variant: '#c1c6d2'
  surface-tint: '#2b609c'
  primary: '#024883'
  on-primary: '#ffffff'
  primary-container: '#2b609c'
  on-primary-container: '#c4daff'
  inverse-primary: '#a4c9ff'
  secondary: '#535f6e'
  on-secondary: '#ffffff'
  secondary-container: '#d4e1f3'
  on-secondary-container: '#586473'
  tertiary: '#44407c'
  on-tertiary: '#ffffff'
  tertiary-container: '#5c5896'
  on-tertiary-container: '#dad5ff'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#d4e3ff'
  primary-fixed-dim: '#a4c9ff'
  on-primary-fixed: '#001c39'
  on-primary-fixed-variant: '#024883'
  secondary-fixed: '#d7e4f5'
  secondary-fixed-dim: '#bbc8d9'
  on-secondary-fixed: '#101c29'
  on-secondary-fixed-variant: '#3c4856'
  tertiary-fixed: '#e3dfff'
  tertiary-fixed-dim: '#c5c0ff'
  on-tertiary-fixed: '#17114e'
  on-tertiary-fixed-variant: '#433f7b'
  background: '#fbf9f2'
  on-background: '#1b1c18'
  surface-variant: '#e4e2dc'
typography:
  headline-lg:
    fontFamily: bricolageGrotesque
    fontSize: 28px
    fontWeight: '600'
    lineHeight: 34px
    letterSpacing: -0.01em
  headline-md:
    fontFamily: bricolageGrotesque
    fontSize: 22px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  headline-sm:
    fontFamily: ibmPlexSans
    fontSize: 17px
    fontWeight: '500'
    lineHeight: 24px
  body-md:
    fontFamily: ibmPlexSans
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md-medium:
    fontFamily: ibmPlexSans
    fontSize: 16px
    fontWeight: '500'
    lineHeight: 24px
  body-sm:
    fontFamily: ibmPlexSans
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 18px
  label-md:
    fontFamily: ibmPlexSans
    fontSize: 13px
    fontWeight: '500'
    lineHeight: 18px
  label-sm:
    fontFamily: ibmPlexSans
    fontSize: 11px
    fontWeight: '500'
    lineHeight: 14px
    letterSpacing: 0.02em
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
---

# Vecingest — Sistema de diseño para gestión de comunidades de propietarios
 
## Identidad y principios
- **Producto:** app móvil de gestión de comunidades de propietarios en España (Vecingest).
- **Perfiles:** vecino, administrador de fincas y empresa de servicios.
- **Tono:** herramienta de gestión seria y funcional, comparable a la banca digital de primer nivel.
- **Reglas visuales estrictas:** sin ilustraciones decorativas, sin gradientes, sin sombras proyectadas (`box-shadow: none`), sin emojis.
- **Voz y lenguaje:** todo en castellano, texto en frase (no en Título), sin signos de exclamación, sin fórmulas de cortesía innecesarias como "por favor".
- **Botones con verbo primero:** "Enviar incidencia", "Asignar", "Fichar salida", "Registrar voto", "Descargar acta".
- **Un solo color de acento:** el azul `#185FA5` es el único color interactivo. No aparece ningún otro azul en la interfaz.
## Paleta de color
- **Fondo de pantalla:** `#F1EFE8`
- **Superficie (tarjetas, campos, barras de navegación):** `#FFFFFF`
- **Texto principal:** `#0F2A4A`
- **Texto secundario y descriptivo:** `#5F6B7A`
- **Líneas divisorias y bordes:** `#D6DAD5`
- **Acento interactivo (único botón primario por pantalla, tab activa, enlaces):** `#185FA5`
- **Fondo de acento suave:** `#E6F1FB`
- **Éxito:** texto `#0F6E56` sobre fondo `#E1F5EE`
- **Aviso:** texto `#854F0B` sobre fondo `#FAEEDA`
- **Error:** texto `#A32D2D` sobre fondo `#FCEBEB`
- **Chip legal (referencias a la Ley de Propiedad Horizontal):** texto `#26215C` sobre fondo `#EEEDFE`
- **Regla:** el texto sobre un fondo de color usa siempre el tono más oscuro de esa misma familia, nunca negro.
### Chips de estado de incidencia (idénticos en todos los perfiles)
- **Abierta:** texto `#2C2C2A` sobre `#F1EFE8`
- **Asignada:** texto `#0C447C` sobre `#E6F1FB`
- **En curso:** texto `#26215C` sobre `#EEEDFE`
- **Resuelta:** texto `#04342C` sobre `#E1F5EE`
- **Cerrada:** texto `#2C2C2A` sobre `#D3D1C7`
- **Rechazada:** texto `#501313` sobre `#FCEBEB`
### Chips de prioridad
- **Urgente:** texto `#501313` sobre `#FCEBEB`
- **Alta:** texto `#412402` sobre `#FAEEDA`
- **Normal:** texto `#2C2C2A` sobre `#F1EFE8`
- **Baja:** texto `#5F5E5A` sobre `#F1EFE8`
### Otros chips
- **Reserva:** confirmada (verde), pendiente (ámbar), cancelada (gris).
- **Factura:** borrador (gris), enviada (azul `#0C447C` sobre `#E6F1FB`), aceptada (ámbar), pagada (verde), disputada (rojo), rectificativa (gris con borde).
- **Empresa:** verificada (verde), pendiente de verificación (ámbar), suspendida (rojo).
### Sistema de votaciones
- **A favor:** `#1D9E75`
- **En contra:** `#D85A30`
- **Abstención:** `#B4B2A9`
- **Privado de voto (art. 15.2 LPH, por deudas vencidas):** rojo `#A32D2D` con icono de prohibido; las opciones de voto se muestran deshabilitadas.
## Tipografía
- **Titulares:** `Bricolage Grotesque`, peso 600.
- **Texto general y datos:** `IBM Plex Sans`, únicamente pesos 400 y 500.
- **Escala tipográfica:**
  - 28px: titulares principales de pantalla.
  - 22px: títulos de sección y de hojas inferiores.
  - 17px: títulos de tarjeta y cabeceras de lista.
  - 16px: cuerpo de texto, campos de formulario, botones.
  - 13px: texto secundario, metadatos, chips, etiquetas de tabs.
  - 11px: marcas temporales, microetiquetas, referencias legales.
  - *Regla estricta:* ningún texto por debajo de 11px.
- **Cifras numéricas:** cuotas de participación, importes, porcentajes y saldos con cifras tabulares (`font-variant-numeric: tabular-nums`).
## Forma, espaciado y maquetación
- **Bordes:** 1px sólido `#D6DAD5`. Sin sombras.
- **Radios:**
  - Botones y campos de entrada: 8px.
  - Tarjetas y contenedores de bloque: 12px.
  - Chips y distintivos: píldora (`rounded-full`).
- **Espaciado:**
  - Márgenes laterales de pantalla: 16px.
  - Relleno interior de tarjetas: 16px.
  - Área táctil mínima: 44px de alto para cualquier elemento interactivo.
  - Listas densas: filas separadas por un divisor de 1px `#D6DAD5`, no tarjetas flotantes apiladas.
- **Iconografía:** iconos de línea geométrica (estilo Material Symbols Outlined), trazo uniforme, 24px por defecto y 20px en chips y metadatos.
## Componentes y patrones de interfaz
 
1. **Navegación**
   - **Cabecera de pantalla:** título alineado a la izquierda, botón de atrás o cerrar a la izquierda, como máximo una acción contextual a la derecha.
   - **Barra de tabs inferior:** 5 ítems con icono y etiqueta corta; ítem activo en `#185FA5`, el resto en `#5F6B7A`.
     - Perfil vecino: Inicio · Incidencias · Avisos · Documentos · Más.
     - Perfil empresa: Bandeja · Tareas · Fichaje · Facturas · Perfil.
     - Perfil administrador en móvil: menú lateral (drawer) con Panel · Comunidades · Incidencias · Avisos · Juntas · Despacho.
2. **Tarjeta de incidencia**
   - Título en 17px/500; ubicación y tiempo relativo en 13px color secundario; chip de estado en píldora a la derecha.
   - Variante urgente sin aceptar (bandeja de empresa): borde de 1px rojo `#A32D2D` y botones "Aceptar" y "Rechazar" dentro de la tarjeta.
3. **Formularios**
   - Etiqueta en 13px/500 encima del campo; campo de 44px de alto, radio 8px, borde `#D6DAD5`, borde `#185FA5` al enfocar; mensaje de error debajo en `#A32D2D`; exactamente un botón primario al pie.
   - Casillas de consentimiento nunca marcadas por defecto.
   - Selectores con el valor actual visible y chevron a la derecha.
4. **Botones**
   - Primario: fondo `#185FA5`, texto blanco, 44px de alto, radio 8px. Uno por pantalla.
   - Secundario: fondo blanco, borde `#0F2A4A`, texto `#0F2A4A`.
   - Terciario: solo texto en `#185FA5`.
   - Destructivo: fondo `#A32D2D`, texto blanco (solo en confirmaciones).
   - Flotante: círculo de 56px en `#185FA5` con icono "+", abajo a la derecha.
5. **Banda de aviso a ancho completo**
   - Directamente bajo la cabecera; fondo suave informativo (`#E6F1FB`), ámbar (`#FAEEDA`), rojo (`#FCEBEB`) o gris para "Sin conexión"; texto conciso en 13px y, si procede, un enlace de acción al final.
6. **Barra de resultado de votación**
   - Segmentos proporcionales contiguos para a favor (`#1D9E75`), en contra (`#D85A30`) y abstención (`#B4B2A9`), 8px de alto, con una marca vertical para el umbral legal exigido (1/3, mayoría, 3/5 o unanimidad, según el apartado del art. 17 LPH). Siempre se muestran dos barras: por cuotas y por cabezas, con el umbral escrito ("necesario 33,34 %", "necesario 8 de 24").
7. **Línea de tiempo vertical**
   - Trazo de 2px `#D6DAD5` con nodos de estado; cada entrada con título en 500, autor y hora en 13px secundario, y cita opcional del comentario.
8. **Estado vacío**
   - Icono de línea de 40px, titular en 22px como invitación ("Crea tu primera incidencia"), una única línea explicativa en secundario y un botón con verbo inicial.
9. **Chip legal**
   - Píldora con fondo `#EEEDFE` y texto `#26215C` en 11px/500, siempre junto al punto del orden del día: "Art. 17.1 · 1/3 del total", "Art. 17.7 · mayoría", "Art. 17.6 · unanimidad". Variante gris (`#F1EFE8` / `#2C2C2A`) para "Coste solo a quien vote sí".
10. **Campo OTP**
    - Seis casillas de 44×52px con radio 8px y borde `#D6DAD5`; casilla activa con borde `#185FA5`; debajo, en 13px, texto de reenvío con cuenta atrás y contador de intentos restantes.
11. **Botón de fichaje**
    - Círculo de 120px con borde de 3px y fondo suave: verde (`#1D9E75` sobre `#E1F5EE`) para "Fichar entrada", rojo (`#D85A30` sobre `#FCEBEB`) para "Fichar salida"; icono de 28px y etiqueta 13px/500 dentro; encima, contador de jornada en 28px con cifras tabulares.
12. **Opciones de voto**
    - Tres filas a ancho completo de 56px ("A favor", "En contra", "Abstención") con icono a la izquierda y radio a la derecha; la seleccionada con borde y fondo de su color (verde, coral o gris). Deshabilitadas y atenuadas cuando el usuario está privado de voto.
13. **Semáforo de documentación (empresa)**
    - Punto de 10px a la izquierda de cada documento: verde vigente, ámbar caduca en menos de 30 días, rojo caducado, gris no subido; fecha de caducidad en 13px secundario.
14. **Hoja inferior**
    - Fondo blanco, radio superior 12px, asa de 32×4px, título en 22px, contenido y un botón primario al pie. Se usa para motivos obligatorios (rechazar, reabrir) y confirmaciones.
15. **Calendario de reservas**
    - Selector de día horizontal; rejilla de franjas de una hora: libre (blanco con borde), ocupada (gris `#E4E2DC` con texto "Ocupada", sin nombres), bloqueo del administrador (rayado diagonal gris con etiqueta), seleccionada (fondo `#E6F1FB`, borde `#185FA5`).
## Datos de ejemplo (usar siempre los mismos)
- Comunidad: "C/ Mayor 12, Irun". Vivienda: "3.º B", cuota 4,25 %. Despacho: "Administración Ríos". Otras comunidades: "Av. Iparralde 4", "Pl. Urdanibia 2".
- Empresa: "Ascensores Norte S.L." Trabajadores: "Mikel Etxeberria", "Aitor Zabala", "Leire Otaegi".
- Propietarios: "Ane Larrañaga" (presidenta), "Jon Larrañaga", "Miren Aduriz (2.º A)".
- Incidencias: "Ascensor parado", "Gotera en el portal", "Bombilla garaje", "Puerta garaje no cierra", "Limpieza escalera B".
- Junta: "Junta ordinaria · 24 sep · 19:00 · 2.ª convocatoria 19:30 · híbrida".
- Importes: recibo mensual "84,50 €", derrama "120,00 €", factura "484,00 €".
