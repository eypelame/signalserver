# Flujo de Comunicación para Llamadas VoIP 1 a 1

Este documento detalla el flujo de comunicación paso a paso que los clientes deben seguir para establecer y gestionar una llamada VoIP 1 a 1 a través del servidor de señalización. Se incluyen los tipos de mensajes, los nombres de los campos JSON esperados y se hace énfasis en la autenticación y las mejores prácticas.

## 1. Introducción

El servidor de señalización actúa como un intermediario para que dos clientes (A y B) puedan intercambiar la información necesaria para establecer una conexión WebRTC directa (peer-to-peer). El flujo se divide en autenticación, conexión, gestión de estado y el ciclo de vida de la llamada.

## 2. Autenticación del Cliente

Antes de establecer una conexión WebSocket, cada cliente debe autenticarse con un JSON Web Token (JWT) válido.

### 2.1. Generación del JWT (callToken)

El JWT debe ser generado por un servicio de autenticación externo (no parte de este servidor de señalización) y debe contener los siguientes claims:

*   **`sub` (Subject)**: El `userId` único del cliente.
*   **`userName`**: El nombre del cliente que se mostrará a otros usuarios.
*   **`type`**: El rol del usuario en la plataforma. **Valores permitidos: `cliente` (para callers) o `escucha` (para listeners).** Este claim es fundamental para que el servidor identifique los permisos y el flujo de llamada correspondiente. Estos valores están sincronizados con la tabla maestra `user_type` del backend.
*   **`iss` (Issuer)**: El emisor del token (ej. "signal-server").
*   **`exp` (Expiration Time)**: Marca de tiempo de expiración del token. **Se recomienda una duración de 1 hora.** El cliente debe conectar al WebSocket inmediatamente después de obtener este token para recibir actualizaciones de disponibilidad (`clients-list`) y estar listo para llamar. Si la sesión dura más de 1 hora, el cliente deberá renovar el token y reconectar.
*   **`iat` (Issued At)**: Marca de tiempo de emisión del token.
*   **`call_time`**: Duración máxima de la llamada permitida para este usuario en minutos. **Debe ser un número entero (int), no una cadena.** Este claim es crucial para limitar la duración de las llamadas.

**Ejemplo de Payload JWT (antes de firmar):**

```json
{
  "sub": "user123",
  "userName": "Sofía García",
  "iss": "signal-server",
  "exp": 1731446400, // Ejemplo: 2025-11-13 00:00:00 UTC
  "iat": 1731360000, // Ejemplo: 2025-11-12 00:00:00 UTC
  "call_time": 30 // 30 minutos
}
```

El servidor de señalización utiliza `JWT_SECRET` y `JWT_ISSUER` (configurables) para validar la firma y los claims del token.

> [!IMPORTANT]
> **¿Por qué es necesaria la expiración (`exp`) y estar conectado?**
> 1. **Seguridad:** El tiempo de expiración (`exp`) actúa como un candado de seguridad. Evita que si alguien intercepta el token, pueda usarlo de forma permanente para realizar llamadas o acceder al sistema de señalización.
> 2. **Lista en Tiempo Real:** Es fundamental que el cliente se conecte al WebSocket inmediatamente después de obtener el token. Esto permite que el usuario vea quién está disponible o quién cambió su estado al instante (`isAvailable`). Sin esta conexión activa desde el inicio, la lista de contactos no se actualizaría automáticamente en la pantalla del usuario.
> 3. **Duración de Llamadas:** La expiración del token (`exp`) solo se valida al intentar conectar o reconectar. Una vez establecida la conexión, si el usuario inicia una llamada, esta puede durar más que el tiempo de expiración del token (por ejemplo, una llamada de 2 horas con un token que expira en 1 hora), siempre y cuando el usuario tenga saldo suficiente (`call_time`).

### 2.2. Envío del Token y Tipo de Cliente al Servidor (¡Importante!)

El token JWT, el tipo de cliente y, opcionalmente, el token FCM **deben ser enviados como parámetros de consulta (query parameters) en la URL de la conexión WebSocket**.

**Razón:** Enviar estos datos en la URL es una forma robusta y compatible que evita problemas con encabezados personalizados en entornos con proxies o firewalls, que pueden afectar a ciertos clientes WebSocket (especialmente en entornos nativos o navegadores con ciertas configuraciones).

**Formato de URL para la conexión WebSocket:**

```
wss://your-signal-server.com/ws?token=<YOUR_JWT_TOKEN>&clientType=<WEB_O_MOBILE>&fcmToken=<YOUR_FCM_TOKEN_OPCIONAL>
```

*   **`token`**: El JWT generado para la autenticación.
*   **`clientType`**: El tipo de cliente. Debe ser `web` o `mobile`. **Este parámetro es obligatorio.** Si no se proporciona o el valor es inválido, el servidor rechazará la conexión con un error `400 Bad Request`.
*   **`fcmToken`**: El token de Firebase Cloud Messaging. **Este parámetro es obligatorio si el rol del usuario (definido en el JWT) es `escucha` (listener)**. Los usuarios con rol `cliente` (caller) pueden omitir este parámetro o enviarlo vacío, ya que su naturaleza es efímera y no requieren ser localizados vía Push.

> [!TIP]
> **Diferencia entre `userRole` y `clientType`**
> Es importante distinguir estos dos conceptos que viajan en la conexión:
> - **`userRole` (Negocio)**: Proviene del claim `type` dentro del JWT. Define **quién eres** (permisos). Por ejemplo, solo un `cliente` puede llamar a un `escucha`.
> - **`clientType` (Técnico)**: Se envía como parámetro en la URL. Define **cómo te conectas** (plataforma). El servidor usa este valor para decidir si aplica lógica de persistencia por Push (en `mobile`) o si debe desconectarte inmediatamente al cerrar el socket (en `web`).

**Ejemplo:**

```
wss://localhost:8080/ws?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMTIzIiwidXNlck5hbWUiOiJTb2bDrWEgR2FyY8OtYSIsImlzcyI6InNpZ25hbC1zZXJ2ZXIiLCJleHAiOjE3MzE0NDY0MDAsImlhdCI6MTczMTM2MDAwMCwiY2FsbF90aW1lIjozMH0.SIGNATURE&clientType=mobile&fcmToken=YOUR_FCM_TOKEN_HERE
```

```
wss://localhost:8080/ws?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMTIzIiwidXNlck5hbWUiOiJTb2bDrWEgR2FyY8OtYSIsImlzcyI6InNpZ25hbC1zZXJ2ZXIiLCJleHAiOjE3MzE0NDY0MDAsImlhdCI6MTczMTM2MDAwMCwiY2FsbF90aW1lIjozMH0.SIGNATURE&clientType=web
```

### 2.3. Validación del Token y Registro del Cliente por el Servidor

