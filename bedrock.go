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
	"github.com/hyperfuse/bedrock/cache"
	"github.com/hyperfuse/bedrock/handler"
	"github.com/hyperfuse/bedrock/job"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	pgxUUID "github.com/vgarvardt/pgx-google-uuid/v5"
)

type bedrock struct {
	api             map[string]handler.Controller
	spa             fs.FS
	embedMigrations embed.FS
	dbCreator       func(*pgxpool.Conn) any

	config Configuration
	pool   *pgxpool.Pool
	cache  *cache.Cache
}

type Configuration struct {
	DatabaseUrl string
	Port        int
	Dev         bool
	CachePath   string
}

func NewConfiguration(DatabaseUrl string, port int, dev bool, cachePath string) Configuration {
	return Configuration{
		DatabaseUrl: DatabaseUrl,
		Port:        port,
		Dev:         dev,
		CachePath:   cachePath,
	}
}

func New(config Configuration) (*bedrock, error) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if config.Dev {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		log.Warn().Msg("running in development mode")
	}
	log.Info().Str("db_url", config.DatabaseUrl).Int("Port", config.Port).Bool("dev", config.Dev).Str("cache path", config.CachePath).Msg("Starting server")

	// start the connection pool
	pgxConfig, err := pgxpool.ParseConfig(config.DatabaseUrl)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse the configuration")
		return &bedrock{}, err
	}
	pgxConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxUUID.Register(conn.TypeMap())
		return nil
	}
	dbPool, err := pgxpool.New(context.Background(), config.DatabaseUrl)
	if err != nil {
		log.Fatal().Err(err).Msg("unable to create a pgx pool")
		return &bedrock{}, err
	}

	cache, err := cache.New(config.CachePath)
	if err != nil {
		log.Fatal().Err(err).Msg("unable to create a cache")
		return &bedrock{}, err
	}

	return &bedrock{
		api:    map[string]handler.Controller{},
		config: config,
		pool:   dbPool,
		cache:  cache,
	}, nil
}

func (b *bedrock) Pool() *pgxpool.Pool {
	return b.pool
}

func (b *bedrock) Cache() *cache.Cache {
	return b.cache
}

func (b *bedrock) handlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := b.spa.Open(strings.TrimPrefix(path.Clean(r.URL.Path), "/"))
		if err == nil {
			defer f.Close()
		}
		if os.IsNotExist(err) {
			r.URL.Path = "/"
		}
		http.FileServer(http.FS(b.spa)).ServeHTTP(w, r)
	}
}

func (b *bedrock) DB(creator func(*pgxpool.Conn) any) {
	b.dbCreator = creator
}

var workers = river.NewWorkers()
var periodicJobs = []*river.PeriodicJob{}

func Worker[T river.JobArgs](j job.Worker[T]) {
	river.AddWorker(workers, j)
	switch v := j.(type) {
	case job.PeriodicWorker[river.JobArgs]:
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(15*time.Minute),
			v.GetMessage(),
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

}

func (b *bedrock) Submit(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	return nil
}

func (b *bedrock) Handler(path string, controller handler.Controller) {
	if strings.HasPrefix(path, "/") {
		b.api[path] = controller
		return
	}
	b.api["/"+path] = controller
}

func (b *bedrock) SPAHandler(spa embed.FS) {
	spaFS, err := fs.Sub(spa, "dist")
	if err != nil {
		log.Fatal().Err(err).Msg("failed getting the sub tree for the site files")
	}
	b.spa = spaFS

}

func (b *bedrock) Migrations(fs embed.FS) {
	b.embedMigrations = fs
}

func migrate(dbURL string, migrations fs.FS) error {
	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create a pgx pool: %v\n", err)
		os.Exit(1)
	}

	// Run the river migration
	var migrator = rivermigrate.New(riverpgxv5.New(dbPool), nil)
	_, err = migrator.Migrate(context.Background(), rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
	if err != nil {
		return err
	}

	// Run other migrations
	goose.SetBaseFS(migrations)
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

func (b *bedrock) Run() error {
	// Migrate the database
	if err := migrate(b.config.DatabaseUrl, b.embedMigrations); err != nil {
		log.Error().Err(err).Msg("unable to migrate the database")
		return err
	}
	// Server run context
	// TODO fix warning
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	runner, err := job.NewRiverRunner(serverCtx, b.pool, workers, periodicJobs)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	// r.Use(middleware.RequestID)
	// r.Use(middleware.Recoverer) // TODO add this by default
	// r.Use(middleware.URLFormat)

	if b.config.Dev {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
			AllowCredentials: true,
		}))
	}

	r.Route("/", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			if b.dbCreator != nil {
				r.Use(handler.DBContext(b.pool, b.dbCreator))
			}
			r.Use(job.JobContext(runner))
			for path, c := range b.api {
				r.Mount(path, c.Routes())
			}
		})
	})

	//TODO: should expose this differently. For now if should be enough
	filesDir := http.Dir(b.config.CachePath)
	FileServer(r, "/static", filesDir)

	if b.spa != nil {
		log.Info().Msg("Serving embedded UI.")
		r.Handle("/*", b.handlerFunc())
	}
	server := &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(b.config.Port), Handler: r}

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

		err = runner.Stop(shutdownCtx)
		if err != nil {
			log.Fatal().Err(err)
		}
		b.pool.Close()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal().Err(err)
		}

		serverStopCtx()
		cancel()
	}()
	log.Info().Int("port", b.config.Port).Msg("Server started")
	// Run the server
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err)
	}

	// Wait for server context to be stopped
	<-serverCtx.Done()
	return nil
}

func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
