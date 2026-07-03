package sync

import (
	"log"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/models"
)

type Scheduler struct {
	syncer    *Syncer
	datasets  []string
	interval  time.Duration
	stopCh    chan struct{}
}

func NewScheduler(syncer *Syncer, datasets []string, interval time.Duration) *Scheduler {
	return &Scheduler{
		syncer:   syncer,
		datasets: datasets,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (sch *Scheduler) Start() {
	go func() {
		log.Printf("scheduler started: interval %s", sch.interval)
		ticker := time.NewTicker(sch.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if sch.syncer.AnyRunning() {
					log.Printf("scheduler: skipping scheduled sync, import in progress")
					continue
				}
				log.Printf("scheduler: starting scheduled sync for %d datasets", len(sch.datasets))
				sch.syncer.SyncAllAsync(sch.datasets, models.SyncTriggerScheduled)
			case <-sch.stopCh:
				log.Printf("scheduler stopped")
				return
			}
		}
	}()
}

func (sch *Scheduler) Stop() {
	close(sch.stopCh)
}
