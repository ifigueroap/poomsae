package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/ifigueroap/poomsae/internal/activity"
	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/issuesync"
	"github.com/ifigueroap/poomsae/internal/tickticksync"
)

var newTickTickSyncDaemonRunner = func(c tickticksync.RunnerConfig) issueSyncDaemonRunner { return tickticksync.NewRunner(c) }

func newConfiguredTickTickSyncFetcher(c config.TickTickSyncConfig) tickticksync.Fetcher {
	return tickticksync.NewClient(tickticksync.ClientConfig{TokenEnv: c.TokenEnv})
}
func startTickTickSyncRunner(
	ctx context.Context,
	workers *daemonWorkerGroup,
	drainAdmission activity.WaitableAdmission,
	store db.Storage,
	fetcher tickticksync.Fetcher,
	publisher daemon.EventPublisher,
	daemonLog *log.Logger,
	progress *issuesync.ProgressTracker,
) func() {
	if fetcher == nil {
		fetcher = newConfiguredTickTickSyncFetcher(config.TickTickSyncConfig{})
	}
	return startIssueSyncRunner(ctx, workers, daemonLog, "ticktick", func(wake <-chan struct{}, logger *slog.Logger) issueSyncDaemonRunner {
		return newTickTickSyncDaemonRunner(tickticksync.RunnerConfig{
			Store:          store,
			Fetcher:        fetcher,
			Progress:       progress,
			Logger:         logger,
			Interval:       30 * time.Second,
			Wake:           wake,
			DrainAdmission: drainAdmission,
			EventSinkFrom: func(_ context.Context, projectID int64, events []db.Event, fork activity.Admission) error {
				publisher.EventsFrom(projectID, events, fork)
				return nil
			},
		})
	})
}
