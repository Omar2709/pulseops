package events

import (
	"errors"
	"strings"
)

type Event struct {
	ID      string
	Type    string
	Source  string
	Version int
}

func NewEvent(id string, eventType string, source string, version int) (Event, error) {
	id = strings.TrimSpace(id)
	eventType = strings.TrimSpace(eventType)
	source = strings.TrimSpace(source)

	if id == "" {
		return Event{}, errors.New("event ID is required")
	}

	if eventType == "" {
		return Event{}, errors.New("event type is required")
	}

	if source == "" {
		return Event{}, errors.New("event source is required")
	}

	if version <= 0 {
		return Event{}, errors.New("event version must be greater than zero")
	}

	event := Event{
		ID:      id,
		Type:    eventType,
		Source:  source,
		Version: version,
	}

	return event, nil
}