Al recibir la solicitud de conexión WebSocket, el servidor:
1. Extrae el `token`, `clientType` y `fcmToken` (si está presente) de los parámetros de la URL.
2. Valida la firma del JWT usando `JWT_SECRET`.
3. Verifica los claims `sub`, `iss`, `exp` y `iat`.
4. Extrae el `userId` (del claim `sub`), `userName` (del claim `userName`) y `callTime` (del claim `call_time`).
5. Si el token es válido, la conexión WebSocket se establece y el cliente se registra. La disponibilidad inicial del cliente se establece en `true`.
6. Si el `clientType` es `mobile` y se proporciona un `fcmToken`, este se registra y se asocia al `userId` para futuras notificaciones push.
7. Si el token no es válido, la conexión es rechazada con un error `401 Unauthorized`.

**Nota Importante sobre FCM Tokens:** El `fcmToken` proporcionado en la URL se utiliza para el registro inicial del token del dispositivo. Si el `fcmToken` cambia posteriormente (por ejemplo, debido a un refresco de token por parte de Firebase), el cliente debe enviar un mensaje `update-push-token` (ver sección 4.2) para informar al servidor sobre el nuevo token. Además, un cliente móvil puede solicitar explícitamente la eliminación de su `fcmToken` enviando un `update-push-token` con un valor vacío, lo que lo marcará como no disponible para llamadas push.

## 3. Conexión WebSocket

Una vez autenticado, el cliente establece una conexión WebSocket con el servidor.

### 3.1. Establecimiento de la Conexión

El cliente inicia la conexión a la URL `wss://your-signal-server.com/ws?token=<YOUR_JWT_TOKEN>`.

### 3.2. Mensaje `connection-ack` (Server -> Client)

Tras una conexión exitosa, autenticación y registro del tipo de cliente, el servidor envía un mensaje de confirmación al cliente.

**Tipo:** `connection-ack`
**Descripción:** Confirma que el cliente ha sido conectado y registrado exitosamente.
**Ejemplo JSON:**

```json
{
  "type": "connection-ack",
  "to": "connectionId_of_this_client",
  "data": {
    "clientId": "connectionId_of_this_client",
    "userId": "userId_of_this_client"
  }
}
```

### 3.3. Mejores Prácticas de Robustez (Evitar Ciclos de Reconexión)

Para garantizar una conexión estable, especialmente en entornos móviles donde el cambio de red o de estado de la App es frecuente, se deben seguir estas reglas:

1.  **Limpieza de Suscripciones**: Antes de iniciar un nuevo intento de conexión (`connect`), el cliente **debe cancelar explícitamente** cualquier suscripción o escucha (listeners) activa de la conexión anterior. Si no se hace, el evento de "cierre" de una conexión antigua puede disparar erróneamente una nueva reconexión innecesaria.
2.  **Control de Estado Concurrente**: Utilizar un flag de control (ej: `_isConnecting`) para evitar que se inicien múltiples procesos de conexión simultáneamente.
3.  **Manejo de cierres por el servidor**: Si el servidor cierra una conexión antigua (ej. por exceder `MAX_CONN_PER_USER`), el cliente debe ser lo suficientemente inteligente para detectar si ya tiene una conexión nueva activa antes de decidir si debe intentar reconectar de nuevo.

> [!TIP]
> **Configuración del Servidor**: Se recomienda que el servidor tenga un `MAX_CONN_PER_USER` mayor a 1 (ej: 3) para permitir que los dispositivos móviles establezcan la nueva sesión antes de que la anterior sea totalmente purgada por el sistema.

## 4. Consideraciones de Seguridad: Validación de Origin

El servidor de señalización implementa protecciones contra ataques de **Cross-Site WebSocket Hijacking (CSWH)** mediante la validación del encabezado `Origin`.

### 4.1. Comportamiento según el Tipo de Cliente

1.  **Clientes Nativos (App Móvil - Flutter):**
    *   Los clientes móviles no son navegadores y, por lo tanto, no envían el encabezado `Origin` (el valor llega vacío al servidor).
    *   El servidor permite estas conexiones siempre que el encabezado esté vacío, asumiendo que provienen de una aplicación legítima y no de un entorno de ejecución de scripts controlado por un navegador.
2.  **Clientes Web (Navegador):**
    *   Los navegadores web envían obligatoriamente el encabezado `Origin` indicando el dominio desde donde se ejecuta el script.
    *   El servidor valida este valor contra una lista blanca configurada en `ALLOWED_ORIGINS` (archivo `.env`).
    *   Si el dominio no está en la lista blanca, la conexión es rechazada para evitar que sitios maliciosos utilicen la sesión del usuario.

> [!NOTE]
> Esta validación es una medida de "Defensa en Profundidad". Aunque el servidor utiliza tokens JWT en la URL para autenticación, la validación de `Origin` añade una capa extra necesaria si en el futuro se implementaran accesos vía web o paneles de administración basados en navegador.

## 5. Gestión de Disponibilidad y Tokens Push

Los clientes pueden actualizar su estado de disponibilidad y registrar tokens push para recibir notificaciones cuando no estén activamente conectados.

### 4.1. `update-availability` (Client -> Server)

El cliente informa al servidor si está disponible para recibir llamadas.

**Tipo:** `update-availability`
**Descripción:** Actualiza el estado de disponibilidad del cliente.
**Ejemplo JSON:**

```json
{
  "type": "update-availability",
  "data": {
    "isAvailable": true // o false
  }
}
```

### 4.2. `update-push-token` (Client -> Server)

El cliente envía su token FCM (Firebase Cloud Messaging) para registrarlo o **actualizarlo** en el servidor. Esto es crucial para que el servidor pueda notificar a un cliente que no tiene una conexión WebSocket activa, especialmente si el token cambia después de la conexión inicial (el registro inicial del token se realiza a través de la URL de conexión WebSocket, como se describe en la sección 2.2).

**Para desregistrar explícitamente el token FCM (ej. cuando la sesión del cliente expira o el usuario cierra sesión), el cliente debe enviar un `pushToken` vacío.**

**Tipo:** `update-push-token`
**Descripción:** Registra, actualiza o elimina el token push del cliente.
**Ejemplo JSON (Actualizar/Registrar):**

```json
{
  "type": "update-push-token",
  "data": {
    "pushToken": "your_fcm_push_token_here"
  }
}
```

**Ejemplo JSON (Eliminar/Desregistrar):**

```json
{
  "type": "update-push-token",
  "data": {
    "pushToken": ""
  }
}
```

### 4.3. `clients-list` (Server -> Client)

El servidor envía una lista actualizada de todos los clientes conectados y su disponibilidad a todos los clientes activos. Esto permite a los clientes saber quién está en línea y disponible para llamar.

**Consideración Importante sobre la Disponibilidad:**
La disponibilidad de un cliente (`isAvailable`) ahora se determina de la siguiente manera:
*   **Clientes Web:** Se consideran disponibles solo si tienen una conexión WebSocket activa.
*   **Clientes Móviles:** Se consideran disponibles si tienen una conexión WebSocket activa **O** si no tienen una conexión activa pero han registrado un `fcmToken` válido. Esto permite que los clientes móviles reciban llamadas a través de notificaciones push incluso cuando la aplicación está en segundo plano o cerrada.

