package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ActiveConnections es un Gauge que registra el número actual de conexiones WebSocket activas por rol.
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_active_connections_total",
			Help: "Número total de conexiones WebSocket activas por rol.",
		},
		[]string{"role"},
	)

	// AvailableUsers es un Gauge que registra el número de usuarios ONLINE y LIBRES por rol.
	AvailableUsers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_available_users_total",
			Help: "Número de usuarios conectados y disponibles para recibir llamadas por rol.",
		},
		[]string{"role"},
	)

	// ActiveCalls es un Gauge que registra el número actual de participantes en llamadas activas por rol.
	ActiveCalls = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_active_calls_total",
			Help: "Número total de participantes en llamadas activas por rol.",
		},
		[]string{"role"},
	)

	// CallDurationHistogram registra la distribución de la duración de las llamadas completadas.
	CallDurationHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "websocket_call_duration_seconds",
			Help:    "Distribución de la duración de las llamadas en segundos.",
			Buckets: []float64{10, 30, 60, 180, 300, 600, 1200, 1800, 3600},
		},
	)

	// BackendLatencyHistogram registra la latencia de las peticiones al API central (PHP).
	BackendLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "backend_request_duration_seconds",
			Help:    "Latencia de las peticiones al backend PHP.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	// RateLimitDropsTotal registra mensajes descartados por exceso de velocidad.
	RateLimitDropsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_ratelimit_drops_total",
			Help: "Número total de mensajes descartados por rate-limit por rol.",
		},
		[]string{"role"},
	)

	// MessagesReceivedTotal es un Counter que registra el número total de mensajes recibidos.
	MessagesReceivedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "websocket_messages_received_total",
			Help: "Número total de mensajes WebSocket recibidos.",
		},
	)

	// MessagesSentTotal es un Counter que registra el número total de mensajes enviados.
	MessagesSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "websocket_messages_sent_total",
			Help: "Número total de mensajes WebSocket enviados.",
		},
	)

	// CallRequestsTotal es un Counter que registra el número total de solicitudes de llamada.
	CallRequestsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "call_requests_total",
			Help: "Número total de solicitudes de llamada.",
		},
	)

	// CallAcceptsTotal es un Counter que registra el número total de aceptaciones de llamada.
	CallAcceptsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "call_accepts_total",
			Help: "Número total de aceptaciones de llamada.",
		},
	)

	// CallRejectsTotal es un Counter que registra el número total de rechazos de llamada.
	CallRejectsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "call_rejects_total",
			Help: "Número total de rechazos de llamada.",
		},
	)

	// CallHangupTotal es un Counter que registra el número total de finalizaciones de llamada.
	CallHangupTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "call_hangup_total",
			Help: "Número total de finalizaciones de llamada.",
		},
	)

	// ErrorsTotal es un Counter que registra el número total de errores internos.
	ErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "server_errors_total",
			Help: "Número total de errores internos del servidor.",
		},
	)

	// FCMNotificationsStatusTotal registra el número total de notificaciones FCM enviadas por estado.
	FCMNotificationsStatusTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fcm_notifications_status_total",
			Help: "Número total de notificaciones FCM enviadas por estado (success/error).",
		},
		[]string{"status"},
	)

	// RedisPublishesTotal es un Counter que registra el número total de publicaciones en Redis.
	RedisPublishesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_publishes_total",
			Help: "Número total de mensajes publicados en Redis.",
		},
	)

	// RedisSubscribesTotal es un Counter que registra el número total de suscripciones a Redis.
	RedisSubscribesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_subscribes_total",
			Help: "Número total de suscripciones a canales de Redis.",
		},
	)
)

// InitMetrics inicializa las métricas de Prometheus.
func InitMetrics() {
	// Las métricas se registran automáticamente con promauto.New*.
	// No se necesita lógica adicional aquí a menos que se requiera un registro personalizado.
}
