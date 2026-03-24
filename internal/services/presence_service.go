package services

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"signalserver/internal/backend"
	"signalserver/internal/config"
	"signalserver/internal/metrics"
	"signalserver/internal/models"
)

// PresenceService gestiona el estado de conexión y disponibilidad de los usuarios.
type PresenceService struct {
	Config            *config.AppConfig
	Backend           *backend.Connector
	clients           sync.Map // map[string]*models.Client (ConnectionID -> Client)
	userToConnections sync.Map // map[string]*models.UserConnections (Role_UserID -> Connections)
	userAvailability  sync.Map // map[string]bool (Role_UserID -> isAvailable)
	pushTokens        sync.Map // map[string]string (Role_UserID -> token)
	userNamesCache    sync.Map // map[string]string (Role_UserID -> name)
}

// NewPresenceService crea una nueva instancia de PresenceService.
func NewPresenceService(cfg *config.AppConfig, backend *backend.Connector) *PresenceService {
	return &PresenceService{
		Config:  cfg,
		Backend: backend,
	}
}

// Register gestiona el alta de una nueva conexión de un cliente.
func (s *PresenceService) Register(client *models.Client) (int, []*models.Client) {
	clientKey := fmt.Sprintf("%s_%s", client.UserRole, client.UserID)
	s.clients.Store(client.ConnectionID, client)

	actual, _ := s.userToConnections.LoadOrStore(clientKey, &models.UserConnections{
		Connections: make(map[string]*models.Client),
	})
	uc := actual.(*models.UserConnections)

	uc.Mu.Lock()
	uc.Connections[client.ConnectionID] = client
	totalConns := len(uc.Connections)
	uc.Mu.Unlock()

	s.userAvailability.Store(clientKey, true) // Por defecto disponible al conectar (sujeto a lógica de llamada)
	s.userNamesCache.Store(clientKey, client.UserName)

	metrics.ActiveConnections.WithLabelValues(client.UserRole).Inc()
	metrics.AvailableUsers.WithLabelValues(client.UserRole).Inc()

	var clientsToEvict []*models.Client
	if totalConns > s.Config.MaxConnPerUser {
		uc.Mu.RLock()
		for connID, oldClient := range uc.Connections {
			if len(uc.Connections)-len(clientsToEvict) <= s.Config.MaxConnPerUser {
				break
			}
			if connID != client.ConnectionID {
				clientsToEvict = append(clientsToEvict, oldClient)
			}
		}
		uc.Mu.RUnlock()
	}

	return totalConns, clientsToEvict
}

// Unregister gestiona la baja de una conexión.
func (s *PresenceService) Unregister(client *models.Client) (bool, bool) {
	if _, loaded := s.clients.LoadAndDelete(client.ConnectionID); loaded {
		clientKey := fmt.Sprintf("%s_%s", client.UserRole, client.UserID)

		isLastConnection := false
		if val, ok := s.userToConnections.Load(clientKey); ok {
			uc := val.(*models.UserConnections)

			uc.Mu.Lock()
			delete(uc.Connections, client.ConnectionID)
			currentConnectionsForUser := len(uc.Connections)
			uc.Mu.Unlock()

			if currentConnectionsForUser == 0 {
				s.userToConnections.Delete(clientKey)
				isLastConnection = true

				// Especial para "escucha": Si tiene token Push, mantener disponibilidad en memoria
				hasPushToken := false
				if _, ok := s.pushTokens.Load(clientKey); ok {
					hasPushToken = true
				}

				if client.UserRole == "escucha" && hasPushToken {
					// No desactivamos disponibilidad, permitimos que sea contactado vía Push
				} else {
					s.userAvailability.Store(clientKey, false)
					metrics.AvailableUsers.WithLabelValues(client.UserRole).Dec()
				}
			}
		}

		metrics.ActiveConnections.WithLabelValues(client.UserRole).Dec()
		return true, isLastConnection
	}
	return false, false
}

// UpdateAvailability actualiza la disponibilidad manual de un usuario.
func (s *PresenceService) UpdateAvailability(client *models.Client, isAvailable bool) {
	clientKey := fmt.Sprintf("%s_%s", client.UserRole, client.UserID)

	currentAvailable := false
	if v, ok := s.userAvailability.Load(clientKey); ok {
		currentAvailable = v.(bool)
	}

	if currentAvailable != isAvailable {
		if isAvailable {
			metrics.AvailableUsers.WithLabelValues(client.UserRole).Inc()
		} else {
			metrics.AvailableUsers.WithLabelValues(client.UserRole).Dec()
		}
	}
	s.userAvailability.Store(clientKey, isAvailable)
}

// SetUserAvailability fuerza el estado de disponibilidad (ej. por llamada activa).
func (s *PresenceService) SetUserAvailability(userID, role string, isAvailable bool) {
	clientKey := fmt.Sprintf("%s_%s", role, userID)

	current := false
	if v, ok := s.userAvailability.Load(clientKey); ok {
		current = v.(bool)
	}

	if current != isAvailable {
		if isAvailable {
			metrics.AvailableUsers.WithLabelValues(role).Inc()
		} else {
			metrics.AvailableUsers.WithLabelValues(role).Dec()
		}
	}
	s.userAvailability.Store(clientKey, isAvailable)
}

