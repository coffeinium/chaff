package bus

import "sync"

const (
	TopicIPDiscovered = "ip.discovered"
	TopicReload       = "reload"
)

type Event struct {
	Topic string
	Data  any
}

type Bus struct {
	mu   sync.RWMutex
	subs map[string][]chan Event
}

func New() *Bus {
	return &Bus{subs: make(map[string][]chan Event)}
}

func (b *Bus) Subscribe(topic string) <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	return ch
}

func (b *Bus) Unsubscribe(topic string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[topic]
	for i, c := range subs {
		if c == ch {
			b.subs[topic] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[e.Topic] {
		select {
		case ch <- e:
		default:
		}
	}
}
