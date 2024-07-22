package bedrock

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hyperfuse/bedrock/api"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog/log"
	pgxUUID "github.com/vgarvardt/pgx-google-uuid/v5"
)

var server = &bedrock{
	api: map[string]api.Controller{},
}

type bedrock struct {
	api             map[string]api.Controller
	spa             fs.FS
	embedMigrations embed.FS
}

func (b *bedrock) handlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := server.spa.Open(strings.TrimPrefix(path.Clean(r.URL.Path), "/"))
		if err == nil {
			defer f.Close()
		}
		if os.IsNotExist(err) {
			r.URL.Path = "/"
		}
		http.FileServer(http.FS(server.spa)).ServeHTTP(w, r)
	}
}

func ApiHandler(path string, controller api.Controller) {
	server.api[path] = controller
}

func SPAHandler(spa embed.FS) {
	spaFS, err := fs.Sub(spa, "dist")
	if err != nil {
		log.Fatal().Err(err).Msg("failed getting the sub tree for the site files")
	}
	server.spa = spaFS

}

func Migrations(fs embed.FS) {
	server.embedMigrations = fs
}

func migrate(dbURL string) error {
	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create a pgx pool: %v\n", err)
		os.Exit(1)
	}
	goose.SetBaseFS(server.embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(dbPool)
	if err := goose.Up(db, "sqlc/migrations"); err != nil {
		log.Error().Err(err).Msg("Unable to migrate the database.")
		return err
	}
	return nil
}

func Run(databaseURL string, port int, dev bool) error {

	// Migrate the database
	if err := migrate(databaseURL); err != nil {
		log.Error().Err(err).Msg("unable to migrate the database")
		os.Exit(1)
	}
	// start the connection pool
	pgxConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse the configuration")
		os.Exit(1)
	}
	pgxConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxUUID.Register(conn.TypeMap())
		return nil
	}
	dbPool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("unable to create a pgx pool")
		os.Exit(1)
	}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	// r.Use(middleware.RequestID)
	// r.Use(middleware.Recoverer)
	// r.Use(middleware.URLFormat)

	if dev {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
			AllowCredentials: true,
		}))
	}

	r.Route("/api", func(r chi.Router) {

		r.Group(func(r chi.Router) {
			// TODO Add authentication
			// r.Use(jwtauth.Verifier(controllers.TokenAuth))
			// r.Use(jwtauth.Authenticator(controllers.TokenAuth))
			// r.Use(middleware.UserContext)
			//
			// TODO configure SQLC DB
			// r.Use(middleware.DBContext(dbPool))

			for path, c := range server.api {
				r.Mount(path, c.Routes())
			}
		})

		// r.Group(func(r chi.Router) {
		// 	r.Use(controllers.DBContext(dbPool))
		// 	r.Mount("/auth", controllers.NewAuthController().Routes())
		// })

	})

	if server.spa != nil {
		log.Info().Msg("Serving embedded UI.")
		r.Handle("/*", server.handlerFunc())
	}
	server := &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(port), Handler: r}

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, os.Interrupt)
	go func() {
		<-sig
		log.Info().Msg("Shutting down server..")

		// Shutdown signal with grace period of 30 seconds
		shutdownCtx, cancel := context.WithTimeout(serverCtx, 30*time.Second)
		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal().Msg("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// TODO: stop the jobs here, before the DB pool
		// err = jobs.Stop(shutdownCtx)
		// if err != nil {
		// 	log.Fatal().Err(err)
		// }
		dbPool.Close()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal().Err(err)
		}

		serverStopCtx()
		cancel()
	}()
	log.Info().Int("port", port).Msg("Server started")
	// Run the server
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err)
	}

	// Wait for server context to be stopped
	<-serverCtx.Done()
	return nil
}
