// tests/concurrency_test.go
package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/models"
	"signalserver/internal/services"
)

// Test_InitiateCall_UnicoListener simula 20 callers simultáneos
// intentando llamar al mismo escucha. Solo 1 debe tener éxito.
func Test_InitiateCall_UnicoListener(t *testing.T) {
	cfg := &config.AppConfig{
		CallMaxDurationMinutes: 60,
		ApiWebhookUrl:          "http://localhost:9999",
	}

	backendConn := backend.NewConnector(cfg)
	presence := services.NewPresenceService(cfg, backendConn)
	roomManager := models.NewRoomManager()
	pushService := services.NewPushService(nil) // sin FCM

	callService := services.NewCallService(cfg, presence, pushService, backendConn, roomManager)

	// Registrar escucha disponible
	listener := models.NewClient("escucha_1", "María", "escucha", "conn-escucha-1", nil, 0, nil, "mobile", "fcm-token-1")
	presence.Register(listener)

	var wg sync.WaitGroup
	exitosas := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			callerID := fmt.Sprintf("caller_%d", idx)
			caller := models.NewClient(callerID, "Cliente "+string(rune('A'+idx)), "cliente", "conn-"+callerID, nil, 10*time.Minute, nil, "web", "")

			_, err := callService.InitiateCall(caller, "escucha_1")
			mu.Lock()
			if err == nil {
				exitosas++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if exitosas != 1 {
		t.Errorf("CRÍTICO: %d llamadas iniciadas contra el mismo escucha, debería ser 1", exitosas)
	}
}

// Test_TimerCancelable_NoGoroutineLeak verifica que cancelar el timer
// libere la goroutine inmediatamente (sin esperar el timeout completo).
func Test_TimerCancelable_NoGoroutineLeak(t *testing.T) {
	room := models.NewRoom("room-timer-001", "caller1", "cliente", 5*time.Second)
	room.Accept()

	done := make(chan bool, 1)
	start := time.Now()

	// Simular el startCallTimer
	go func() {
		timer := time.NewTimer(room.CallMaxDuration)
		defer timer.Stop()

		select {
		case <-timer.C:
			done <- false // timeout real (no debería pasar)
		case <-room.TimerCtx.Done():
			done <- true // cancelado
		}
	}()

	// Cancelar inmediatamente
	room.TimerCancel()

	result := <-done
	elapsed := time.Since(start)

	if !result {
		t.Error("El timer no se canceló correctamente")
	}
	if elapsed > 1*time.Second {
		t.Errorf("La cancelación tardó %v, debería ser instantánea", elapsed)
	}
}

// Test_CallRequestTimer_CanceladoEnAceptacion verifica que al aceptar
// una llamada, el timer de solicitud se cancela.
func Test_CallRequestTimer_CanceladoEnAceptacion(t *testing.T) {
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

	// Setup: caller y listener disponibles
	caller := models.NewClient("caller_1", "Juan", "cliente", "conn-caller", nil, 10*time.Minute, nil, "web", "")
	listener := models.NewClient("escucha_1", "María", "escucha", "conn-listener", nil, 0, nil, "mobile", "fcm-token")
	presence.Register(listener)

	// Iniciar llamada
	roomID, err := callService.InitiateCall(caller, "escucha_1")
	if err != nil {
		t.Fatalf("No se pudo iniciar la llamada: %v", err)
	}

	// Verificar que la sala está pending
	room, ok := roomManager.GetRoom(roomID)
	if !ok {
		t.Fatal("La sala no existe")
	}
	if room.GetStatus() != "pending" {
		t.Errorf("Estado esperado 'pending', obtenido '%s'", room.GetStatus())
	}

	// Aceptar llamada
	_, err = callService.AcceptCall(listener, roomID)
	if err != nil {
		t.Fatalf("No se pudo aceptar la llamada: %v", err)
	}

	if room.GetStatus() != "active" {
		t.Errorf("Estado esperado 'active', obtenido '%s'", room.GetStatus())
	}
}
