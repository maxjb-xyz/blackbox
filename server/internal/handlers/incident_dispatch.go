package handlers

import (
	"log"
	"sync"
	"time"

	"blackbox/shared/types"
)

const (
	incidentDispatchTimeout = 30 * time.Second
	maxPendingIncidentSends = 64
)

var incidentDispatchSem = make(chan struct{}, maxPendingIncidentSends)
var incidentDispatchWG sync.WaitGroup

func dispatchToIncidentChannelWithShutdown(ch chan<- types.Entry, shutdown <-chan struct{}, entry types.Entry) {
	if ch == nil {
		log.Printf("incidents: skipping incident dispatch for entry %s (service %s): channel is nil", entry.ID, entry.Service)
		return
	}

	select {
	case incidentDispatchSem <- struct{}{}:
	default:
		log.Printf(
			"incidents: dispatch queue saturated, skipping entry %s (service %s depth=%d cap=%d)",
			entry.ID,
			entry.Service,
			len(ch),
			cap(ch),
		)
		return
	}

	incidentDispatchWG.Add(1)
	go func(e types.Entry) {
		defer incidentDispatchWG.Done()
		defer func() { <-incidentDispatchSem }()

		timer := time.NewTimer(incidentDispatchTimeout)
		defer timer.Stop()

		select {
		case ch <- e:
		case <-timer.C:
			log.Printf(
				"incidents: channel stalled >30s, skipping incident processing for entry %s (service %s depth=%d cap=%d)",
				e.ID,
				e.Service,
				len(ch),
				cap(ch),
			)
		case <-shutdown:
		}
	}(entry)
}

func WaitForIncidentDispatches(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		incidentDispatchWG.Wait()
		close(done)
	}()

	if timeout <= 0 {
		<-done
		return true
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
