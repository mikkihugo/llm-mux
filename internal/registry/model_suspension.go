package registry

import (
	"strings"
	"time"

	log "github.com/nghyane/llm-mux/internal/logging"
)

func (r *ModelRegistry) SetModelQuotaExceeded(clientID, modelID string) {
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	newState := r.snapshot().clone()
	reg := newState.findModelRegistration(modelID)
	if reg == nil {
		return
	}
	now := time.Now()
	if reg.QuotaExceededClients == nil {
		reg.QuotaExceededClients = make(map[string]*time.Time)
	}
	reg.QuotaExceededClients[clientID] = &now
	r.state.Store(newState)
	log.Debugf("Marked model %s as quota exceeded for client %s", modelID, clientID)
}

func (r *ModelRegistry) ClearModelQuotaExceeded(clientID, modelID string) {
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	newState := r.snapshot().clone()
	reg := newState.findModelRegistration(modelID)
	if reg == nil {
		return
	}
	if reg.QuotaExceededClients == nil {
		return
	}
	if _, exists := reg.QuotaExceededClients[clientID]; !exists {
		return
	}
	delete(reg.QuotaExceededClients, clientID)
	r.state.Store(newState)
}

func (r *ModelRegistry) SuspendClientModel(clientID, modelID, reason string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	newState := r.snapshot().clone()
	reg := newState.findModelRegistration(modelID)
	if reg == nil {
		return
	}
	if reg.SuspendedClients == nil {
		reg.SuspendedClients = make(map[string]string)
	}
	if _, already := reg.SuspendedClients[clientID]; already {
		return
	}
	reg.SuspendedClients[clientID] = reason
	reg.LastUpdated = time.Now()
	r.state.Store(newState)
	if reason != "" {
		log.Debugf("Suspended client %s for model %s: %s", clientID, modelID, reason)
	} else {
		log.Debugf("Suspended client %s for model %s", clientID, modelID)
	}
}

func (r *ModelRegistry) ResumeClientModel(clientID, modelID string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	newState := r.snapshot().clone()
	reg := newState.findModelRegistration(modelID)
	if reg == nil || reg.SuspendedClients == nil {
		return
	}
	if _, ok := reg.SuspendedClients[clientID]; !ok {
		return
	}
	delete(reg.SuspendedClients, clientID)
	reg.LastUpdated = time.Now()
	r.state.Store(newState)
	log.Debugf("Resumed client %s for model %s", clientID, modelID)
}

func (r *ModelRegistry) ClientSupportsModel(clientID, modelID string) bool {
	clientID = strings.TrimSpace(clientID)
	modelID = strings.TrimSpace(modelID)
	if clientID == "" || modelID == "" {
		return false
	}

	normalizer := NewModelIDNormalizer()
	cleanModelID := normalizer.NormalizeModelID(modelID)

	s := r.snapshot()

	models, exists := s.clientModels[clientID]
	if !exists || len(models) == 0 {
		return false
	}

	for _, id := range models {
		if strings.EqualFold(strings.TrimSpace(id), cleanModelID) {
			return true
		}
	}

	return false
}

func (r *ModelRegistry) CleanupExpiredQuotas() {
	r.writerMu.Lock()
	defer r.writerMu.Unlock()

	now := time.Now()
	quotaExpiredDuration := 5 * time.Minute

	newState := r.snapshot().clone()
	modified := false
	for modelID, registration := range newState.models {
		for clientID, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) >= quotaExpiredDuration {
				delete(registration.QuotaExceededClients, clientID)
				log.Debugf("Cleaned up expired quota tracking for model %s, client %s", modelID, clientID)
				modified = true
			}
		}
	}
	if modified {
		r.state.Store(newState)
	}
}