// IsUserAvailable verifica si un usuario está marcado como disponible.
func (s *PresenceService) IsUserAvailable(userID, role string) bool {
	clientKey := fmt.Sprintf("%s_%s", role, userID)
	if v, ok := s.userAvailability.Load(clientKey); ok {
		return v.(bool)
	}
	return false
}

// UpdatePushToken actualiza el token de notificaciones push.
func (s *PresenceService) UpdatePushToken(userID, role, token string) {
	clientKey := fmt.Sprintf("%s_%s", role, userID)
	if token == "" {
		s.pushTokens.Delete(clientKey)
	} else {
		s.pushTokens.Store(clientKey, token)
	}
}

// GetPushToken obtiene el token push de un usuario.
func (s *PresenceService) GetPushToken(userID, role string) (string, bool) {
	clientKey := fmt.Sprintf("%s_%s", role, userID)
	if v, ok := s.pushTokens.Load(clientKey); ok {
		return v.(string), true
	}
	return "", false
}

// GetUserConnections retorna todas las conexiones activas de un usuario.
func (s *PresenceService) GetUserConnections(userID, role string) []*models.Client {
	clientKey := fmt.Sprintf("%s_%s", role, userID)
	if val, ok := s.userToConnections.Load(clientKey); ok {
		uc := val.(*models.UserConnections)

		uc.Mu.RLock()
		defer uc.Mu.RUnlock()

		conns := make([]*models.Client, 0, len(uc.Connections))
		for _, c := range uc.Connections {
			conns = append(conns, c)
		}
		return conns
	}
	return nil
}

// GetClientByID obtiene un cliente por su ID de conexión.
func (s *PresenceService) GetClientByID(connID string) (*models.Client, bool) {
	if v, ok := s.clients.Load(connID); ok {
		return v.(*models.Client), true
	}
	return nil, false
}

// BuildClientsList genera la lista de clientes disponibles para enviar a las apps.
func (s *PresenceService) BuildClientsList() models.ClientsListPayload {
	uniqueClients := make(map[string]models.ClientInfo)

	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*models.Client)
		clientKey := fmt.Sprintf("%s_%s", client.UserRole, client.UserID)
		if _, exists := uniqueClients[clientKey]; !exists {
			available := false
			if v, ok := s.userAvailability.Load(clientKey); ok {
				available = v.(bool)
			}
			uniqueClients[clientKey] = models.ClientInfo{
				UserID:       client.UserID,
				UserName:     client.UserName,
				ConnectionID: client.ConnectionID,
				IsAvailable:  available,
				ClientType:   client.ClientType,
				UserRole:     client.UserRole,
			}
		}
		return true
	})

	s.pushTokens.Range(func(key, value interface{}) bool {
		clientKey := key.(string)
		token := value.(string)
		if token != "" {
			if _, exists := uniqueClients[clientKey]; !exists {
				parts := strings.SplitN(clientKey, "_", 2)
				role := "escucha"
				userID := clientKey
				if len(parts) == 2 {
					role = parts[0]
					userID = parts[1]
				}

				userName := "Usuario (Push)"
				if cachedName, ok := s.userNamesCache.Load(clientKey); ok && cachedName != "" {
					userName = cachedName.(string)
				}

				available := false
				if v, ok := s.userAvailability.Load(clientKey); ok {
					available = v.(bool)
				}

				uniqueClients[clientKey] = models.ClientInfo{
					UserID:      userID,
					UserName:    userName,
					IsAvailable: available,
					ClientType:  "mobile",
					UserRole:    role,
				}
			}
		}
		return true
	})

	var availableListeners []models.ClientInfo
	var otherClients []models.ClientInfo

	for _, info := range uniqueClients {
		if info.UserRole == "escucha" && info.IsAvailable {
			availableListeners = append(availableListeners, info)
		} else {
			otherClients = append(otherClients, info)
		}
	}

	totalAvailable := len(availableListeners)

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(availableListeners), func(i, j int) {
		availableListeners[i], availableListeners[j] = availableListeners[j], availableListeners[i]
	})

	limit := 10
	if len(availableListeners) > limit {
		availableListeners = availableListeners[:limit]
	}

	var clientInfos []models.ClientInfo
	clientInfos = append(clientInfos, availableListeners...)

	otherLimit := 5
	if len(otherClients) > otherLimit {
		otherClients = otherClients[:otherLimit]
	}
	clientInfos = append(clientInfos, otherClients...)

	return models.ClientsListPayload{
		TotalAvailable: totalAvailable,
		Clients:        clientInfos,
	}
}

// GetAllConnections retorna todas las conexiones activas en el servidor.
func (s *PresenceService) GetAllConnections() []*models.Client {
	var all []*models.Client
	s.clients.Range(func(key, value interface{}) bool {
		all = append(all, value.(*models.Client))
		return true
	})
	return all
}
