//go:build integration
// +build integration

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestDBContext(t *testing.T) {
	dbPool, err := pgxpool.New(context.TODO(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal().Err(err).Msg("Unable to create a pgx pool")
	}
	r := chi.NewRouter()
	r.Use(DBContext(dbPool))
	r.Get("/hi", func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(databaseKey).(*db.Queries)
		assert.NotNil(t, q)

		w.Header().Set("X-Test", "yes")
		w.Write([]byte("bye"))
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/hi", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)

	}
	stats := dbPool.Stat()
	assert.Equal(t, int64(1), stats.AcquireCount())
}
