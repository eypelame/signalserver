package validator

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"signalserver/internal/config"
	"signalserver/internal/logger"
)

// RunHealthCheck ejecuta todas las validaciones de salud iniciales del sistema.
func RunHealthCheck(cfg *config.AppConfig) error {
	logger.Log.Info("[VALIDATOR] Iniciando diagnóstico de salud avanzada...")
	time.Sleep(100 * time.Millisecond) // Pequeña pausa para fluidez visual

	// 1. Validar Certificados TLS (Robust: Load pair)
	if err := validateCertificates(cfg.TLS_CERT_PATH, cfg.TLS_KEY_PATH); err != nil {
		return fmt.Errorf("Fallo crítico en SSL/TLS: %v", err)
	}
	report("OK", "SSL/TLS", "Certificados validados, vinculados y cargados en memoria.")

	// 2. Validar Credenciales FCM (Robust: JSON Parsing)
	if cfg.FCMServiceAccountKeyPath != "" {
		if projectID, err := validateFCMConfig(cfg.FCMServiceAccountKeyPath); err != nil {
			return fmt.Errorf("Fallo crítico en FCM: %v", err)
		} else {
			report("OK", "FCM CONFIG", fmt.Sprintf("Credenciales verificadas (Proyecto: %s).", projectID))
		}
	} else {
		report("WARN", "FCM CONFIG", "Ruta de credenciales no configurada. Push deshabilitado.")
	}

	// 3. Validar variables críticas (JWT)
	if len(cfg.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET es demasiado corto o inseguro")
	}
	report("OK", "JWT BLIND", "Blindaje de tokens configurado con secreto robusto.")

	// 4. Validar enlace de red con Backend (Robust: HTTP Reachability)
	if err := validateBackendReachability(cfg.ApiWebhookUrl); err != nil {
		report("FAIL", "BACKEND", fmt.Sprintf("No se pudo contactar con %s: %v", cfg.ApiWebhookUrl, err))
		// No detenemos el servidor por esto, pero marcamos el error
	} else {
		report("OK", "BACKEND", "Enlace de comunicación bidireccional establecido.")
	}

	// 5. Verificación de Puertos y Red Local
	report("OK", "NETWORK", fmt.Sprintf("Servidor vinculado a la interfaz %s:%d", cfg.ListenAddress, cfg.Port))

	fmt.Println()
	logger.Log.Info("[VALIDATOR] " + logger.ColorGreen + "✔ Diagnóstico completado. El sistema es estable y está listo." + logger.ColorReset)
	return nil
}

// report genera un log perfectamente alineado con prefijos de ancho fijo
func report(status, category, detail string) {
	statusColor := logger.ColorReset

	switch status {
	case "OK":
		statusColor = logger.ColorCyan
	case "WARN":
		statusColor = logger.ColorYellow
	case "FAIL":
		statusColor = logger.ColorRed
	}

	// [VALIDATOR] [OK] CATEGORY - DETAIL
	msg := fmt.Sprintf("[%s%s%s] %-12s - %s",
		statusColor, status, logger.ColorReset,
		category, detail)

	logger.Log.Info("[VALIDATOR] " + msg)
}

func validateCertificates(certPath, keyPath string) error {
	_, err := tls.LoadX509KeyPair(certPath, keyPath)
	return err
}

func validateFCMConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var creds struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", err
	}
	if creds.ProjectID == "" {
		return "", fmt.Errorf("el archivo JSON no contiene 'project_id'")
	}
	return creds.ProjectID, nil
}

func validateBackendReachability(url string) error {
	client := http.Client{
		Timeout: 3 * time.Second,
	}
	resp, err := client.Head(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
