// tests/presence_test.go
package tests

import (
	"sync"
	"testing"

	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/models"
	"signalserver/internal/services"
)

// Test_SetUserAvailability_Concurrente verifica que múltiples goroutines
// modificando la disponibilidad no causen race conditions.
func Test_SetUserAvailability_Concurrente(t *testing.T) {
	cfg := &config.AppConfig{
		ApiWebhookUrl: "http://localhost:9999",
	}

	backendConn := backend.NewConnector(cfg)
	presence := services.NewPresenceService(cfg, backendConn)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			isAvailable := idx%2 == 0
			presence.SetUserAvailability("user_1", "escucha", isAvailable)
		}(i)
	}

	wg.Wait()
	// Si no hay panic ni race detectada por -race, el test pasa
}

// Test_Register_Unregister_Ciclo rapido de conexión/desconexión.
func Test_Register_Unregister_Ciclo(t *testing.T) {
	cfg := &config.AppConfig{
		MaxConnPerUser: 3,
		ApiWebhookUrl:  "http://localhost:9999",
	}

	backendConn := backend.NewConnector(cfg)
	presence := services.NewPresenceService(cfg, backendConn)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client := models.NewClient("user_1", "Test", "escucha", "conn-"+string(rune(idx)), nil, 0, nil, "mobile", "")
			presence.Register(client)
			presence.Unregister(client)
		}(i)
	}

	wg.Wait()
}

// Test_BuildClientsList_NoPanic llama a BuildClientsList mientras se modifican
// datos concurrentemente.
func Test_BuildClientsList_NoPanic(t *testing.T) {
	cfg := &config.AppConfig{
		ApiWebhookUrl: "http://localhost:9999",
	}

	backendConn := backend.NewConnector(cfg)
	presence := services.NewPresenceService(cfg, backendConn)

	// Registrar varios clientes
	for i := 0; i < 20; i++ {
		client := models.NewClient("user_"+string(rune(i)), "Test", "escucha", "conn-"+string(rune(i)), nil, 0, nil, "mobile", "")
		presence.Register(client)
	}

	var wg sync.WaitGroup

	// Goroutine que modifica disponibilidad constantemente
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			presence.SetUserAvailability("user_1", "escucha", i%2 == 0)
		}
	}()

	// Goroutine que lee la lista constantemente
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = presence.BuildClientsList()
		}
	}()

	wg.Wait()
	// Si no hay panic, el test pasa
}
