// tests/billing_test.go
package tests

import (
	"sync"
	"testing"
	"time"

	"signalserver/internal/models"
)

// Test_DobleFacturacion_Prevenida simula 100 goroutines intentando facturar
// la misma sala simultáneamente. Solo UNA debe lograrlo.
func Test_DobleFacturacion_Prevenida(t *testing.T) {
	room := models.NewRoom("room-billing-001", "caller1", "cliente", 30*time.Minute)
	room.Accept()

	var wg sync.WaitGroup
	resultados := make([]bool, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			resultados[index] = room.MarkBillingFinalized()
		}(i)
	}

	wg.Wait()

	exitosas := 0
	for _, ok := range resultados {
		if ok {
			exitosas++
		}
	}

	if exitosas != 1 {
		t.Errorf("CRÍTICO: Se facturó %d veces, debería ser 1 sola", exitosas)
	}
}

// Test_Complete_Sala_Idempotente 50 goroutines llaman Complete simultáneamente.
// El estado final debe ser "completed" y no debe haber panic.
func Test_Complete_Sala_Idempotente(t *testing.T) {
	room := models.NewRoom("room-complete-001", "caller1", "cliente", 30*time.Minute)
	room.Accept()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room.Complete("hangup_cliente")
		}()
	}

	wg.Wait()

	if room.GetStatus() != "completed" {
		t.Errorf("Estado esperado 'completed', obtenido '%s'", room.GetStatus())
	}
}

// Test_Facturacion_Unica_Con_Multiples_Hangups simula el escenario real:
// un hangup por desconexión + un hangup por timeout.
func Test_Facturacion_Unica_Con_Multiples_Hangups(t *testing.T) {
	room := models.NewRoom("room-real-001", "caller1", "cliente", 100*time.Millisecond)
	room.Accept()

	var wg sync.WaitGroup

	// Simular hangup por desconexión
	wg.Add(1)
	go func() {
		defer wg.Done()
		room.Complete("network_error")
		if room.MarkBillingFinalized() {
			// Facturar (simulado)
		}
	}()

	// Simular hangup por timeout (llega 200ms después)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		room.Complete("timeout")
		if room.MarkBillingFinalized() {
			// Intentar facturar otra vez
		}
	}()

	wg.Wait()

	// Verificar que no se puede facturar dos veces
	if room.MarkBillingFinalized() {
		t.Error("CRÍTICO: Se pudo facturar después de que ya fue facturada")
	}
}
