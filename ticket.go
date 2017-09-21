package main

import (
	"fmt"
	"log"

	"github.com/bytemine/go-icinga2/event"
	"github.com/bytemine/icinga2rt/rt"
)

type permitFunc func(e *event.Notification) bool

func newPermitFunc(permit []event.State) permitFunc {
	return func(e *event.Notification) bool {
		for _, x := range permit {
			if e.CheckResult.State == x {
				return true
			}
		}
		return false
	}
}

type eventTicket struct {
	Event    *event.Notification
	TicketID int
}

type ticketUpdater struct {
	cache    *cache
	rtClient *rt.Client
	pf       permitFunc
	nobody   string
	queue    string
}

func formatEventSubject(e *event.Notification) string {
	switch {
	case e.Host != "" && e.Service != "":
		return fmt.Sprintf("Host: %v Service: %v is %v", e.Host, e.Service, e.CheckResult.State.String())
	case e.Host != "" && e.Service == "":
		return fmt.Sprintf("Host: %v is %v", e.Host, e.CheckResult.State.String())
	default:
		return fmt.Sprintf("Host: %v Service: %v is %v", e.Host, e.Service, e.CheckResult.State.String())
	}
}

func formatEventComment(e *event.Notification) string {
	if e.CheckResult.Output != "" {
		return fmt.Sprintf("New status: %v Output: %v", e.CheckResult.State.String(), e.CheckResult.Output)
	}

	return e.CheckResult.State.String()
}

func newTicketUpdater(cache *cache, rtClient *rt.Client, pf permitFunc, nobody string, queue string) *ticketUpdater {
	return &ticketUpdater{cache: cache, rtClient: rtClient, pf: pf, nobody: nobody, queue: queue}
}

func (t *ticketUpdater) updateTicket(e *event.Notification) error {
	et, err := t.cache.getEventTicket(e)
	if err != nil {
		return err
	}

	oldState := event.StateUnknown
	if et != nil {
		oldState = et.Event.CheckResult.State
	}

	if *debug {
		log.Printf("ticket updater: incoming event %v/%v %v (was %v)\n", e.Host, e.Service, e.CheckResult.State.String(), oldState.String())
	}

	if !t.pf(e) {
		log.Printf("ticket updater: ignoring event for %v/%v %v due to permit function.", e.Host, e.Service, e.CheckResult.State.String())
		return nil
	}

	// no ticket existing for this host/service combination
	if et == nil {
		return t.newEvent(e)
	}

	// ticket existing for this host/service combination
	return t.oldEvent(e, et.Event, et.TicketID)
}

func (t *ticketUpdater) newEvent(e *event.Notification) error {
	// don't create a new ticket if state is OK
	if e.CheckResult.State == event.StateOK {
		return nil
	}

	// create ticket
	return t.createTicket(e)
}

func (t *ticketUpdater) oldEvent(newEvent *event.Notification, oldEvent *event.Notification, ticketID int) error {
	// if old ticket isn't existing anymore, create a new ticket.
	oldTicket, err := t.rtClient.Ticket(ticketID)
	if err != nil {
		log.Printf("ticket updater: %v/%v: old ticket %v doesn't exist anymore", newEvent.Host, newEvent.Service, ticketID)
		return t.createTicket(newEvent)
	}

	// if old ticket has status "deleted", create a new one to prevent reopening tickets.
	// we don't need to delete the cache entry as it will be overwritten in createTicket.
	if oldTicket.Status == "deleted" {
		log.Printf("ticket updater: %v/%v: not reusing ticket %v with status %s", newEvent.Host, newEvent.Service, oldTicket.ID, oldTicket.Status)
		return t.createTicket(newEvent)
	}

	switch newEvent.CheckResult.State {
	case event.StateOK:
		// new state is OK and old ticket is existing
		return t.deleteOrCommentTicket(newEvent, oldEvent, ticketID)
	default:
		// comment ticket with new state.
		// don't update if the state hasn't changed.
		if newEvent.CheckResult.State != oldEvent.CheckResult.State {
			return t.commentTicket(newEvent, oldEvent, ticketID)
		}
		return nil
	}
}

func (t *ticketUpdater) createTicket(e *event.Notification) error {
	ticket := &rt.Ticket{Queue: t.queue, Subject: formatEventSubject(e), Text: fmt.Sprintf("Output: %s", e.CheckResult.Output)}

	newTicket, err := t.rtClient.NewTicket(ticket)
	if err != nil {
		return err
	}

	if *debug {
		log.Printf("ticket updater: %v/%v: created ticket #%v", e.Host, e.Service, newTicket.ID)
	}

	err = t.cache.updateEventTicket(e, newTicket.ID)
	if err != nil {
		return err
	}

	return nil
}

func (t *ticketUpdater) deleteOrCommentTicket(newEvent, oldEvent *event.Notification, ticketID int) error {
	oldTicket, err := t.rtClient.Ticket(ticketID)
	if err != nil {
		return err
	}

	switch oldTicket.Owner {
	case t.nobody:
		// nobody owns this ticket, delete it
		return t.deleteTicket(newEvent, oldEvent, ticketID)
	default:
		// ticket is owned, comment and forget about this ticket.
		err := t.commentTicket(newEvent, oldEvent, ticketID)
		if err != nil {
			return err
		}
		return t.cache.deleteEventTicket(newEvent)
	}
}

func (t *ticketUpdater) commentTicket(newEvent, oldEvent *event.Notification, ticketID int) error {
	// Comment existing ticket with new status.
	err := t.rtClient.CommentTicket(ticketID, formatEventComment(newEvent))
	if err != nil {
		return err
	}

	if *debug {
		log.Printf("ticket updater: %v/%v: commented ticket #%v", newEvent.Host, newEvent.Service, ticketID)
	}

	err = t.cache.updateEventTicket(newEvent, ticketID)
	if err != nil {
		return err
	}

	return nil
}

func (t *ticketUpdater) deleteTicket(newEvent, oldEvent *event.Notification, ticketID int) error {
	newTicket := &rt.Ticket{ID: ticketID, Status: "deleted"}

	updatedTicket, err := t.rtClient.UpdateTicket(newTicket)
	if err != nil {
		return err
	}

	if *debug {
		log.Printf("ticket updater: %v/%v: deleted ticket #%v", newEvent.Host, newEvent.Service, updatedTicket.ID)
	}

	if err = t.cache.deleteEventTicket(newEvent); err != nil {
		return err
	}

	return nil
}
