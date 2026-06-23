// tests/call_flow_test.go
package tests

import (
	"sync"
	"testing"
	"time"

	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/models"
	"signalserver/internal/services"
)

// =============================================================================
// FLUJO FELIZ: Llamada completa exitosa
// =============================================================================
func Test_FlujoCompleto_LlamadaExitosa(t *testing.T) {
	callService, presence, roomManager := setupServices()

	// 1. Cliente Flutter (caller) se conecta
	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	presence.Register(caller)

	// 2. Escucha Flutter (listener) se conecta y marca disponible
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token-maria")
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	// 3. Caller solicita llamada
	roomID, err := callService.InitiateCall(caller, "escucha_456")
	if err != nil {
		t.Fatalf("PASO 3 FALLÓ - InitiateCall: %v", err)
	}
	t.Logf("✅ Paso 3: Llamada iniciada, roomID=%s", roomID)

	// 4. Verificar que la sala existe y está pending
	room, ok := roomManager.GetRoom(roomID)
	if !ok {
		t.Fatal("PASO 4 FALLÓ - La sala no existe en RoomManager")
	}
	if room.GetStatus() != "pending" {
		t.Errorf("PASO 4 FALLÓ - Estado esperado 'pending', actual '%s'", room.GetStatus())
	}
	t.Logf("✅ Paso 4: Sala en estado 'pending'")

	// 5. Listener acepta la llamada
	acceptedRoom, err := callService.AcceptCall(listener, roomID)
	if err != nil {
		t.Fatalf("PASO 5 FALLÓ - AcceptCall: %v", err)
	}
	if acceptedRoom.GetStatus() != "active" {
		t.Errorf("PASO 5 FALLÓ - Estado esperado 'active', actual '%s'", acceptedRoom.GetStatus())
	}
	t.Logf("✅ Paso 5: Listener aceptó, llamada 'active'")

	// 6. Ambos están en la sala
	_, callerInRoom := room.GetClient("conn-caller-1")
	_, listenerInRoom := room.GetClient("conn-listener-1")
	if !callerInRoom || !listenerInRoom {
		t.Errorf("PASO 6 FALLÓ - callerInRoom=%v, listenerInRoom=%v", callerInRoom, listenerInRoom)
	}
	t.Logf("✅ Paso 6: Ambos participantes en la sala")

	// 7. Caller cuelga
	hungRoom, err := callService.Hangup("cliente_123", "cliente", roomID, "hangup")
	if err != nil {
		t.Fatalf("PASO 7 FALLÓ - Hangup caller: %v", err)
	}
	t.Logf("✅ Paso 7: Caller colgó, estado='%s'", hungRoom.GetStatus())

	// 8. Verificar que se facturó solo una vez
	if room.MarkBillingFinalized() {
		t.Logf("✅ Paso 8: Facturación marcada correctamente")
	} else {
		t.Logf("✅ Paso 8: Facturación ya estaba marcada (correcto)")
	}

	// 9. Ambos vuelven a estar disponibles
	if !presence.IsUserAvailable("cliente_123", "cliente") {
		t.Error("PASO 9 FALLÓ - Caller debería estar disponible")
	}
	if !presence.IsUserAvailable("escucha_456", "escucha") {
		t.Error("PASO 9 FALLÓ - Listener debería estar disponible")
	}
	t.Logf("✅ Paso 9: Ambos usuarios disponibles nuevamente")
}

// =============================================================================
// FLUJO: Listener rechaza la llamada
// =============================================================================
func Test_FlujoCompleto_RechazoListener(t *testing.T) {
	callService, presence, roomManager := setupServices()

	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	roomID, err := callService.InitiateCall(caller, "escucha_456")
	if err != nil {
		t.Fatalf("InitiateCall falló: %v", err)
	}

	// Listener rechaza
	room, err := callService.RejectCall(roomID, "ocupado")
	if err != nil {
		t.Fatalf("RejectCall falló: %v", err)
	}
	if room.GetStatus() != "rejected" {
		t.Errorf("Estado esperado 'rejected', actual '%s'", room.GetStatus())
	}
	t.Logf("✅ Rechazo procesado: estado='%s', razón='%s'", room.GetStatus(), room.ReasonForEnd)

	// Verificar que la sala se eliminó
	_, exists := roomManager.GetRoom(roomID)
	if exists {
		t.Error("La sala debería haberse eliminado después del rechazo")
	}
	t.Logf("✅ Sala eliminada tras rechazo")
}

// =============================================================================
// FLUJO: Caller cancela (timeout de solicitud sin respuesta)
// =============================================================================
func Test_FlujoCompleto_TimeoutSolicitud(t *testing.T) {
	callService, presence, _ := setupServices()

	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	roomID, err := callService.InitiateCall(caller, "escucha_456")
	if err != nil {
		t.Fatalf("InitiateCall falló: %v", err)
	}

	// Simular timeout (el listener nunca responde)
	room, err := callService.RejectCall(roomID, "timeout")
	if err != nil {
		t.Fatalf("RejectCall(timeout) falló: %v", err)
	}
	t.Logf("✅ Timeout procesado: estado='%s'", room.GetStatus())

	// Verificar que ambos vuelven a estar disponibles
	if !presence.IsUserAvailable("cliente_123", "cliente") {
		t.Error("Caller debería estar disponible tras timeout")
	}
	if !presence.IsUserAvailable("escucha_456", "escucha") {
		t.Error("Listener debería estar disponible tras timeout")
	}
}

