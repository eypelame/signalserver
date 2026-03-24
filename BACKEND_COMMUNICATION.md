# Comunicación Servidor-a-Servidor (Go -> CodeIgniter)

Este documento describe la comunicación interna (Backend-to-Backend) entre el Servidor de Señalización WebRTC (escrito en Go) y la API principal / Base de Datos (escrita en PHP / CodeIgniter).

> [!IMPORTANT]
> A diferencia del archivo `CLIENT_COMMUNICATION_FLOW.md` (que define los WebSockets públicos para las apps móviles), los endpoints descritos aquí **no están expuestos a los clientes móviles**. Solo el servidor de Go tiene los permisos (vía `X-API-KEY`) para invocar estas rutas.

---

## 1. Autenticación y Configuración

El servidor de Go lee las credenciales desde su archivo `.env`:

*   `ApiWebhookUrl`: URL base de la API en PHP (Ej. `https://api.eypelame.com.mx`).
*   `ApiWebhookSecret`: Llave de seguridad compartida. Se envía en el header `X-API-KEY` en todas las peticiones a CodeIgniter para garantizar que solo el servidor autorizado pueda modificar el estado en BD.

**Headers Obligatorios en cada petición POST:**
```http
Content-Type: application/json
X-API-KEY: <tu_api_webhook_secret>
```

---

## 2. Endpoints Go -> CodeIgniter

El Servidor Go orquesta el flujo de llamadas, pero CodeIgniter es la "Fuente de la Verdad" (SSOT) para la facturación y la disponibilidad pública de los perfiles.

### 2.1 Actualización de Disponibilidad (`update-call-status`)

Se dispara de forma asíncrona cada vez que un usuario (Cliente o Escucha) entra o sale de una llamada, o cuando ocurre una expiración por timeout.

*   **Método:** `POST`
*   **Endpoint:** `/api/webrtc/update-call-status`
*   **Propósito:** Actualizar en tiempo real el estatus del usuario en la base de datos de CodeIgniter.

**Payload JSON:**
```json
{
  "userId": "15",
  "status": 1,
  "userType": "escucha"
}
```

**Definición de Campos:**
- `userId` (string): ID del usuario.
- `status` (int): 
  - `1` = Disponible (El usuario colgó, la llamada fue rechazada o el timeout expiró).
  - `2` = Ocupado / En Llamada (La llamada fue aceptada vía WebRTC).
- `userType` (string): Rol del usuario, usualmente `"cliente"` o `"escucha"`. CodeIgniter utiliza esto para saber qué tabla o lógica interna afectar.

---

### 2.2 Facturación y Registro de Llamada (`process-call`)

Se dispara **únicamente cuando una llamada finaliza** por cualquier motivo (colgada, desconexión por pérdida de red, timeout máximo de saldo).

*   **Método:** `POST`
*   **Endpoint:** `/api/webrtc/process-call`
*   **Propósito:** Notificarle a CodeIgniter la duración exacta en segundos para aplicar los cobros al Cliente y el pago al Escucha.

**Payload JSON:**
```json
{
  "roomId": "room_a1b2c3d4",
  "callerUserId": "8",
  "listenerUserId": "15",
  "startTime": "2026-02-23T10:00:00Z",
  "endTime": "2026-02-23T10:15:20Z",
  "durationSeconds": 920,
  "reason": "hangup"
}
```

**Definición de Campos:**
- `roomId` (string): Identificador único de la sala de la llamada.
- `callerUserId` (string): ID del usuario que originó y paga la llamada.
- `listenerUserId` (string): ID del usuario que recibe y cobra la llamada.
- `startTime` (string RFC3339): Timestamp exacto (UTC) de cuando la llamada pasó al estado "activa".
- `endTime` (string RFC3339): Timestamp exacto (UTC) del momento en que se cortó la llamada.
- `durationSeconds` (int): Duración total facturable en segundos (`endTime` - `startTime`).
- `reason` (string): Motivo del fin de la llamada. Valores comunes generados por el servidor Go:
  - `"hangup"`: Un participante cortó la llamada voluntariamente.
  - `"peer_disconnected"`: Un participante perdió conexión de internet abruptamente.
  - `"Call duration limit reached"`: El temporizador máximo de saldo/tiempo se agotó.
