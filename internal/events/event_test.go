package events

import "testing"

func TestNewEvent(t *testing.T) {
	event, err := NewEvent(
		"evt_001",
		"order.completed",
		"orders-service",
		1,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if event.ID != "evt_001" {
		t.Errorf("expected ID evt_001, got %s", event.ID)
	}

	if event.Type != "order.completed" {
		t.Errorf("expected type order.completed, got %s", event.Type)
	}

	if event.Source != "orders-service" {
		t.Errorf("expected source orders-service, got %s", event.Source)
	}

	if event.Version != 1 {
		t.Errorf("expected version 1, got %d", event.Version)
	}
}

func TestNewEventValidation(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		eventType string
		source    string
		version   int
	}{
		{
			name:      "empty ID",
			id:        "",
			eventType: "order.completed",
			source:    "orders-service",
			version:   1,
		},
		{
			name:      "ID with only spaces",
			id:        "   ",
			eventType: "order.completed",
			source:    "orders-service",
			version:   1,
		},
		{
			name:      "empty event type",
			id:        "evt_001",
			eventType: "",
			source:    "orders-service",
			version:   1,
		},
		{
			name:      "empty source",
			id:        "evt_001",
			eventType: "order.completed",
			source:    "",
			version:   1,
		},
		{
			name:      "zero version",
			id:        "evt_001",
			eventType: "order.completed",
			source:    "orders-service",
			version:   0,
		},
		{
			name:      "negative version",
			id:        "evt_001",
			eventType: "order.completed",
			source:    "orders-service",
			version:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEvent(
				tt.id,
				tt.eventType,
				tt.source,
				tt.version,
			)

			if err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}