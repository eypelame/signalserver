package config

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/joho/godotenv"
)

// AppConfig define la estructura para la configuración de la aplicación.
type AppConfig struct {
	Port                    int           `env:"PORT" envDefault:"8080"`
	ListenAddress           string        `env:"LISTEN_ADDRESS" envDefault:"0.0.0.0"`
	TLS_CERT_PATH           string        `env:"TLS_CERT_PATH,required"` // Ruta al archivo del certificado TLS
	TLS_KEY_PATH            string        `env:"TLS_KEY_PATH,required"`  // Ruta al archivo de la clave privada TLS
	WebSocketPingPeriod     time.Duration `env:"WEBSOCKET_PING_PERIOD" envDefault:"10s"`
	WebSocketPongWait       time.Duration `env:"WEBSOCKET_PONG_WAIT" envDefault:"60s"`
	WebSocketMaxMessageSize int64         `env:"WEBSOCKET_MAX_MESSAGE_SIZE" envDefault:"65536"` // Tamaño máximo de mensaje en bytes
	WriteTimeout            time.Duration `env:"WEBSOCKET_WRITE_TIMEOUT" envDefault:"5s"`
	MaxTotalConnections     int           `env:"MAX_TOTAL_CONNECTIONS" envDefault:"1000"`
	RateLimitPerSecond      float64       `env:"RATE_LIMIT_PER_SECOND" envDefault:"10"`
	RateLimitBurst          int           `env:"RATE_LIMIT_BURST" envDefault:"20"`
	JWTSecret               string        `env:"JWT_SECRET,required"`
	JWTIssuer               string        `env:"JWT_ISSUER,required"`
	AllowedOrigins          []string      `env:"ALLOWED_ORIGINS" envSeparator:","`

	FCMServiceAccountKeyPath  string `env:"FCM_SERVICE_ACCOUNT_KEY_PATH"`
	EnableMetrics             bool   `env:"ENABLE_METRICS" envDefault:"false"`
	MetricsPort               int    `env:"METRICS_PORT" envDefault:"8081"`
	MetricsListenAddress      string `env:"METRICS_LISTEN_ADDR" envDefault:"127.0.0.1"` // Dirección de escucha para el servidor de métricas
	ConsoleLogLevel           string `env:"CONSOLE_LOG_LEVEL" envDefault:"info"`        // debug, info, warn, error
	FileLogLevel              string `env:"FILE_LOG_LEVEL" envDefault:"debug"`          // debug, info, warn, error
	LogFilePath               string `env:"LOG_FILE_PATH"`                              // Ruta al archivo de log
	ShowBanner                bool   `env:"SHOW_BANNER" envDefault:"true"`              // Mostrar banner al inicio
	ConsoleVerbose            bool   `env:"CONSOLE_VERBOSE" envDefault:"true"`          // Controla la verbosidad de la salida en consola
	CallMaxDurationMinutes    int    `env:"CALL_MAX_DURATION_MINUTES" envDefault:"60"`
	CallRequestTimeoutSeconds int    `env:"CALL_REQUEST_TIMEOUT_SECONDS" envDefault:"30"`
	MaxConnPerUser            int    `env:"MAX_CONN_PER_USER" envDefault:"1"` // Máximo de conexiones WebSocket permitidas por usuario.
	ApiWebhookUrl             string `env:"API_WEBHOOK_URL,required"`
	ApiWebhookSecret          string `env:"SIGNALING_SERVER_API_KEY,required"`
	TurnRestApiSecret         string `env:"TURN_REST_API_SECRET,required"` // Secreto compartido con Coturn
	TurnServerUrls            string `env:"TURN_SERVER_URLS,required"`     // Ej: "turn:ice.eypelame.com.mx:3478,turn:ice.eypelame.com.mx:3478?transport=tcp"
	StunServerUrls            string `env:"STUN_SERVER_URLS" envDefault:"stun:stun.l.google.com:19302,stun:ice.eypelame.com.mx:3478"`
	TurnCredentialsTTL        int    `env:"TURN_CREDENTIALS_TTL_HOURS" envDefault:"24"`
	CleanupIntervalSeconds    int    `env:"CLEANUP_INTERVAL_SECONDS" envDefault:"60"`

	// Optimizaciones de Audio WebRTC
	EnableWebRTCAudioOptimization bool `env:"ENABLE_WEBRTC_AUDIO_OPTIMIZATION" envDefault:"false"`
	WebRTCAudioBitrate            int  `env:"WEBRTC_AUDIO_BITRATE" envDefault:"64000"`
}

// GetConfig carga la configuración desde un archivo .env y variables de entorno.
func GetConfig() (*AppConfig, error) {
	// Cargar .env si existe
	_ = godotenv.Load()

	cfg := &AppConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("fallo al cargar la configuración: %w", err)
	}

	// Validaciones adicionales
	if cfg.TLS_CERT_PATH == "" {
		return nil, fmt.Errorf("TLS_CERT_PATH es requerido")
	}
	if cfg.TLS_KEY_PATH == "" {
		return nil, fmt.Errorf("TLS_KEY_PATH es requerido")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET es requerido")
	}
	if cfg.JWTIssuer == "" {
		return nil, fmt.Errorf("JWT_ISSUER es requerido")
	}
	if cfg.ApiWebhookUrl == "" {
		return nil, fmt.Errorf("API_WEBHOOK_URL es requerido")
	}
	if cfg.ApiWebhookSecret == "" {
		return nil, fmt.Errorf("SIGNALING_SERVER_API_KEY es requerido")
	}
	if cfg.TurnRestApiSecret == "" {
		return nil, fmt.Errorf("TURN_REST_API_SECRET es requerido")
	}
	if cfg.TurnServerUrls == "" {
		return nil, fmt.Errorf("TURN_SERVER_URLS es requerido")
	}

	// Verificar si los archivos de certificado y clave existen
	if _, err := os.Stat(cfg.TLS_CERT_PATH); os.IsNotExist(err) {
		return nil, fmt.Errorf("el archivo de certificado TLS no existe en: %s", cfg.TLS_CERT_PATH)
	}
	if _, err := os.Stat(cfg.TLS_KEY_PATH); os.IsNotExist(err) {
		return nil, fmt.Errorf("el archivo de clave TLS no existe en: %s", cfg.TLS_KEY_PATH)
	}

	// Si FCM_SERVICE_ACCOUNT_KEY_PATH está configurado, verificar que el archivo exista
	if cfg.FCMServiceAccountKeyPath != "" {
		if _, err := os.Stat(cfg.FCMServiceAccountKeyPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("el archivo de clave de cuenta de servicio FCM no existe en: %s", cfg.FCMServiceAccountKeyPath)
		}
	}

	return cfg, nil
}
