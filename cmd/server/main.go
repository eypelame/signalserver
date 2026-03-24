package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/sirupsen/logrus"

	"signalserver/internal/auth"
	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/fcm"
	"signalserver/internal/handlers"
	"signalserver/internal/logger"
	"signalserver/internal/metrics"
	"signalserver/internal/models"
	"signalserver/internal/services"
	"signalserver/internal/validator"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"signalserver/internal/trafficlogger"
)

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func main() {
	// Limpiar pantalla de forma multiplataforma inicial
	clearScreen()

	// 1. Cargar configuración
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("Error al cargar la configuración: %v", err)
	}
	// Middleware CORS para aplicar a todas las rutas HTTP
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			// Permitir cualquier origen si no hay orígenes configurados (comportamiento anterior)
			if len(cfg.AllowedOrigins) == 0 {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				// Verificar si el origen del request está en la lista de permitidos
				for _, allowedOrigin := range cfg.AllowedOrigins {
					if origin == allowedOrigin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}
			// Encabezados adicionales para solicitudes preflight (OPTIONS)
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// 2. Inicializar logger de la aplicación (consola)
	logger.InitLogger(cfg)

	// 2.1. Ejecutar diagnósticos de salud iniciales
	if err := validator.RunHealthCheck(cfg); err != nil {
		logger.Log.Fatalf("Fallo en diagnóstico de salud: %v", err)
	}

	// 2.2. Inicializar logger de tráfico (archivo)
	var trafficLogger *trafficlogger.TrafficLogger
	if cfg.LogFilePath != "" {
		// Parsear el nivel de log para el archivo
		fileLevel, err := logrus.ParseLevel(cfg.FileLogLevel)
		if err != nil {
			logger.Log.Warnf("[VALIDATOR] - Nivel de log de archivo inválido '%s', usando 'debug' por defecto.", cfg.FileLogLevel)
			fileLevel = logrus.DebugLevel
		}

		trafficLogger, err = trafficlogger.New(cfg.LogFilePath, fileLevel)
		if err != nil {
			logger.Log.Fatalf("Fallo al inicializar el logger de tráfico: %v", err)
		}
		defer trafficLogger.Close()
		// Eliminamos este log redundante para mantener la consola limpia
	}

	// 3. Inicializar servicio JWT
	jwtService := auth.NewJWTService(cfg)

	// 4. Inicializar cliente FCM (si la ruta de la clave está configurada)
	ctx := context.Background()
	fcmClient, err := fcm.NewFCMClient(ctx, cfg)
	if err != nil {
		logger.Log.Fatalf("Fallo al inicializar el cliente FCM: %v", err)
	}
	// El log de éxito ya lo maneja el Validator robusto

	// 6. Inicializar métricas de Prometheus
	metrics.InitMetrics()

	// 6.5 Inicializar Backend Connector
	backendConnector := backend.NewConnector(cfg)

	// 6.6 Inicializar RoomManager
	roomManager := models.NewRoomManager()

	// 6.7 Inicializar Capa de Servicios
	presenceService := services.NewPresenceService(cfg, backendConnector)
	pushService := services.NewPushService(fcmClient)
	callService := services.NewCallService(cfg, presenceService, pushService, backendConnector, roomManager)

	// 7. Inicializar WebSocketHandler
	wsHandler := handlers.NewWebSocketHandler(
		cfg,
		jwtService,
		pushService,
		presenceService,
		callService,
		trafficLogger,
		backendConnector,
		roomManager,
	)

	// Iniciar el bucle de gestión de clientes y salas en una goroutine
	go wsHandler.Run()

	// Configurar rutas HTTP y WebSocket para el servidor principal
	http.HandleFunc("/ws", wsHandler.HandleConnections)
	http.HandleFunc("/health", handlers.HandleHealthCheck)
	http.HandleFunc("/api/turn-credentials", handlers.HandleTurnCredentials(cfg, jwtService))

	// Aplicar el middleware CORS a todas las rutas HTTP
	wrappedMux := corsMiddleware(http.DefaultServeMux)

	// Iniciar servidor HTTP/HTTPS principal en una goroutine
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.Port),
		Handler: wrappedMux,
	}

	go func() {
		if err := server.ListenAndServeTLS(cfg.TLS_CERT_PATH, cfg.TLS_KEY_PATH); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatalf("[SERVER] - Fallo al iniciar HTTPS: %v", err)
		}
	}()

	// Configurar el manejo de señales para un apagado limpio
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	signal.Ignore(os.Interrupt) // Ignorar Ctrl+C

	// Configurar lectura interactiva para salir con Ctrl+Q
	go func() {
		if err := keyboard.Open(); err != nil {
			logger.Log.Errorf("Error inicializando teclado: %v", err)
			return
		}
		defer keyboard.Close()

		// El mensaje de control se considera redundante tras el Validator

		for {
			_, key, err := keyboard.GetKey()
			if err != nil {
				break
			}
			if key == keyboard.KeyCtrlQ {
				logger.Log.Info("[SERVER] - Ctrl+Q detectado. Apagando...")
				c <- syscall.SIGTERM
				return
			} else if key == keyboard.KeyCtrlL {
				clearScreen()
				logger.Log.Info("[SERVER] - Pantalla limpiada manualmente con Ctrl+L.")
			}
		}
	}()

	// Servidor de métricas (si está habilitado)
	var metricsServer *http.Server
	if cfg.EnableMetrics {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsServer = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.MetricsListenAddress, cfg.MetricsPort),
			Handler: metricsMux,
		}
		go func() {
			logger.Log.Infof("[METRICS] - Servidor escuchando en %s:%d/metrics", cfg.MetricsListenAddress, cfg.MetricsPort)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Log.Errorf("[METRICS] - Fallo al iniciar servidor: %v", err)
			}
		}()
	}

	// Bloquear hasta que se reciba una señal.
	<-c

	logger.Log.Info("[SERVER] - Señal de apagado recibida. Iniciando apagado...")

	// Crear un contexto con un timeout para el apagado
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // 5 segundos para un apagado elegante
	defer cancel()

	// Intentar apagar el servidor HTTP principal
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Log.Errorf("Error al apagar el servidor HTTP principal: %v", err)
	}

	// Intentar apagar el servidor de métricas si estaba activo
	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Log.Errorf("Error al apagar el servidor de métricas: %v", err)
		}
	}

	logger.Log.Info("Servidor apagado.")
}
