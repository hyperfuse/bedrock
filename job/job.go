package job

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	JobRunnerKey contextKey = "database"
)

func JobContext(runner JobRunner) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), JobRunnerKey, runner)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type Worker[T river.JobArgs] interface {
	river.Worker[T]
}

type PeriodicWorker[T river.JobArgs] interface {
	river.Worker[T]
	GetMessage() func() (river.JobArgs, *river.InsertOpts)
}

type CustomErrorHandler struct{}

func (*CustomErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	zerolog.Ctx(ctx).Error().Err(err).Msg("Job failed")
	return nil
}

func (*CustomErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	zerolog.Ctx(ctx).Error().Any("panic_val", panicVal).Msg("Job failed")
	// TODO only-do this in debug mode
	fmt.Printf("Job panicked with: %v\n", panicVal)
	fmt.Printf("Stack trace: %s\n", trace)
	return nil
}

type JobRunner interface {
	Submit(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

type RiverRunner struct {
	pool *pgxpool.Pool
	*river.Client[pgx.Tx]
}

func NewRiverRunner(ctx context.Context, pool *pgxpool.Pool, workers *river.Workers, jobs []*river.PeriodicJob) (*RiverRunner, error) {
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			"default": {MaxWorkers: 1},
		},
		ErrorHandler: &CustomErrorHandler{},
		Workers:      workers,
		PeriodicJobs: jobs,
	})
	if err != nil {
		return nil, err
	}

	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}

	return &RiverRunner{
		pool,
		riverClient,
	}, nil
}

func (r *RiverRunner) Stop(ctx context.Context) error {
	return r.Stop(ctx)
}

func (r *RiverRunner) Submit(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		err := tx.Rollback(ctx)
		if err != nil && err != pgx.ErrTxClosed {
			log.Error().Err(err).Msg("error rolling back transaction")
		}
	}()

	_, err = r.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}
