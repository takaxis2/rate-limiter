package broker

type EventBroker struct {
	events chan string
}

func NewEventBroker() *EventBroker {
	return &EventBroker{
		events: make(chan string, 100),
	}
}

func (b *EventBroker) Subsribe() <-chan string {
	return b.events
}

func (b *EventBroker) Publish(userID string) {
	b.events <- userID
}
