package pkg

// EventBus is a simple event bus
type EventBus chan any
// NewEventBus creates a new EventBus
func NewEventBus() EventBus {
	return make(chan any)
}

// // Publish publishes an event
// func (e *EventBus) Publish(event any) {
// }

// // Subscribe subscribes to an event
// func (e *EventBus) Subscribe(event any, handler func(event any)) {
// }