**Tipo:** `clients-list`
**Descripción:** Lista de clientes conectados y su estado de disponibilidad, incluyendo el tipo de cliente y su rol.

**Comportamiento de Difusión (Broadcast):**
El servidor envía automáticamente este mensaje a **todos** los clientes conectados cada vez que ocurre un cambio relevante en la disponibilidad:
1.  **Conexión/Desconexión:** Un usuario entra o sale.
2.  **Cambio de Estado Manual:** Un usuario envía `update-availability`.
3.  **Inicio de Llamada:** Cuando se solicita una llamada (`call-request`), el servidor marca a ambos participantes como ocupados. Si la solicitud falla antes de conectar (ej. destinatario no alcanzable), el servidor revierte el estado y difunde la lista actualizada. Si la llamada se establece (`call-accept`), el servidor confirma el estado `isAvailable: false`.
4.  **Fin de Llamada:** Cuando termina una llamada (`hangup`, `reject`, `timeout`), el servidor marca a los participantes como `isAvailable: true` y difunde la lista actualizada.

**Manejo en el Cliente:**
El cliente **debe** escuchar este evento para mantener su interfaz de usuario sincronizada. Se recomienda:
*   Actualizar la lista local de contactos/usuarios.
*   **Deshabilitar o filtrar visualmente** a los usuarios cuyo estado sea `isAvailable: false`, impidiendo iniciar nuevas llamadas hacia ellos mientras estén ocupados.
*   Mostrar indicadores de estado (ej. punto verde para disponible, punto gris/rojo para ocupado).
**Ejemplo JSON:**

```json
{
  "type": "clients-list",
  "data": {
    "clients": [
      {
        "userId": "user123",
        "userName": "Sofía García",
        "connectionId": "connectionId_user123",
        "isAvailable": true, // Conexión WebSocket activa (web o móvil) o móvil con FCM token
        "clientType": "mobile",
        "userRole": "cliente"
      },
      {
        "userId": "user456",
        "userName": "Carlos Mendoza",
        "connectionId": "connectionId_user456",
        "isAvailable": false, // Sin conexión activa y sin FCM token (o web sin conexión)
        "clientType": "web",
        "userRole": "escucha"
      },
      {
        "userId": "user789",
        "userName": "Ana García",
        "connectionId": "", // Sin conexión WebSocket activa
        "isAvailable": true, // Cliente móvil con FCM token registrado
        "clientType": "mobile",
        "userRole": "escucha"
      }
    ]
  }
}
```

## 6. Servidores ICE (STUN/TURN) y Seguridad (Shared Secret)

Para garantizar la seguridad y evitar robo de ancho de banda en el servidor TURN (Coturn), los clientes **no deben** usar credenciales fijas (`username`/`password`) obtenidas a nivel de código (`hardcoded`). 

En su lugar, el servidor de señalización implementa la autenticación **TURN REST API (Shared Secret)**. Cada vez que el cliente inicia o recibe una llamada, requiere credenciales temporales generadas dinámicamente. 

### 5.1. Obtención de Credenciales Temporales TURN (vía API HTTP)

Antes de iniciar una conexión WebRTC (es decir, antes de crear el `RTCPeerConnection` y antes de iniciar la solicitud `call-request` o responder `call-accept`), el cliente debe consumir un endpoint HTTP(S) proporcionado por el servidor de señalización para obtener sus credenciales ICE frescas.

*   **Endpoint:** `GET /api/turn-credentials` (La ruta exacta debe coincidir con la implementación del servidor Go).
*   **Autenticación:** El cliente debe de probar su identidad al servidor. Si el servidor expone este endpoint independientemente, el cliente debe enviar el mismo token JWT usado para el WebSocket en el encabezado de la petición (e.g. `Authorization: Bearer <token>`).
*   **Comportamiento esperado del Servidor:** El servidor generará un `username` de la forma `timestamp:userId` (p.ej.: `1731446400:user123`) y generará un `password` basado en el `TURN_REST_API_SECRET` configurado en el servidor usando HMAC-SHA1 y codificándolo en Base64.
*   **Respuesta JSON esperada:** El servidor responderá con una lista de diccionarios que el cliente podrá enchufar directamente en la configuración `iceServers` de WebRTC.

**Ejemplo de Respuesta HTTP (JSON):**

```json
{
  "iceServers": [
    {
      "urls": ["stun:ice.eypelame.com.mx:3478"]
    },
    {
      "urls": ["turn:ice.eypelame.com.mx:3478"],
      "username": "1731446400:user123",
      "credential": "XyzBase64HmacHash="
    }
  ]
}
```

El cliente debe inyectar este array generado dinámicamente en su configuración WebRTC (`rtcConfiguration`) para la llamada actual. **Estas credenciales expiran automáticamente**, generalmente 24 horas después del timestamp o según lo configure el administrador, por lo que el cliente siempre debe refrescarlas al iniciar una nueva llamada.

## 7. Flujo de Llamada 1 a 1

Este es el proceso central para establecer una llamada WebRTC.

### 5.1. Inicio de Llamada (`call-request`)

El cliente A (caller) inicia una llamada al cliente B (listener).

#### 5.1.1. Client A -> Server (`call-request`)

El cliente A envía una solicitud de llamada al servidor, incluyendo su oferta SDP.

**Tipo:** `call-request`
**Descripción:** Solicita iniciar una llamada con un `targetUserId`.
**Campos JSON:**
*   `to`: `userId` del cliente B (listener).
*   `roomId`: (Opcional) Un `roomId` preexistente si se intenta reconectar a una sala. Si no se proporciona, el servidor generará uno.
*   `data`:
    *   `sdp`: La descripción de sesión SDP (Session Description Protocol) del cliente A.
    *   `sdpType`: El tipo de SDP (ej. "offer").

**Ejemplo JSON:**

```json
{
  "type": "call-request",
  "to": "listener_userId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "offer"
  }
}
```

*   El servidor valida la disponibilidad del usuario destino.
*   El servidor genera un **`roomId` único** para la sesión.
*   El servidor envía un mensaje de confirmación (`call-request-ack`) al llamante, incluyendo el `roomId` generado.

#### 5.1.2. Validaciones del Servidor

Al recibir `call-request`, el servidor realiza las siguientes validaciones:
1.  Verifica que el `targetUserId` (listener) exista y esté **disponible** (`userAvailability[targetUserID] == true`).
2.  Si el `targetUserId` no está disponible, el servidor envía un `call-request-ack` de fallo al cliente A.

#### 5.1.3. Server -> Client A (`call-request-ack` y `call-ringing`)

Si el `targetUserId` está disponible, el servidor responde al cliente A.

**a) `call-request-ack` (Server -> Client A)**
**Tipo:** `call-request-ack`
**Descripción:** Acuse de recibo de la solicitud de llamada.
**Campos JSON:**
*   `to`: `connectionId` del cliente A.
*   `roomId`: El `roomId` generado o proporcionado para esta llamada.
*   `data`:
    *   `status`: "ringing" (si el listener está siendo contactado) o "failed" (si no se pudo contactar).
    *   `reason`: Mensaje descriptivo en caso de fallo.

