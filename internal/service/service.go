// Package service contiene la lógica de negocio.
// Declara la interfaz `Repository` (lo que necesita de la capa
// inferior) y recibe una implementación por constructor.
package service

// Repository es la interfaz que el service consume.
// Los métodos se agregan a medida que se implementan los casos de uso.
// La implementación concreta vive en el paquete `repository`.
type Repository interface{}

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}
