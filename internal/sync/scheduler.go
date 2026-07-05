package sync

import (
	"log"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/models"
)

type Scheduler struct {
	syncer   *Syncer
	datasets []string
	loc      *time.Location
	hour     int
	minute   int
	stopCh   chan struct{}
}

func NewScheduler(syncer *Syncer, datasets []string, loc *time.Location, hour, minute int) *Scheduler {
	return &Scheduler{
		syncer:   syncer,
		datasets: datasets,
		loc:      loc,
		hour:     hour,
		minute:   minute,
		stopCh:   make(chan struct{}),
	}
}

func nextScheduledTime(now time.Time, loc *time.Location, hour, minute int) time.Time {
	local := now.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func (sch *Scheduler) Start() {
	go func() {
		log.Printf("scheduler started: daily sync at %02d:%02d %s", sch.hour, sch.minute, sch.loc)

		for {
			next := nextScheduledTime(time.Now(), sch.loc, sch.hour, sch.minute)
			wait := time.Until(next)
			log.Printf("scheduler: next sync at %s (in %s)", next.Format(time.RFC3339), wait.Round(time.Second))

			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
				timer.Stop()
				if sch.syncer.AnyRunning() {
					log.Printf("scheduler: skipping scheduled sync, import in progress")
					continue
				}
				log.Printf("scheduler: starting scheduled sync for %d datasets", len(sch.datasets))
				sch.syncer.SyncAllAsync(sch.datasets, models.SyncTriggerScheduled)
			case <-sch.stopCh:
				timer.Stop()
				log.Printf("scheduler stopped")
				return
			}
		}
	}()
}

func (sch *Scheduler) Stop() {
	close(sch.stopCh)
}
