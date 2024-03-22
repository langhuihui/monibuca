package pkg

// EventBus is a simple event bus
type EventBus chan any

// NewEventBus creates a new EventBus
func NewEventBus(size int) EventBus {
	return make(chan any, size)
}

// // Publish publishes an event
// func (e *EventBus) Publish(event any) {
// }

// // Subscribe subscribes to an event
// func (e *EventBus) Subscribe(event any, handler func(event any)) {
// }