**Ejemplo JSON (éxito):**

```json
{
  "type": "call-request-ack",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "status": "ringing",
    "reason": ""
  }
}
```

**Ejemplo JSON (fallo):**

```json
{
  "type": "call-request-ack",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "status": "failed",
    "reason": "El usuario no está disponible para recibir llamadas."
  }
}
```

**b) `call-ringing` (Server -> Client A)**
**Tipo:** `call-ringing`
**Descripción:** Indica al cliente A que el cliente B está siendo notificado de la llamada.
**Campos JSON:**
*   `to`: `connectionId` del cliente A.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `targetUserId`: `userId` del cliente B.

**Ejemplo JSON:**

```json
{
  "type": "call-ringing",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "targetUserId": "listener_userId"
  }
}
```

#### 5.1.4. Server -> Client B (Notificación al Listener)

El servidor intenta notificar al cliente B (listener) de la llamada entrante.

**a) Si Client B tiene una conexión WebSocket activa:**
El servidor reenvía el mensaje `call-request` original al cliente B.

**Tipo:** `call-request`
**Descripción:** Notificación de llamada entrante.
**Campos JSON:**
*   `from`: `connectionId` del cliente A.
*   `to`: `connectionId` del cliente B.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `sdp`: La oferta SDP del cliente A.
    *   `sdpType`: "offer".
    *   `callerUserId`: `userId` del cliente A.
    *   `callerUserName`: Nombre del cliente A.

**Ejemplo JSON:**

```json
{
  "type": "call-request",
  "from": "caller_connectionId",
  "to": "listener_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "offer",
    "callerUserId": "caller_userId",
    "callerUserName": "Sofía García"
  }
}
```

**b) Si Client B no tiene una conexión WebSocket activa (pero tiene un `pushToken`):**
El servidor envía una notificación push a través de FCM al dispositivo del cliente B.

**Tipo:** Notificación Push (FCM)
**Descripción:** Notificación de llamada entrante.
**Payload de Datos FCM (ejemplo):**

```json
	notificationData := map[string]string{
		"type":           "call-request",
		"callerUserId":   callerClient.UserID,
		"callerUserName": callerClient.UserName,
		"roomId":         roomID,
		"sdp":            sdp,
		"sdpType":        sdpType,
	}
```

**Descripción del Payload de Datos FCM:**
*   **`type`**: Siempre será `"call-request"` para indicar que es una notificación de llamada entrante.
*   **`callerUserId`**: El `userId` del cliente que está iniciando la llamada.
*   **`callerUserName`**: El nombre del cliente que está iniciando la llamada.
*   **`roomId`**: El `roomId` único de la sala de llamada.
*   **`sdp`**: La descripción de sesión SDP (Session Description Protocol) del cliente que llama.
*   **`sdpType`**: El tipo de SDP, que será `"offer"`.
*   **`from`**: El `connectionId` del cliente que inicia la llamada.
*   **`messagePayload`**: Una representación en cadena JSON del payload original del mensaje `call-request` enviado por WebSocket. Esto permite al cliente reconstruir el mensaje completo si es necesario.

El cliente B, al recibir la notificación push, debe iniciar su aplicación, establecer una conexión WebSocket y luego decidir si acepta o rechaza la llamada.

### 5.2. Aceptación de Llamada (`call-accept`)

El cliente B (listener) acepta la llamada.

#### 5.2.1. Client B -> Server (`call-accept`)

El cliente B envía su respuesta SDP al servidor.

**Tipo:** `call-accept`
**Descripción:** Acepta la llamada entrante y envía la respuesta SDP.
**Campos JSON:**
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `sdp`: La descripción de sesión SDP del cliente B.
    *   `sdpType`: El tipo de SDP (ej. "answer").

**Ejemplo JSON:**

```json
{
  "type": "call-accept",
  "roomId": "generated_roomId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "answer"
  }
}
```

#### 5.2.2. Validaciones del Servidor

El servidor verifica que el cliente B sea el `listenerUserId` esperado para esa `roomId`.

#### 5.2.3. Server -> Client A (`call-accept` con SDP Answer)

El servidor reenvía la respuesta SDP del cliente B al cliente A.

**Tipo:** `call-accept`
**Descripción:** Reenvía la respuesta SDP del listener al caller.
**Campos JSON:**
*   `from`: `connectionId` del cliente B.
*   `to`: `connectionId` del cliente A.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `sdp`: La respuesta SDP del cliente B.
    *   `sdpType`: "answer".
    *   `recipientUserId`: `userId` del cliente B.

**Ejemplo JSON:**

```json
{
  "type": "call-accept",
  "from": "listener_connectionId",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "answer",
    "recipientUserId": "listener_userId"
  }
}
```
En este punto, ambos clientes tienen la información SDP necesaria para establecer la conexión WebRTC.

### 5.3. Rechazo de Llamada (`call-reject`)

El cliente B (listener) rechaza la llamada.

#### 5.3.1. Client B -> Server (`call-reject`)

El cliente B envía un mensaje de rechazo al servidor.

**Tipo:** `call-reject`
**Descripción:** Rechaza la llamada entrante.
**Campos JSON:**
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `reason`: (Opcional) Razón del rechazo (ej. "busy", "unavailable").

**Ejemplo JSON:**

```json
{
  "type": "call-reject",
  "roomId": "generated_roomId",
  "data": {
    "reason": "busy"
  }
}
```

#### 5.3.2. Server -> Client A (`call-rejected`)

El servidor notifica al cliente A que la llamada ha sido rechazada.

**Tipo:** `call-rejected`
**Descripción:** Notifica al caller que la llamada fue rechazada.
**Campos JSON:**
*   `from`: `connectionId` del cliente B.
*   `to`: `connectionId` del cliente A.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `reason`: Razón del rechazo.
    *   `recipientUserId`: `userId` del cliente B.

**Ejemplo JSON:**

```json
{
  "type": "call-rejected",
  "from": "listener_connectionId",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "reason": "busy",
    "recipientUserId": "listener_userId"
  }
}
```
Después de un rechazo, el servidor limpia la sala.

**Acción del Cliente (Caller):** Al recibir un mensaje `call-rejected`, el cliente que inició la llamada (Caller) debe:
1.  Detener cualquier indicador de llamada saliente (como el tono de "ringback").
2.  Actualizar su interfaz de usuario para informar que la llamada fue rechazada.
3.  **No debe** enviar un mensaje `hangup`. El rechazo es un estado final de la llamada.
4.  Proceder a limpiar los recursos locales de la llamada (como el `RTCPeerConnection` y los streams de medios) y cerrar la conexión WebSocket.

#### 5.4. Intercambio de Señalización WebRTC

Una vez que la llamada es aceptada, los clientes intercambian candidatos ICE y ofertas/respuestas SDP adicionales si es necesario.

#### 5.4.1. `sdp-offer` (Client -> Server -> Other Client)

