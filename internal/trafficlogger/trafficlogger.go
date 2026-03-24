package trafficlogger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TrafficLogEntry representa una entrada de log de tráfico en formato JSON.
type TrafficLogEntry struct {
	Timestamp    string `json:"timestamp"`
	Level        string `json:"level"`                  // Siempre "info" o "debug" para tráfico
	Direction    string `json:"direction"`              // "IN" o "OUT"
	UserKey      string `json:"userKey,omitempty"`      // Identificador del usuario (ej. escucha_1)
	ConnectionID string `json:"connectionId,omitempty"` // ID de la conexión del socket
	Message      string `json:"message"`                // El mensaje WebSocket crudo
}

// TrafficLogger se encarga de escribir el tráfico crudo de WebSocket a un archivo.
type TrafficLogger struct {
	file     *os.File
	minLevel logrus.Level
	mu       sync.Mutex
}

// New crea e inicializa un nuevo TrafficLogger con un nivel mínimo.
func New(filePath string, minLevel logrus.Level) (*TrafficLogger, error) {
	if filePath == "" {
		return nil, fmt.Errorf("la ruta del archivo de log no puede estar vacía")
	}

	// Asegurarse de que el directorio de logs exista
	logDir := filepath.Dir(filePath)
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio de logs: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el archivo de log de tráfico %s: %w", filePath, err)
	}

	logger := &TrafficLogger{
		file:     file,
		minLevel: minLevel,
	}

	// Loguear el inicio de la sesión en formato JSON
	startEntry := TrafficLogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     "info",
		Direction: "SYSTEM",
		Message:   "Nueva sesión de log iniciada.",
	}
	jsonStartEntry, _ := json.Marshal(startEntry)
	_, _ = logger.file.WriteString(string(jsonStartEntry) + "\n")

	return logger, nil
}

// Log escribe un mensaje de bytes crudos en el archivo de log en formato JSON.
func (l *TrafficLogger) Log(prefix string, userKey string, connID string, message []byte) {
	// Consistencia semántica: El tráfico crudo se considera nivel de sev. 'debug'
	if l.minLevel > logrus.DebugLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := TrafficLogEntry{
		Timestamp:    time.Now().Format(time.RFC3339Nano),
		Level:        "debug", // Los mensajes de tráfico suelen ser de debug
		Direction:    prefix,
		UserKey:      userKey,
		ConnectionID: connID,
		Message:      string(message),
	}

	jsonEntry, err := json.Marshal(entry)
	if err != nil {
		// Si falla la serialización a JSON, loguear el error en un formato simple
		fmt.Fprintf(os.Stderr, "Error al serializar log de tráfico a JSON: %v\n", err)
		return
	}

	_, _ = l.file.WriteString(string(jsonEntry) + "\n")
}

// LogRawString escribe una cadena de texto simple en el archivo.
func (l *TrafficLogger) LogRawString(message string) {
	// Consistencia semántica: Los logs raw/sistema se consideran nivel de sev. 'info'
	if l.minLevel > logrus.InfoLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// Para mantener la consistencia, incluso los mensajes raw se intentan loguear como JSON.
	entry := TrafficLogEntry{
		Timestamp:    time.Now().Format(time.RFC3339Nano),
		Level:        "info",
		Direction:    "RAW",
		UserKey:      "",
		ConnectionID: "",
		Message:      message,
	}
	jsonEntry, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al serializar log raw a JSON: %v\n", err)
		return
	}
	_, _ = l.file.WriteString(string(jsonEntry) + "\n")
}

// Close cierra el archivo de log.
func (l *TrafficLogger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}
