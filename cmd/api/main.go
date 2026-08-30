package main

import (
	"fmt"

	"github.com/Omar2709/pulseops/internal/events"
)

func main() {
	event, err := events.NewEvent(
		"evt_001",
		"order.completed",
		"orders-service",
		1,
	)

	if err != nil {
		fmt.Println("error creating event:", err)
		return
	}

	fmt.Println("Event created successfully")
	fmt.Println("ID:", event.ID)
	fmt.Println("Type:", event.Type)
	fmt.Println("Source:", event.Source)
	fmt.Println("Version:", event.Version)
}
