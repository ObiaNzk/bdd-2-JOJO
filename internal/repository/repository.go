// Package repository contiene la implementación concreta de acceso a datos.
// Declara las interfaces de los clientes de BD que consume y recibe
// implementaciones por constructor.
package repository

// Postgres, Mongo, Redis y Neo4j son las interfaces que el repository
// consume de la capa de platform. Los métodos se agregan a medida que
// se implementan las queries; las implementaciones concretas las
// satisfacen los clientes creados en `internal/platform`.
type (
	Postgres interface{}
	Mongo    interface{}
	Redis    interface{}
	Neo4j    interface{}
)

type Repository struct {
	pg    Postgres
	mongo Mongo
	redis Redis
	neo4j Neo4j
}

func New(pg Postgres, mongo Mongo, rdb Redis, neo Neo4j) *Repository {
	return &Repository{
		pg:    pg,
		mongo: mongo,
		redis: rdb,
		neo4j: neo,
	}
}