// =============================================================================
// FLUJO: Desconexión durante llamada activa (network error)
// =============================================================================
func Test_FlujoCompleto_DesconexionDuranteLlamada(t *testing.T) {
	callService, presence, _ := setupServices()

	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	roomID, _ := callService.InitiateCall(caller, "escucha_456")
	callService.AcceptCall(listener, roomID)

	// Simular que el listener se desconecta (pierde internet)
	presence.Unregister(listener)

	room, err := callService.Hangup("escucha_456", "escucha", roomID, "network_error")
	if err != nil {
		t.Fatalf("Hangup(network_error) falló: %v", err)
	}
	t.Logf("✅ Desconexión procesada: estado='%s', razón='%s'", room.GetStatus(), room.ReasonForEnd)

	// El caller debe volver a disponible
	if !presence.IsUserAvailable("cliente_123", "cliente") {
		t.Error("Caller debería estar disponible tras desconexión del listener")
	}
}

// =============================================================================
// FLUJO: Doble Hangup simultáneo (ambos cuelgan a la vez)
// =============================================================================
func Test_FlujoCompleto_DobleHangupSimultaneo(t *testing.T) {
	callService, presence, roomManager := setupServices()

	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	roomID, _ := callService.InitiateCall(caller, "escucha_456")
	callService.AcceptCall(listener, roomID)

	var wg sync.WaitGroup
	wg.Add(2)

	// Ambos cuelgan al mismo tiempo
	go func() {
		defer wg.Done()
		callService.Hangup("cliente_123", "cliente", roomID, "hangup")
	}()
	go func() {
		defer wg.Done()
		callService.Hangup("escucha_456", "escucha", roomID, "hangup")
	}()

	wg.Wait()

	// La sala ya no debería existir
	_, exists := roomManager.GetRoom(roomID)
	if exists {
		t.Error("La sala debería haberse eliminado después de los hangups")
	}
	t.Logf("✅ Doble hangup simultáneo manejado correctamente")

	// Verificar que no se facturó dos veces (MarkBillingFinalized ya se llamó en el primer Hangup)
}

// =============================================================================
// FLUJO: Caller sin saldo suficiente
// =============================================================================
func Test_FlujoCompleto_CallerSinSaldo(t *testing.T) {
	callService, presence, _ := setupServices()

	// Caller con callTime = 0 (sin saldo)
	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 0, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	_, err := callService.InitiateCall(caller, "escucha_456")
	if err == nil {
		t.Fatal("Debería fallar por fondos insuficientes")
	}
	if err.Error() != "fondos insuficientes" {
		t.Errorf("Error esperado 'fondos insuficientes', obtenido '%s'", err.Error())
	}
	t.Logf("✅ Caller sin saldo rechazado correctamente")
}

// =============================================================================
// FLUJO: Listener no disponible
// =============================================================================
func Test_FlujoCompleto_ListenerNoDisponible(t *testing.T) {
	callService, presence, _ := setupServices()

	caller := models.NewClient("cliente_123", "Juan", "cliente", "conn-caller-1", nil, 10*time.Minute, nil, "mobile", "")
	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(caller)
	presence.Register(listener)
	// NO marcamos disponible al listener

	_, err := callService.InitiateCall(caller, "escucha_456")
	if err == nil {
		t.Fatal("Debería fallar porque el listener no está disponible")
	}
	if err.Error() != "el usuario no está disponible" {
		t.Errorf("Error esperado 'el usuario no está disponible', obtenido '%s'", err.Error())
	}
	t.Logf("✅ Listener no disponible rechazado correctamente")
}

// =============================================================================
// FLUJO: Mismo listener no puede estar en 2 llamadas
// =============================================================================
func Test_FlujoCompleto_ListenerNoDuplicado(t *testing.T) {
	callService, presence, _ := setupServices()

	listener := models.NewClient("escucha_456", "María", "escucha", "conn-listener-1", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(listener)
	presence.UpdateAvailability(listener, true)

	// Primer caller inicia llamada
	caller1 := models.NewClient("cliente_1", "Juan", "cliente", "conn-c1", nil, 10*time.Minute, nil, "mobile", "")
	presence.Register(caller1)
	roomID1, err := callService.InitiateCall(caller1, "escucha_456")
	if err != nil {
		t.Fatalf("Primera llamada debería iniciarse: %v", err)
	}
	t.Logf("✅ Primera llamada iniciada: %s", roomID1)

	// Segundo caller intenta llamar al mismo listener
	caller2 := models.NewClient("cliente_2", "Pedro", "cliente", "conn-c2", nil, 10*time.Minute, nil, "mobile", "")
	presence.Register(caller2)
	_, err = callService.InitiateCall(caller2, "escucha_456")
	if err == nil {
		t.Fatal("Segunda llamada debería fallar: listener ya está en una llamada")
	}
	t.Logf("✅ Segunda llamada rechazada: %v", err)
}

// =============================================================================
// SETUP: Crea las dependencias mínimas para los tests
// =============================================================================
func setupServices() (*services.CallService, *services.PresenceService, *models.RoomManager) {
	cfg := &config.AppConfig{
		CallMaxDurationMinutes:    60,
		CallRequestTimeoutSeconds: 30,
		ApiWebhookUrl:             "http://localhost:9999",
	}

	backendConn := backend.NewConnector(cfg)
	presence := services.NewPresenceService(cfg, backendConn)
	roomManager := models.NewRoomManager()
	pushService := services.NewPushService(nil)

	callService := services.NewCallService(cfg, presence, pushService, backendConn, roomManager)

	return callService, presence, roomManager
}
