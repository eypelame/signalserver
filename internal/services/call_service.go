// services/call_service.go
package services

import (
	"fmt"
	"sync"
	"time"

	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/logger"
	"signalserver/internal/metrics"
	"signalserver/internal/models"

	"github.com/google/uuid"
)

// CallService orquestra la lógica de negocio de las llamadas WebRTC.
type CallService struct {
	Config          *config.AppConfig
	PresenceService *PresenceService
	PushService     *PushService
	Backend         *backend.Connector
	RoomManager     *models.RoomManager
	initiateMu      sync.Mutex // Garantiza atomicidad en InitiateCall
}

// NewCallService crea una nueva instancia de CallService.
func NewCallService(cfg *config.AppConfig, presence *PresenceService, push *PushService, backend *backend.Connector, roomManager *models.RoomManager) *CallService {
	return &CallService{
		Config:          cfg,
		PresenceService: presence,
		PushService:     push,
		Backend:         backend,
		RoomManager:     roomManager,
	}
}

// InitiateCall inicia el proceso de una llamada de forma atómica.
// Solo un caller puede iniciar una llamada a la vez contra el mismo target.
func (s *CallService) InitiateCall(caller *models.Client, targetID string) (string, error) {
	s.initiateMu.Lock()
	defer s.initiateMu.Unlock()

	metrics.CallRequestsTotal.Inc()
	targetRole := "escucha"

	// 1. Verificar disponibilidad del objetivo (atómico con el resto de la operación)
	if !s.PresenceService.IsUserAvailable(targetID, targetRole) {
		return "", fmt.Errorf("el usuario no está disponible")
	}

	// 2. Verificar si el objetivo ya está en otra sala
	for _, room := range s.RoomManager.GetAllRoomsSnapshot() {
		status := room.GetStatus()
		if (status == "active" || status == "pending") && (room.CallerUserID == targetID || room.ListenerUserID == targetID) {
			return "", fmt.Errorf("el usuario ya está en una llamada")
		}
	}

	// 3. Verificar saldo del cliente
	if caller.UserRole == "cliente" && caller.CallTime <= 0 {
		return "", fmt.Errorf("fondos insuficientes")
	}

	// 4. Calcular duración máxima
	callMaxDuration := caller.CallTime
	configMax := time.Duration(s.Config.CallMaxDurationMinutes) * time.Minute
	if callMaxDuration <= 0 || callMaxDuration > configMax {
		callMaxDuration = configMax
	}

	// 5. Crear la sala
	roomID := uuid.New().String()
	room, _ := s.RoomManager.GetOrCreateRoom(roomID, caller.UserID, caller.UserRole, callMaxDuration)
	room.AddClient(caller)
	room.ListenerUserID = targetID
	room.ListenerRole = targetRole

	// 6. Marcar indisponibilidad temporal
	s.PresenceService.SetUserAvailability(caller.UserID, caller.UserRole, false)
	s.PresenceService.SetUserAvailability(targetID, targetRole, false)

	// 7. Actualizar estado en Backend (Busy)
	go s.Backend.UpdateCallStatus(caller.UserID, 2, caller.UserRole)
	go s.Backend.UpdateCallStatus(targetID, 2, targetRole)

	return roomID, nil
}

// AcceptCall gestiona la aceptación de una llamada por el listener.
func (s *CallService) AcceptCall(listener *models.Client, roomID string) (*models.Room, error) {
	metrics.CallAcceptsTotal.Inc()

	room, ok := s.RoomManager.GetRoom(roomID)
	if !ok {
		return nil, fmt.Errorf("la sala no existe")
	}

	if room.GetStatus() != "pending" {
		return nil, fmt.Errorf("la llamada no está en estado pendiente")
	}
	if room.ListenerUserID != listener.UserID {
		return nil, fmt.Errorf("no autorizado para aceptar esta llamada")
	}

	room.AddClient(listener)
	room.Accept()

	metrics.ActiveCalls.WithLabelValues(room.CallerRole).Inc()
	metrics.ActiveCalls.WithLabelValues(room.ListenerRole).Inc()

	go s.Backend.UpdateCallStatus(room.ListenerUserID, 3, "escucha")
	go s.Backend.UpdateCallStatus(room.CallerUserID, 3, "cliente")

	if room.CallRequestCancelFunc != nil {
		room.CallRequestCancelFunc()
		room.CallRequestCancelFunc = nil
	}

	return room, nil
}

