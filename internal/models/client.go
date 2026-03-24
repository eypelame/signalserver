package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// UserConnections envuelve un mapa de clientes de un usuario con su propio mutex.
type UserConnections struct {
	Connections map[string]*Client
	Mu          sync.RWMutex
}

// Client representa un cliente conectado al servidor de señalización.

type Client struct {
	UserID       string          // ID del usuario (del claim 'sub' del JWT)
	UserName     string          // Nombre del usuario (del claim 'userName' del JWT)
	ConnectionID string          // ID único de la conexión WebSocket
	Conn         *websocket.Conn // Conexión WebSocket subyacente
	Send         chan []byte     // Canal para enviar mensajes al cliente
	IsAvailable  bool            // Estado de disponibilidad del cliente
	PushToken    string          // Token FCM para notificaciones push (solo para listeners)
	CallTime     time.Duration   // Duración máxima de llamada permitida para este caller (del claim 'call_time' del JWT)
	RateLimiter  *rate.Limiter   // Limitador de tasa para mensajes del cliente
	ClientType   string          // Tipo de cliente: "web" o "mobile"
	UserRole     string          // "cliente" o "escucha", extraído del JWT
}

// NewClient crea una nueva instancia de Client.
func NewClient(userID, userName, userRole, connectionID string, conn *websocket.Conn, callTime time.Duration, limiter *rate.Limiter, clientType, fcmToken string) *Client {
	return &Client{
		UserID:       userID,
		UserName:     userName,
		UserRole:     userRole,
		ConnectionID: connectionID,
		Conn:         conn,
		Send:         make(chan []byte, 256),
		IsAvailable:  true, // Por defecto, un cliente recién conectado está disponible
		PushToken:    fcmToken,
		CallTime:     callTime,
		RateLimiter:  limiter,
		ClientType:   clientType,
	}
}

// LogKey retorna la clave compuesta `Role_UserID` para identificar al usuario en los logs
func (c *Client) LogKey() string {
	return c.UserRole + "_" + c.UserID
}
