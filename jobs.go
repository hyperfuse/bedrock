package bedrock

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PeriodicJobWrapper interface {
	GetWorker() river.Worker[river.JobArgs]
	// TODO change name
	GetJobDetails() func() (river.JobArgs, *river.InsertOpts)
}

type CustomErrorHandler struct{}

func (*CustomErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	zerolog.Ctx(ctx).Error().Err(err).Msg("Job failed")
	return nil
}

func (*CustomErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, v string) *river.ErrorHandlerResult {
	zerolog.Ctx(ctx).Error().Any("panic_val", panicVal).Msg("Job failed")
	return nil
}

type JobRunner interface {
	Submit(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

type RiverRunner struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
}

func NewRiverRunner(ctx context.Context, pool *pgxpool.Pool, pjs []PeriodicJobWrapper) (*RiverRunner, error) {
	// TODO maybe we should consider starting the runner in a different function
	if len(pjs) != 0 {
		return &RiverRunner{pool, nil}, nil
	}
	workers := river.NewWorkers()
	periodicJobs := []*river.PeriodicJob{}
	for _, j := range pjs {
		river.AddWorker(workers, j.GetWorker())
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(15*time.Minute),
			j.GetJobDetails(),
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			"default": {MaxWorkers: 1},
		},
		ErrorHandler: &CustomErrorHandler{},
		Workers:      workers,
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		return nil, err
	}

	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}

	return &RiverRunner{
		pool:        pool,
		riverClient: riverClient,
	}, nil
}

func (r *RiverRunner) Stop(ctx context.Context) error {
	if r.riverClient != nil {
		return r.riverClient.Stop(ctx)
	}
	return nil

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

	_, err = r.riverClient.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}
