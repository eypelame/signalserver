package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"signalserver/internal/auth"
	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/logger"
	"signalserver/internal/metrics"
	"signalserver/internal/models"
	"signalserver/internal/services"
	"signalserver/internal/trafficlogger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// WebSocketHandler gestiona las conexiones WebSocket y el enrutamiento de mensajes.
type WebSocketHandler struct {
	Config          *config.AppConfig
	JWTValidator    auth.JWTValidator
	PushService     *services.PushService
	PresenceService *services.PresenceService
	CallService     *services.CallService
	TrafficLogger   *trafficlogger.TrafficLogger
	Backend         *backend.Connector
	upgrader        websocket.Upgrader
	RoomManager     *models.RoomManager
	register        chan *models.Client

	unregister chan *models.Client
}

// UserConnections envuelve un mapa de clientes de un usuario con su propio mutex.
type UserConnections struct {
	Connections map[string]*models.Client
	Mu          sync.RWMutex
}

// NewWebSocketHandler crea y retorna una nueva instancia de WebSocketHandler.
func NewWebSocketHandler(cfg *config.AppConfig, jwtValidator auth.JWTValidator, pushService *services.PushService, presenceService *services.PresenceService, callService *services.CallService, trafficLogger *trafficlogger.TrafficLogger, backendConnector *backend.Connector, roomManager *models.RoomManager) *WebSocketHandler {

	return &WebSocketHandler{
		Config:          cfg,
		JWTValidator:    jwtValidator,
		PushService:     pushService,
		PresenceService: presenceService,
		CallService:     callService,
		TrafficLogger:   trafficLogger,
		Backend:         backendConnector,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				if len(cfg.AllowedOrigins) > 0 {
					for _, allowedOrigin := range cfg.AllowedOrigins {
						if origin == allowedOrigin {
							return true
						}
					}
				} else {
					return true
				}
				return false
			},
		},
		RoomManager: roomManager,
		register:    make(chan *models.Client, 100),
		unregister:  make(chan *models.Client, 100),
	}
}

// Run inicia el bucle principal del WebSocketHandler.
func (h *WebSocketHandler) Run() {
	go h.startRoomCleanupWorker()
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)
		case client := <-h.unregister:
			h.handleUnregister(client)
		}
	}
}

func (h *WebSocketHandler) handleRegister(client *models.Client) {
	totalConns, clientsToEvict := h.PresenceService.Register(client)

	logger.Log.Infof("[SYSTEM] [ACTION:register] [USER: %s] - Cliente registrado. Total conexiones usuario: %d.", client.LogKey(), totalConns)

	for _, oldClient := range clientsToEvict {
		h.sendSessionReplacedMessage(oldClient, "Nueva conexión iniciada desde otra ubicación.")
		go func(oc *models.Client) {
			h.unregister <- oc
		}(oldClient)
	}

	h.sendConnectionAck(client)

	// Solo marcar como disponible (1) si no está ya en una llamada activa/pendiente
	if client.UserRole == "escucha" {
		if !h.RoomManager.IsUserInActiveCall(client.UserID) {
			go h.Backend.UpdateCallStatus(client.UserID, 1, client.UserRole)
		} else {
			logger.Log.Infof("[USER: %s] [CONN: %s] - Reconexión detectada durante llamada activa. Manteniendo estado ocupado.", client.LogKey(), client.ConnectionID)

			// Si el usuario está en una sala, entregarle los mensajes del búfer inmediatamente
			if room, ok := h.RoomManager.FindRoomByUserID(client.UserID); ok {
				msgs := room.FlushBuffer(client.UserID)
				if len(msgs) > 0 {
					logger.Log.Infof("[SIGNAL-BUFFER] [RoomID: %s] - Entregando %d mensajes acumulados al usuario %s tras reconexión.", room.ID, len(msgs), client.LogKey())
					for _, m := range msgs {
						m.To = client.ConnectionID
						h.sendMessageToClient(client, m)
					}
				}
			}
		}
	}

	h.broadcastClientList()
}

func (h *WebSocketHandler) handleUnregister(client *models.Client) {
	unregistered, isLastConnection := h.PresenceService.Unregister(client)
	if unregistered {
		close(client.Send)
		logger.Log.Infof("[SYSTEM] [ACTION:unregister] [USER: %s] - Cliente desregistrado [CONN: %s].", client.LogKey(), client.ConnectionID)

		if isLastConnection {
			// El estado en la base de datos ya no se marca como 4 (No disponible) automáticamente
			// para permitir que el escucha sea contactado vía Push si tiene token.
			// Solo el logout explícito cambiará el estado a 4.
		}

		// Limpiar llamadas de esta conexión
		for roomID, room := range h.RoomManager.GetAllRoomsSnapshot() {
			if _, ok := room.GetClient(client.ConnectionID); ok {
				status := room.GetStatus()
				if status == "active" || status == "pending" {
					h.CallService.Hangup(client.UserID, client.UserRole, roomID, "network_error")

					otherClient, found := room.GetOtherClient(client.ConnectionID)
					if found {
						h.sendHangupMessage(otherClient, roomID, "peer_disconnected", fmt.Sprintf("El usuario %s se ha desconectado.", client.UserID))
					}
				}
				break
			}
		}
		h.broadcastClientList()
	}
}

