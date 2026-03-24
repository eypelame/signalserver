package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RoomManager encapsula la lógica de gestión de salas de llamada (Rooms).
type RoomManager struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

// NewRoomManager inicializa un nuevo gestor de salas.
func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*Room),
	}
}

// CreateRoom genera una nueva sala y la registra.
func (rm *RoomManager) CreateRoom(callerUserID, callerRole string, callMaxDuration time.Duration) *Room {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	roomID := uuid.New().String()
	room := NewRoom(roomID, callerUserID, callerRole, callMaxDuration)
	rm.rooms[roomID] = room

	return room
}

// GetRoom obtiene una sala por su ID de manera segura.
func (rm *RoomManager) GetRoom(roomID string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, ok := rm.rooms[roomID]
	return room, ok
}

// DeleteRoom elimina una sala del registro.
func (rm *RoomManager) DeleteRoom(roomID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.rooms, roomID)
}

// IsUserInActiveCall evalúa si un usuario está en una llamada en curso o pendiente.
func (rm *RoomManager) IsUserInActiveCall(userID string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, room := range rm.rooms {
		if (room.CallStatus == "active" || room.CallStatus == "pending") &&
			(room.CallerUserID == userID || room.ListenerUserID == userID) {
			return true
		}
	}
	return false
}

// GetAllRoomsSnapshot devuelve una copia del mapa de salas actual para operaciones que requieran iterar.
func (rm *RoomManager) GetAllRoomsSnapshot() map[string]*Room {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	snapshot := make(map[string]*Room, len(rm.rooms))
	for k, v := range rm.rooms {
		snapshot[k] = v
	}
	return snapshot
}

// FindRoomByUserID busca la primera sala activa o pendiente a la que pertenezca este usuario.
func (rm *RoomManager) FindRoomByUserID(userID string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, room := range rm.rooms {
		if (room.CallStatus == "active" || room.CallStatus == "pending") &&
			(room.CallerUserID == userID || room.ListenerUserID == userID) {
			return room, true
		}
	}
	return nil, false
}

// GetOrCreateRoom obtiene una sala existente o la crea si no existe.
func (rm *RoomManager) GetOrCreateRoom(roomID string, callerUserID, callerRole string, callMaxDuration time.Duration) (*Room, bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if room, exists := rm.rooms[roomID]; exists {
		return room, false
	}

	room := NewRoom(roomID, callerUserID, callerRole, callMaxDuration)
	rm.rooms[roomID] = room
	return room, true
}

// DumpStats retorna estadísticas básicas de las salas.
func (rm *RoomManager) DumpStats() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	active := 0
	pending := 0
	other := 0

	for _, room := range rm.rooms {
		switch room.CallStatus {
		case "active":
			active++
		case "pending":
			pending++
		default:
			other++
		}
	}

	return fmt.Sprintf("Salas Totales: %d (Activas: %d, Pendientes: %d, Otras: %d)", len(rm.rooms), active, pending, other)
}
