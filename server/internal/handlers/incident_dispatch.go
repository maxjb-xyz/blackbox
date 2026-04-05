package handlers

import (
	"log"
	"time"

	"blackbox/shared/types"
)

const (
	incidentDispatchTimeout = 30 * time.Second
	maxPendingIncidentSends = 64
)

var incidentDispatchSem = make(chan struct{}, maxPendingIncidentSends)

func dispatchToIncidentChannel(ch chan<- types.Entry, entry types.Entry) {
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

	go func(e types.Entry) {
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
		}
	}(entry)
}