func (h *WebSocketHandler) broadcastClientList() {
	payload := h.PresenceService.BuildClientsList()
	data, _ := json.Marshal(payload)
	message := models.Message{
		Type: "clients-list",
		Data: data,
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		logger.Log.Errorf("[BACKEND] Error al serializar la lista de clientes: %v", err)
		return
	}

	for _, client := range h.PresenceService.GetAllConnections() {
		select {
		case client.Send <- jsonMessage:
		case <-time.After(h.Config.WriteTimeout):
			logger.Log.Warnf("[USER: %s] [CONN:%s] - Timeout al enviar lista de clientes.", client.LogKey(), client.ConnectionID)
		}
	}
}

func (h *WebSocketHandler) HandleConnections(w http.ResponseWriter, r *http.Request) {
	tokenString := r.Header.Get("Sec-WebSocket-Protocol")
	if tokenString == "" {
		tokenString = r.URL.Query().Get("token")
	}
	clientType := r.URL.Query().Get("clientType")
	fcmToken := r.URL.Query().Get("fcmToken")

	logger.Log.Infof("[WebSocket] - Intento de conexión: clientType=%s, fcmTokenPresente=%t", clientType, fcmToken != "")

	if tokenString == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	claims, err := h.JWTValidator.ValidateToken(tokenString)
	if err != nil {
		logger.Log.Warnf("[WebSocket] - Fallo de validación JWT: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	logger.Log.Infof("[WebSocket] - JWT [valido] [ROLE: %s] | [USER: %s_%s]", claims.Type, claims.Type, claims.UserID)

	if clientType == "" || (clientType != "web" && clientType != "mobile") {
		logger.Log.Warnf("[WebSocket] - clientType inválido o ausente: %s", clientType)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	userID := claims.UserID
	userName := claims.UserName
	userRole := claims.Type
	callTime := time.Duration(claims.CallTime) * time.Minute

	if userRole == "escucha" && fcmToken == "" {
		errMsg := "Connection Rejected: fcmToken is mandatory for role 'escucha' to receive calls via Push"
		logger.Log.Errorf("[WebSocket] [USER: %s] - %s", userID, errMsg)
		if h.TrafficLogger != nil {
			h.TrafficLogger.LogRawString(fmt.Sprintf("[ERROR] [USER: %s] - %s", userID, errMsg))
		}
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Log.Errorf("[WebSocket] - Fallo al actualizar a WebSocket: %v", err)
		return
	}

	connectionID := uuid.New().String()
	limiter := rate.NewLimiter(rate.Limit(h.Config.RateLimitPerSecond), h.Config.RateLimitBurst)
	client := models.NewClient(userID, userName, userRole, connectionID, conn, callTime, limiter, clientType, fcmToken)

	if fcmToken != "" {
		h.PresenceService.UpdatePushToken(userID, userRole, fcmToken)
	}

	metrics.ActiveConnections.WithLabelValues(client.UserRole).Inc()
	go h.writePump(client)
	go h.readPump(client)

	h.register <- client
}

func (h *WebSocketHandler) readPump(client *models.Client) {
	defer func() {
		h.unregister <- client
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(h.Config.WebSocketMaxMessageSize)
	client.Conn.SetReadDeadline(time.Now().Add(h.Config.WebSocketPongWait))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(h.Config.WebSocketPongWait))
		return nil
	})

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			break
		}
		if h.TrafficLogger != nil {
			h.TrafficLogger.Log("IN", client.LogKey(), client.ConnectionID, message)
		}
		metrics.MessagesReceivedTotal.Inc()

		if !client.RateLimiter.Allow() {
			h.sendError(client, "RATE_LIMIT_EXCEEDED", "Demasiadas solicitudes.")
			continue
		}

		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			h.sendError(client, "INVALID_JSON", "Mensaje JSON malformado.")
			continue
		}

		if err := h.validateMessage(&msg); err != nil {
			h.sendError(client, "INVALID_MESSAGE", err.Error())
			continue
		}

		switch msg.Type {
		case "disconnect":
			return
		case "update-availability":
			h.handleUpdateAvailability(client, msg)
		case "update-push-token":
			h.handleUpdatePushToken(client, msg)
		case "call-request":
			h.handleCallRequest(client, msg)
		case "call-accept":
			h.handleCallAccept(client, msg)
		case "call-reject":
			h.handleCallReject(client, msg)
		case "hangup":
			h.handleHangup(client, msg)
		case "clear-fcm-and-disconnect":
			h.handleClearFcmAndDisconnect(client, msg)
		case "connection-report":
			h.handleConnectionReport(client, msg)
		case "ice-candidate", "sdp-offer", "sdp-answer":
			h.handleSignalingMessage(client, msg)
		default:
			h.sendError(client, "UNKNOWN_MESSAGE_TYPE", fmt.Sprintf("Tipo de mensaje desconocido: %s", msg.Type))
		}
	}
}

func (h *WebSocketHandler) validateMessage(msg *models.Message) error {
	switch msg.Type {
	case "call-request":
		if msg.To == "" {
			return fmt.Errorf("el campo 'to' es obligatorio")
		}
	case "call-accept", "call-reject", "hangup", "ice-candidate", "sdp-offer", "sdp-answer":
		if msg.RoomID == "" {
			return fmt.Errorf("el campo 'roomId' es obligatorio")
		}
	}
	return nil
}

func (h *WebSocketHandler) writePump(client *models.Client) {
	ticker := time.NewTicker(h.Config.WebSocketPingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			if h.TrafficLogger != nil {
				h.TrafficLogger.Log("OUT", client.LogKey(), client.ConnectionID, message)
			}
			client.Conn.SetWriteDeadline(time.Now().Add(h.Config.WriteTimeout))
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(h.Config.WriteTimeout))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