Si un cliente necesita enviar una nueva oferta SDP (ej. para renegociación).

**Tipo:** `sdp-offer`
**Descripción:** Envía una oferta SDP a otro cliente en la sala.
**Campos JSON:**
*   `to`: `connectionId` del cliente destinatario.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `sdp`: La oferta SDP.
    *   `sdpType`: "offer".

**Ejemplo JSON:**

```json
{
  "type": "sdp-offer",
  "to": "other_client_connectionId",
  "roomId": "current_roomId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "offer"
  }
}
```
El servidor reenvía este mensaje al `to` especificado.

#### 5.4.2. `sdp-answer` (Client -> Server -> Other Client)

Respuesta a una oferta SDP.

**Tipo:** `sdp-answer`
**Descripción:** Envía una respuesta SDP a otro cliente en la sala.
**Campos JSON:**
*   `to`: `connectionId` del cliente destinatario.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `sdp`: La respuesta SDP.
    *   `sdpType`: "answer".

**Ejemplo JSON:**

```json
{
  "type": "sdp-answer",
  "to": "other_client_connectionId",
  "roomId": "current_roomId",
  "data": {
    "sdp": "v=0\r\no=- 12345 67890 IN IP4 127.0.0.1\r\ns=-\r\n...",
    "sdpType": "answer"
  }
}
```
El servidor reenvía este mensaje al `to` especificado.

#### 5.4.3. `ice-candidate` (Client -> Server -> Other Client)

Intercambio de candidatos ICE para establecer la conectividad.

**Tipo:** `ice-candidate`
**Descripción:** Envía un candidato ICE a otro cliente en la sala.
**Campos JSON:**
*   `to`: `connectionId` del cliente destinatario.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `candidate`: El objeto candidato ICE (puede variar en estructura, pero generalmente incluye `candidate`, `sdpMid`, `sdpMLineIndex`).

**Ejemplo JSON:**

```json
{
  "type": "ice-candidate",
  "to": "other_client_connectionId",
  "roomId": "current_roomId",
  "data": {
    "candidate": {
      "candidate": "candidate:1 1 UDP 2122267903 192.168.1.100 50000 typ host",
      "sdpMid": "audio",
      "sdpMLineIndex": 0
    }
  }
}
```
El servidor reenvía este mensaje al `to` especificado.

### 5.5. Finalización de Llamada (`hangup`)

Un cliente finaliza la llamada.

#### 5.5.1. Client -> Server (`hangup`)

Un cliente envía un mensaje de `hangup` al servidor.

**Tipo:** `hangup`
**Descripción:** Finaliza la llamada en la `roomId` especificada.
**Campos JSON:**
*   `roomId`: El `roomId` de la llamada a finalizar.

**Ejemplo JSON:**

```json
{
  "type": "hangup",
  "roomId": "current_roomId"
}
```

#### 5.5.2. Server -> Other Client (`hangup`)

El servidor notifica al otro cliente en la sala que la llamada ha terminado.

**Tipo:** `hangup`
**Descripción:** Notifica al cliente que la llamada ha terminado.
**Campos JSON:**
*   `to`: `connectionId` del cliente destinatario.
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `reasonCode`: Código de la razón (ej. "user_hung_up", "timeout").
    *   `reasonMessage`: Mensaje descriptivo.

**Ejemplo JSON:**

```json
{
  "type": "hangup",
  "to": "other_client_connectionId",
  "roomId": "current_roomId",
  "data": {
    "reasonCode": "user_hung_up",
    "reasonMessage": "El usuario caller_userId ha colgado la llamada."
  }
}
```
Después de un `hangup`, el servidor limpia la sala.

Comportamiento del Servidor al Finalizar la Llamada (Hangup o Reject):
Al recibir un mensaje de `hangup` o `call-reject`, el servidor realiza las siguientes acciones:
1.  **Notifica al otro participante:** Si un cliente cuelga, el servidor envía un mensaje `hangup` al otro cliente en la sala. Si un cliente rechaza, envía un `call-rejected` al llamante.
2.  **Limpia la Sala:** La sala de la llamada se elimina del registro interno del servidor.
3.  **Restablece la Disponibilidad:** Ambos participantes de la llamada son marcados inmediatamente como disponibles (`isAvailable: true`), permitiéndoles iniciar o recibir nuevas llamadas.

**Importante:** La conexión WebSocket de ambos clientes **permanece abierta y activa**. El servidor ya no fuerza la desconexión del llamante. Esto permite a los clientes realizar nuevas llamadas de forma consecutiva sin necesidad de reconectarse y obtener un nuevo `callToken` para cada una.
### 7.6. Reporte de Calidad de Red (`connection-report`)

Este mensaje es opcional y se utiliza para fines de monitoreo y auditoría de la infraestructura de red (STUN vs TURN).

**Emisor Recomendado:** El cliente que origina la llamada (**Caller**), una vez que la conexión WebRTC se ha estabilizado.

**Timing:** Se recomienda enviarlo aproximadamente **3 segundos después** de recibir/procesar el `call-accept` y confirmar el estado `connected` en el `RTCPeerConnection`.

**Tipo:** `connection-report`
**Descripción:** Informa al servidor sobre el tipo de candidato ICE que finalmente se seleccionó para la sesión.
**Campos JSON:**
*   `roomId`: El `roomId` de la llamada.
*   `data`:
    *   `candidateType`: El tipo de conexión detectada. Valores comunes: `"host"` (Local/LAN), `"srflx"` (STUN/P2P), `"relay"` (TURN/Servidor).

**Ejemplo JSON:**

```json
{
  "type": "connection-report",
  "roomId": "current_roomId",
  "data": {
    "candidateType": "relay"
  }
}
```

**Comportamiento del Servidor:**
El servidor registrará esta información en sus logs de tráfico (`signalserver.log`) con el prefijo `[ICE-REPORT]`. Esto permite al administrador identificar llamadas que consumen ancho de banda del servidor TURN de forma imprevista.

## 8. Sincronización de Estado con API Backend (Solo para Escuchas)

El servidor de señalización sincroniza automáticamente el estado de los usuarios con el backend del sistema (API PHP) para mantener actualizados los registros de disponibilidad en la base de datos. **IMPORTANTE: Esta sincronización solo se realiza para usuarios con rol `escucha`.** Los usuarios con rol `cliente` (callers) son ignorados por el backend para evitar procesamientos innecesarios y posibles errores de base de datos.

### 6.1. Estados de Usuario

Los estados que se reportan a la API son:

*   **1 (Disponible)**: El usuario está conectado al WebSocket y libre para recibir llamadas.
*   **2 (Llamando)**: El usuario está iniciando una llamada o recibiendo una solicitud (timbrando).
*   **3 (En Llamada)**: La llamada ha sido aceptada y está en curso.
*   **4 (No Disponible)**: El usuario se ha desconectado del WebSocket.

### 6.2. Momentos de Actualización

El servidor de señalización invoca el endpoint `POST /api/webrtc/update-call-status` en los siguientes eventos:

