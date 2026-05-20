// Package handler contiene los handlers HTTP del binario `server`.
// Declara la interfaz `Service` (lo que necesita de la capa inferior)
// y recibe una implementación por constructor.
package handler

import "net/http"

// Service es la interfaz que el handler consume.
// Los métodos se agregan a medida que se implementan los endpoints.
// La implementación concreta vive en el paquete `service`.
type Service interface{}

type Handler struct {
	svc Service
}

func New(svc Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