// RejectCall gestiona el rechazo de una llamada.
func (s *CallService) RejectCall(roomID, reason string) (*models.Room, error) {
	metrics.CallRejectsTotal.Inc()

	room, ok := s.RoomManager.GetRoom(roomID)
	if !ok {
		return nil, fmt.Errorf("la sala no existe")
	}

	if room.GetStatus() != "pending" {
		return nil, fmt.Errorf("la llamada no está en estado pendiente")
	}

	room.Reject(reason)
	s.finalizeCallState(room)

	return room, nil
}

// Hangup gestiona la finalización de una llamada activa o pendiente.
func (s *CallService) Hangup(actorID, actorRole, roomID, eventType string) (*models.Room, error) {
	room, ok := s.RoomManager.GetRoom(roomID)
	if !ok {
		return nil, fmt.Errorf("la sala no existe")
	}

	status := room.GetStatus()

	// Determinar quién colgó y por qué razón
	reason := actorRole + "_" + actorID
	if eventType == "network_error" {
		reason = "network_error_" + reason
	} else if eventType == "hangup" {
		reason = "hang_up_" + reason
	} else if eventType == "timeout" {
		reason = "timeout_" + reason
	}

	room.Complete(reason)

	if status == "active" {
		s.FinalizeBilling(room, reason)
		metrics.ActiveCalls.WithLabelValues(room.CallerRole).Dec()
		metrics.ActiveCalls.WithLabelValues(room.ListenerRole).Dec()
	}

	s.finalizeCallState(room)

	return room, nil
}

// FinalizeBilling procesa la facturación al finalizar una llamada activa.
// Usa MarkBillingFinalized para garantizar facturación única.
func (s *CallService) FinalizeBilling(room *models.Room, reason string) {
	if !room.MarkBillingFinalized() {
		logger.Log.Warnf("[BILLING] Facturación ya realizada para sala %s, ignorando duplicado.", room.ID)
		return
	}

	if room.CallStartTime.IsZero() {
		logger.Log.Warnf("[BILLING] CallStartTime es zero para sala %s, no se puede facturar.", room.ID)
		return
	}

	endTime := time.Now()
	duration := int(endTime.Sub(room.CallStartTime).Seconds())

	metrics.CallDurationHistogram.Observe(float64(duration))

	go s.Backend.ProcessCall(room.ID, room.CallerUserID, room.ListenerUserID, room.CallStartTime, endTime, duration, reason, room.ConnType)
}

// finalizeCallState limpia los estados de disponibilidad y notifica al backend tras finalizar.
func (s *CallService) finalizeCallState(room *models.Room) {
	// Cancelar el timer de duración máxima si existe
	if room.TimerCancel != nil {
		room.TimerCancel()
	}

	go s.Backend.UpdateCallStatus(room.CallerUserID, 1, room.CallerRole)
	go s.Backend.UpdateCallStatus(room.ListenerUserID, 1, room.ListenerRole)

	if room.CallRequestCancelFunc != nil {
		room.CallRequestCancelFunc()
		room.CallRequestCancelFunc = nil
	}

	s.PresenceService.SetUserAvailability(room.CallerUserID, room.CallerRole, true)
	s.PresenceService.SetUserAvailability(room.ListenerUserID, room.ListenerRole, true)

	s.RoomManager.DeleteRoom(room.ID)
}
