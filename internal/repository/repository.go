// Package repository contains one concrete data-access type per backend:
// PostgresRepository (relational source of truth), RedisRepository
// (leaderboards + popularity), MongoRepository (olympic records) and
// Neo4jRepository (graph). The service layer declares the interfaces it
// needs; these types satisfy them implicitly.
package repository
