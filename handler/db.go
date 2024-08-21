package handler

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	DatabaseKey contextKey = "database"
)

// TODO(guillaumebreton, 2024-08-13): it would be better to return something else than "any", and use the type
// system to ensure we have a connection
func DBContext(pool *pgxpool.Pool, dbCreactor func(*pgxpool.Conn) any) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := pool.Acquire(r.Context())
			log.Debug().Msg("Acquired connection")
			if err != nil {
				write(w, http.StatusInternalServerError, Error{Message: err.Error()})
				return
			}
			q := dbCreactor(conn)
			ctx := context.WithValue(r.Context(), DatabaseKey, q)
			next.ServeHTTP(w, r.WithContext(ctx))
			log.Debug().Msg("Releasing connection")
			conn.Release()
		})
	}
}
