// services/presence_service.go
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
	userAvailability  map[string]bool
	availabilityMu    sync.Mutex
	pushTokens        sync.Map // map[string]string (Role_UserID -> token)
	userNamesCache    sync.Map // map[string]string (Role_UserID -> name)
}

// NewPresenceService crea una nueva instancia de PresenceService.
func NewPresenceService(cfg *config.AppConfig, backend *backend.Connector) *PresenceService {
	return &PresenceService{
		Config:           cfg,
		Backend:          backend,
		userAvailability: make(map[string]bool),
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

	s.SetUserAvailability(client.UserID, client.UserRole, true)
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
					s.SetUserAvailability(client.UserID, client.UserRole, false)
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
	s.SetUserAvailability(client.UserID, client.UserRole, isAvailable)
}

// SetUserAvailability fuerza el estado de disponibilidad de forma atómica.
func (s *PresenceService) SetUserAvailability(userID, role string, isAvailable bool) {
	clientKey := fmt.Sprintf("%s_%s", role, userID)

	s.availabilityMu.Lock()
	defer s.availabilityMu.Unlock()

	current := s.userAvailability[clientKey]

	if current != isAvailable {
		if isAvailable {
			metrics.AvailableUsers.WithLabelValues(role).Inc()
		} else {
			metrics.AvailableUsers.WithLabelValues(role).Dec()
		}
	}
	s.userAvailability[clientKey] = isAvailable
}

// IsUserAvailable verifica si un usuario está marcado como disponible.
func (s *PresenceService) IsUserAvailable(userID, role string) bool {
	clientKey := fmt.Sprintf("%s_%s", role, userID)

	s.availabilityMu.Lock()
	defer s.availabilityMu.Unlock()

	return s.userAvailability[clientKey]
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
// Usa un snapshot de disponibilidad para garantizar consistencia durante la iteración.
func (s *PresenceService) BuildClientsList() models.ClientsListPayload {
	// 1. Congelar el estado de disponibilidad primero
	s.availabilityMu.Lock()
	availSnapshot := make(map[string]bool, len(s.userAvailability))
	for k, v := range s.userAvailability {
		availSnapshot[k] = v
	}
	s.availabilityMu.Unlock()

	// 2. Snapshot de push tokens (menos crítico pero mantiene consistencia)
	pushSnapshot := make(map[string]string)
	s.pushTokens.Range(func(key, value interface{}) bool {
		pushSnapshot[key.(string)] = value.(string)
		return true
	})

	// 3. Snapshot de nombres cacheados
	namesSnapshot := make(map[string]string)
	s.userNamesCache.Range(func(key, value interface{}) bool {
		namesSnapshot[key.(string)] = value.(string)
		return true
	})

	// 4. Construir la lista con los snapshots
	uniqueClients := make(map[string]models.ClientInfo)

	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*models.Client)
		clientKey := fmt.Sprintf("%s_%s", client.UserRole, client.UserID)
		if _, exists := uniqueClients[clientKey]; !exists {
			available := availSnapshot[clientKey] // usa snapshot, no sync.Map
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

	// 5. Agregar usuarios que solo tienen push token (sin conexión activa)
	for clientKey, token := range pushSnapshot {
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
				if cachedName, ok := namesSnapshot[clientKey]; ok && cachedName != "" {
					userName = cachedName
				}

				available := availSnapshot[clientKey] // usa snapshot

				uniqueClients[clientKey] = models.ClientInfo{
					UserID:      userID,
					UserName:    userName,
					IsAvailable: available,
					ClientType:  "mobile",
					UserRole:    role,
				}
			}
		}
	}

	// 6. Separar disponibles de no disponibles
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

	// 7. Aleatorizar y limitar
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