1.  **Al Conectar (Web/Móvil):**
    *   Estado enviado: **1 (Disponible)**.

2.  **Al Desconectar (Web/Móvil):**
    *   Estado enviado: **4 (No Disponible)**.
    *   *Nota:* Si un cliente móvil tiene FCM token, técnicamente sigue alcanzable, pero visualmente se marca como desconectado (4) hasta que interactúe.

3.  **Al Iniciar Llamada (`call-request`):**
    *   **Caller:** Estado enviado: **2 (Llamando)**.
    *   **Listener (Destino):** Estado enviado: **2 (Llamando)** (para prevenir colisiones de llamadas entrantes).

4.  **Al Aceptar Llamada (`call-accept`):**
    *   **Caller:** Estado enviado: **3 (En Llamada)**.
    *   **Listener:** Estado enviado: **3 (En Llamada)**.

5.  **Al Finalizar Llamada (`hangup`, `call-reject`, Timeout):**
    *   **Caller:** Estado enviado: **1 (Disponible)**.
    *   **Listener:** Estado enviado: **1 (Disponible)**.

## 7. Procesamiento y Facturación Post-Llamada ("ProcessCall")

Además de la sincronización de estado (disponible/ocupado), el servidor de señalización se encarga de reportar la actividad de la llamada al finalizar para propósitos de facturación y registro histórico.

### 7.1. Flujo de Procesamiento

1.  La llamada finaliza (por `hangup`, `call-reject` o `timeout`).
2.  El servidor calcula la duración exacta (`EndTime - StartTime`).
3.  Si la llamada tuvo una duración mayor a 0 (fue aceptada), el servidor invoca el endpoint `POST /api/webrtc/process-call`.

### 7.2. Datos Enviados al Backend

El payload JSON incluye:

```json
{
  "roomId": "uuid-de-la-sala",
  "callerUserId": 123,
  "listenerUserId": 456,
  "startTime": "2023-10-27T10:00:00Z",
  "endTime": "2023-10-27T10:05:30Z",
  "durationSeconds": 330,
  "reason": "hangup"
}
```

### 7.3. Lógica del Backend (API PHP)

El endpoint `WebRTCCallProcessorController::process` realiza lo siguiente:

1.  **Cálculo de Minutos:** Convierte segundos a minutos, redondeando hacia arriba (ej. 61 segundos = 2 minutos).
2.  **Descuento de Crédito:** Resta los minutos consumidos de la tabla `cliente_credito` del usuario llamante.
3.  **Registro:** Inserta un registro en la tabla `llamadas_log` con el costo calculado y detalles de la sesión.

## 8. Mensajes de Error (`error`)

El servidor puede enviar mensajes de error a los clientes en caso de problemas.

**Tipo:** `error`
**Descripción:** Notifica un error al cliente.
**Campos JSON:**
*   `to`: `connectionId` del cliente que recibe el error.
*   `data`:
    *   `code`: Código de error (ej. "INVALID_JSON", "ROOM_NOT_FOUND", "UNAUTHORIZED").
    *   `message`: Mensaje descriptivo del error.

**Ejemplo JSON:**

```json
{
  "type": "error",
  "to": "client_connectionId",
  "data": {
    "code": "ROOM_NOT_FOUND",
    "message": "La sala de llamada no existe."
  }
}
```

## 7. Desconexión del Cliente y Gestión de Sesión

### 7.1. Client -> Server (`clear-fcm-and-disconnect`) (¡Nuevo y Recomendado para Logout!)

Este es el método preferido para que un cliente realice un **Logout Global**. Asegura que el token FCM sea desregistrado en el servidor y que **todas las conexiones activas** asociadas al `userId` (incluyendo sesiones "zombie" o en segundo plano) se cierren de forma inmediata y elegante.

**Tipo:** `clear-fcm-and-disconnect`
**Descripción:** Solicita al servidor un cierre de sesión global: elimina el `fcmToken` asociado al `userId` y cierra **todas las conexiones WebSocket** del usuario.
**Campos JSON:**
*   Este mensaje no requiere un `data` payload específico, se envía un objeto JSON vacío.

**Ejemplo JSON:**

```json
{
  "type": "clear-fcm-and-disconnect",
  "data": {}
}
```

**Comportamiento del Servidor:**
Al recibir este mensaje, el servidor:
1.  **Elimina el `fcmToken`** asociado al `UserID` de la conexión actual de su registro interno.
2.  **Marca al `UserID` como no disponible** si no tiene otras conexiones activas.
3.  **Cierra la conexión WebSocket** de forma controlada desde su lado.
4.  **Envía una lista de clientes actualizada** a todos los clientes restantes.

**Acción del Cliente (Flutter/Móvil):**
El cliente debe:
1.  Enviar este mensaje al servidor.
2.  Luego de un breve tiempo (para asegurar que el servidor procese el mensaje), proceder a limpiar sus tokens de sesión locales (JWT, `callToken`, etc.).
3.  No necesita cerrar explícitamente el WebSocket desde su lado, ya que el servidor lo hará.

### 7.2. Client -> Server (`disconnect`) (Desconexión Simple de WebSocket)

Este mensaje se utiliza cuando el cliente desea cerrar su conexión WebSocket, pero **no** desregistrar su token FCM del servidor. Es útil para cierres temporales o cuando la aplicación pasa a segundo plano sin intención de logout. Para un logout completo, se recomienda usar `clear-fcm-and-disconnect`.

**Tipo:** `disconnect`
**Descripción:** Solicita al servidor que cierre la conexión WebSocket actual. **No desregistra el `fcmToken`.**
**Ejemplo JSON:**

```json
{
  "type": "disconnect"
}
```

Al recibir este mensaje, el servidor cerrará la conexión WebSocket y el cliente será desregistrado de las conexiones activas, pero **su `fcmToken` permanecerá en el servidor si fue registrado previamente y no hay otras conexiones activas para ese usuario.**

## 8. Sesión Reemplazada (`session-replaced`) (Server -> Client)

Si el servidor está configurado para limitar el número de conexiones simultáneas por usuario (ej. `MAX_CONN_PER_USER=1`), y un usuario establece una nueva conexión que excede este límite, el servidor cerrará la conexión más antigua. Antes de cerrarla, enviará este mensaje a la conexión antigua.

**Tipo:** `session-replaced`
**Descripción:** Notifica al cliente que su sesión ha sido terminada porque se ha iniciado una nueva conexión con el mismo `userId` desde otra ubicación. El cliente debe interpretar esto como una desconexión forzada y limpiar su estado.
**Ejemplo JSON:**

```json
{
  "type": "session-replaced",
  "to": "old_client_connectionId",
  "data": {
    "code": "SESSION_REPLACED",
    "message": "Nueva conexión iniciada desde otra ubicación."
  }
}
```

## 9. Estructura General de Mensajes JSON

Todos los mensajes intercambiados a través de WebSocket siguen una estructura base:

