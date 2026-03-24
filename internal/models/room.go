package models

import (
	"context"
	"sync"
	"time"
)

// Room representa una sala de llamada donde los clientes intercambian señalización.
type Room struct {
	ID                    string
	Clients               map[string]*Client   // Map de ConnectionID a Client
	CallerUserID          string               // UserID del cliente que inició la llamada
	CallerRole            string               // Rol del cliente que inició la llamada ("cliente")
	ListenerUserID        string               // UserID del cliente que recibe la llamada
	ListenerRole          string               // Rol del cliente que recibe la llamada ("escucha")
	CallMaxDuration       time.Duration        // Duración máxima permitida para la llamada en esta sala
	CallStartTime         time.Time            // Marca de tiempo del inicio de la llamada
	CallEndTime           time.Time            // Marca de tiempo del fin de la llamada
	CallStatus            string               // Estado actual de la llamada (e.g., "pending", "active", "completed", "rejected")
	ReasonForEnd          string               // Razón por la que terminó la llamada
	BillingProcessed      bool                 // Flag para asegurar que la llamada solo se facture una vez
	CallRequestCancelFunc context.CancelFunc   // Función para cancelar el temporizador de solicitud de llamada
	SignalingBuffer       map[string][]Message // Mensajes de señalización retenidos por UserID
	ConnType              int                  // Tipo de conexión (1: Host, 2: Srflx, 3: Relay)
	mu                    sync.RWMutex         // Mutex para proteger el acceso concurrente a los campos de la sala
}

// NewRoom crea una nueva instancia de Room.
func NewRoom(id, callerUserID, callerRole string, callMaxDuration time.Duration) *Room {
	return &Room{
		ID:              id,
		Clients:         make(map[string]*Client),
		CallerUserID:    callerUserID,
		CallerRole:      callerRole,
		CallMaxDuration: callMaxDuration,
		CallStatus:      "pending", // Estado inicial de la llamada
		SignalingBuffer: make(map[string][]Message),
	}
}

// AddClient añade un cliente a la sala.
func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Clients[client.ConnectionID] = client
}

// RemoveClient elimina un cliente de la sala.
func (r *Room) RemoveClient(connectionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Clients, connectionID)
}

// GetClient obtiene un cliente por su ConnectionID.
func (r *Room) GetClient(connectionID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.Clients[connectionID]
	return client, ok
}

// SetConnType actualiza el tipo de conexión de forma thread-safe.
func (r *Room) SetConnType(connType int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ConnType = connType
}

// GetOtherClient obtiene el otro cliente en una llamada 1 a 1.
func (r *Room) GetOtherClient(currentConnectionID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, client := range r.Clients {
		if client.ConnectionID != currentConnectionID {
			return client, true
		}
	}
	return nil, false
}

// RemoveClientByUserID elimina un cliente de la sala por su UserID.
func (r *Room) RemoveClientByUserID(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for connID, client := range r.Clients {
		if client.UserID == userID {
			delete(r.Clients, connID)
			return
		}
	}
}

// Accept marca la sala como activa y registra la hora de inicio.
func (r *Room) Accept() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CallStatus = "active"
	r.CallStartTime = time.Now()
}

// Reject marca la sala como rechazada con la razón dada.
func (r *Room) Reject(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CallStatus = "rejected"
	r.CallEndTime = time.Now()
	r.ReasonForEnd = reason
}

// Complete marca la sala como finalizada de manera normal (colgo un participante).
func (r *Room) Complete(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CallStatus = "completed"
	r.CallEndTime = time.Now()
	r.ReasonForEnd = reason
}

// Timeout marca la sala por limite de tiempo o falta de respuesta.
func (r *Room) Timeout(reason string, isPending bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if isPending {
		r.CallStatus = "no-answer"
	} else {
		r.CallStatus = "timeout"
	}
	r.CallEndTime = time.Now()
	r.ReasonForEnd = reason
}

// GetStatus retorna el estado actual de forma segura.
func (r *Room) GetStatus() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.CallStatus
}

// IsTimedOut verifica si la llamada ha excedido su duración máxima.
func (r *Room) IsTimedOut(now time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.CallStatus == "active" && !r.CallStartTime.IsZero() {
		return now.After(r.CallStartTime.Add(r.CallMaxDuration))
	}
	// Las llamadas pendientes se manejan por un temporizador específico (startCallRequestTimer),
	// pero este método puede usarse como respaldo en el cleanup general.
	return false
}

// BufferMessage almacena un mensaje para un usuario que aún no tiene conexión activa.
func (r *Room) BufferMessage(userID string, msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.SignalingBuffer[userID] = append(r.SignalingBuffer[userID], msg)
}

// FlushBuffer recupera y elimina los mensajes almacenados para un usuario.
func (r *Room) FlushBuffer(userID string) []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	msgs := r.SignalingBuffer[userID]
	delete(r.SignalingBuffer, userID)
	return msgs
}