```json
{
  "type": "message_type_string",
  "from": "sender_connectionId_optional",
  "to": "recipient_userId_or_connectionId_optional",
  "roomId": "call_roomId_optional",
  "data": {
    // Payload específico del tipo de mensaje
  }
}
```

**Consideraciones Clave para el Cliente (Especialmente Flutter/Móvil):**

*   **Manejo de `roomId`**: Siempre que se inicie o se interactúe con una llamada, el `roomId` es fundamental para identificar la sesión.
*   **Estado de Disponibilidad**: Los clientes deben mantener su estado de disponibilidad actualizado con el servidor.
*   **Tokens Push (FCM)**: Es vital que los clientes con rol **`escucha`** (listener) obtengan y envíen su `fcmToken` en la URL de conexión inicial para ser contactables. Para clientes con rol **`cliente`** (caller), este token es opcional. Además, los listeners deben gestionar el ciclo de vida de este token (refrescos) y enviarlo al servidor si cambia (usando `update-push-token`). Para un **logout completo que desregistre el FCM token**, el cliente debe enviar el mensaje `clear-fcm-and-disconnect` al servidor.
*   **Gestión de Sesión del Cliente (Conexión Persistente)**: La conexión WebSocket está diseñada para ser persistente. Después de que una llamada finaliza (ya sea por `hangup` local, `call-rejected`, `hangup` remoto o un error), el cliente **no debe** cerrar su conexión WebSocket. En su lugar, debe:
    1.  Limpiar su estado de llamada local (cerrar la `RTCPeerConnection`, liberar el stream de audio/video, etc.).
    2.  Volver a un estado "disponible" en su interfaz de usuario.
    3.  Permanecer listo para iniciar o recibir una nueva llamada utilizando la misma conexión WebSocket y el mismo `callToken` (mientras este no haya expirado). **Para un logout completo, el cliente debe usar el mensaje `clear-fcm-and-disconnect` para desregistrar el FCM token y cerrar la conexión.**

Este documento proporciona una guía completa para la implementación del lado del cliente, asegurando una comunicación robusta y consistente con el servidor de señalización.

## 9. Manejo de Escenarios de Robustez (Mejoras del Servidor)

El servidor de señalización ha sido mejorado para manejar de forma más robusta varios escenarios que pueden ocurrir durante el ciclo de vida de una llamada. Los clientes deben estar preparados para recibir mensajes adicionales o diferentes en estas situaciones.

### 9.1. Cliente de Destino Ocupado

Si un cliente (A) intenta llamar a otro cliente (B) que ya está en una llamada activa (estado `active` o `pending`), el servidor rechazará la nueva solicitud de llamada.

**Mensaje Recibido por el Cliente A (Caller):**

**Tipo:** `call-request-ack`
**Descripción:** Acuse de recibo de la solicitud de llamada con estado de fallo.
**Ejemplo JSON:**

```json
{
  "type": "call-request-ack",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "status": "failed",
    "reason": "El usuario ya está en una llamada activa."
  }
}
```

**Acción del Cliente:** El cliente A debe detener cualquier tono de llamada o indicador visual y notificar a su usuario que el destinatario está ocupado.

### 9.2. Timeout de Solicitud de Llamada (Listener No Responde)

Si un cliente (A) llama a otro cliente (B), y el cliente B no acepta ni rechaza la llamada dentro de un período configurable (por defecto, 30 segundos, definido por `CALL_REQUEST_TIMEOUT_SECONDS` en la configuración del servidor), el servidor cancelará la solicitud de llamada.

**Mensaje Recibido por el Cliente A (Caller):**

**Mensaje Recibido por el Cliente A (Caller):**

**Tipo:** `call-request-ack`
**Descripción:** Acuse de recibo de la solicitud de llamada con estado de fallo debido a timeout.
**Ejemplo JSON:**

```json
{
  "type": "call-request-ack",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "status": "failed",
    "reason": "El usuario no respondió a la llamada a tiempo."
  }
}
```

**Acción del Cliente:** El cliente A debe detener cualquier tono de llamada o indicador visual y notificar a su usuario que el destinatario no respondió.

**Tipo:** `call-request-ack`
**Descripción:** Acuse de recibo de la solicitud de llamada con estado de fallo debido a timeout.
**Ejemplo JSON:**

```json
{
  "type": "call-request-ack",
  "to": "caller_connectionId",
  "roomId": "generated_roomId",
  "data": {
    "status": "failed",
    "reason": "El usuario no respondió a la llamada a tiempo."
  }
}
```

**Acción del Cliente:** El cliente A debe detener cualquier tono de llamada o indicador visual y notificar a su usuario que el destinatario no respondió.

### 9.3. Desconexión Inesperada de un Compañero Durante una Llamada Activa

Si un cliente (A) está en una llamada con el cliente (B), y el cliente B se desconecta inesperadamente (ej. pierde la conexión a Internet, cierra la aplicación), el servidor detectará esta desconexión y finalizará la llamada para el cliente A.

**Mensaje Recibido por el Cliente A (Compañero Restante):**

**Tipo:** `hangup`
**Descripción:** Notifica que la llamada ha terminado debido a la desconexión del compañero.
**Ejemplo JSON:**

```json
{
  "type": "hangup",
  "to": "remaining_client_connectionId",
  "roomId": "current_roomId",
  "data": {
    "reasonCode": "peer_disconnected",
    "reasonMessage": "El usuario [ID_del_cliente_desconectado] se ha desconectado."
  }
}
```

Acción del Cliente:
El cliente que recibe el mensaje `hangup` (el compañero restante) debe:
1.  **Escuchar el stream de señalización:** El cliente Flutter (específicamente `ActiveCallPage`) se suscribe al `SignalService.onSignalingMessage`.
2.  **Verificar el mensaje:** Al recibir un mensaje de tipo `hangup` con el `roomId` correspondiente a la llamada actual.
3.  **Finalizar la llamada localmente:** Ejecutar la lógica de limpieza de la llamada (ej. `_performHangupCleanup()` en Flutter) que incluye colgar la conexión WebRTC y navegar fuera de la página de llamada activa.
4.  **Limpiar recursos:** Liberar cualquier recurso WebRTC asociado y actualizar el estado de la UI para reflejar que la llamada ha terminado.

### 9.4. Reconexión a una Llamada Existente

Si un cliente se desconecta brevemente y luego se reconecta, puede intentar reincorporarse a una llamada activa si su compañero aún está conectado y la sala no ha sido limpiada por el servidor.

**Flujo de Reconexión (Cliente -> Servidor):**

1.  **Cliente se reconecta:** El cliente establece una nueva conexión WebSocket y se autentica.
2.  **Cliente envía `call-request` con `roomId` existente:** El cliente debe enviar un mensaje `call-request` incluyendo el `roomId` de la llamada a la que desea reincorporarse.
    *   **Tipo:** `call-request`
    *   **Campos JSON:**
        *   `to`: `userId` del compañero en la llamada.
        *   `roomId`: El `roomId` de la llamada activa a la que se desea reincorporar.
        *   `data`: (Opcional) Puede incluir una nueva oferta SDP si es necesario para la renegociación de la conexión WebRTC.

**Comportamiento del Servidor:**
*   El servidor verificará que el `userId` del cliente que se reconecta sea un participante válido de la `roomId` proporcionada.
*   Si es válido, el servidor actualizará la sala con el nuevo `connectionId` del cliente.
*   El servidor puede reenviar el `call-request` (o un mensaje de señalización apropiado) al compañero para facilitar la renegociación de la conexión WebRTC.

**Acción del Cliente:** El cliente debe estar preparado para renegociar la conexión WebRTC (ej. intercambiar nuevas ofertas/respuestas SDP y candidatos ICE) para restablecer el flujo de medios.

### 9.5. Mensajes Malformados o Acciones en Estado Inválido

El servidor ahora realiza validaciones más estrictas en los mensajes recibidos. Si un cliente envía un mensaje con un payload incompleto/incorrecto o intenta una acción que no es válida para el estado actual de la llamada, el servidor responderá con un mensaje de `error`.

**Mensaje Recibido por el Cliente:**

**Tipo:** `error`
**Descripción:** Notifica un error debido a un mensaje inválido o una acción no permitida.
**Ejemplo JSON:**

```json
{
  "type": "error",
  "to": "client_connectionId",
  "data": {
    "code": "INVALID_PAYLOAD",
    "message": "El payload de solicitud de llamada debe contener 'sdp' y 'sdpType'."
  }
}
```
O:
```json
{
  "type": "error",
  "to": "client_connectionId",
  "data": {
    "code": "INVALID_CALL_STATE",
    "message": "La llamada no está en un estado que permita la aceptación."
  }
}
```

**Acción del Cliente:** El cliente debe manejar estos errores, posiblemente mostrando un mensaje al usuario y ajustando el estado de su aplicación.

## 10. Chequeo de Salud del Servidor (`/health`)

El servidor de señalización expone un endpoint HTTP simple para que los clientes y sistemas de monitoreo puedan verificar su disponibilidad y estado operativo.

**Ruta:** `/health`
**Método:** `GET`
**Descripción:** Proporciona una verificación básica de que el servidor está en funcionamiento y puede responder a las solicitudes HTTP.
**Respuesta Esperada (200 OK):**

```json
{
  "status": "ok",
  "message": "Signal server is healthy"
}
```

**Acción del Cliente:** Los clientes pueden usar este endpoint antes de intentar establecer una conexión WebSocket para confirmar que el servidor está accesible. Esto puede ayudar a diferenciar problemas de red del cliente de problemas del servidor.

## 11. Ciclo de Vida de la Conexión (Flujo Elegante)

Si bien el servidor está diseñado para mantener la conexión WebSocket abierta después de que una llamada finaliza (por `hangup`, `call-rejected`, etc.) para permitir llamadas consecutivas de manera eficiente, el flujo de comunicación más limpio y recomendado es que el cliente gestione activamente el fin de la conexión.

**Comportamiento Esperado del Cliente:**

Una vez que una llamada ha concluido por cualquier motivo (el usuario cuelga, la llamada es rechazada, o se recibe un mensaje `hangup` del otro participante), se espera que el cliente:

1.  Complete toda su lógica de limpieza local (cerrar la `RTCPeerConnection`, liberar streams, etc.).
2.  Envíe un mensaje de tipo `disconnect` al servidor.
3.  Cierre la conexión WebSocket desde su lado.

Este enfoque asegura un ciclo de vida de la conexión explícito y controlado por el cliente, lo que resulta en un flujo de comunicación más robusto y "elegante", evitando conexiones inactivas y asegurando que el estado del servidor se limpie de manera predecible.

## 12. Sincronización de Estado Servidor-a-Servidor (Server-Side)

Para mantener la consistencia en el estado de los usuarios (únicamente los **Escuchas**) en la base de datos de la API principal, el Servidor de Señalización comunica los cambios de estado de las llamadas directamente a la API a través de Webhooks internos. **Este proceso se omite para los Clientes (Callers)**, ya que su estado no es rastreado en la base de datos del backend para fines de disponibilidad pública.

### 12.1. Actualización de Ocupado (`Busy`)

Cuando una llamada se establece exitosamente (`call-accept`), el Servidor de Señalización notifica a la API que ambos participantes están en una llamada.

*   **Evento:** `call-accept` procesado.
*   **Acción:** POST a API interna.
*   **Dato Actualizado:** `id_call_status` se establece en **2** (En llamada/Busy) en la tabla `escucha` (y `cliente` si aplica).

### 12.2. Actualización de Disponible (`Available`)

Cuando una llamada finaliza por cualquier motivo (colgar, rechazar, error, desconexión), el Servidor de Señalización notifica a la API para liberar a los usuarios.

*   **Eventos:** `hangup`, `call-reject`, `unregister` (desconexión), `timeout`.
*   **Acción:** POST a API interna.
*   **Dato Actualizado:** `id_call_status` se establece en **1** (Disponible) en la tabla `escucha`.

**Nota:** Este mecanismo asegura que si un usuario consulta la API para ver la lista de escuchas, verá el estado real "En llamada" si están ocupados en el sistema de señalización, sin necesidad de consultar al signaling server directamente.

## 13. Especificación de Seguridad para Webhooks (HMAC SHA-256)

El servidor de señalización implementa un mecanismo de verificación de integridad y autenticidad para todas las peticiones salientes hacia la API del backend mediante firmas HMAC con el algoritmo SHA-256.

### 13.1. Especificación de Encabezados HTTP

Cada petición `POST` enviada desde Go hacia tu API (como `update-call-status` o `process-call`) incluye los siguientes encabezados de seguridad:

*   **`X-Hub-Signature-256`**: Firma digital hexadecimal generada aplicando el algoritmo HMAC-SHA256 sobre el cuerpo crudo (*raw body*) de la petición JSON, utilizando el secreto compartido (`SIGNALING_SERVER_API_KEY`) como llave criptográfica.

### 13.2. Procedimiento de Verificación en el Receptor

Para validar la legitimidad de una petición entrante, el sistema receptor debe seguir estrictamente este procedimiento:

1.  **Captura del Payload**: Obtener el cuerpo crudo (*raw body*) de la petición HTTP antes de cualquier procesamiento o decodificación JSON.
2.  **Cálculo de Hash**: Calcular un hash HMAC-SHA256 utilizando el payload obtenido y el secreto compartido (`SIGNALING_SERVER_API_KEY`).
3.  **Comparación de Firmas**: Comparar el hash calculado con el valor recibido en el encabezado `X-Hub-Signature-256`. Se debe utilizar una función de comparación de tiempo constante (*constant-time comparison*) para mitigar ataques de temporización.

### 13.3. Garantías Técnicas

*   **Autenticidad de Origen**: Garantiza que la petición fue generada por el servidor de señalización autorizado.
*   **Integridad de Datos**: Asegura que el contenido del mensaje no ha sido alterado durante el tránsito; cualquier modificación en el JSON invalidará la coincidencia de la firma.
