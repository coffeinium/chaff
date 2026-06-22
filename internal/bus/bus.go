// Пакет bus — крошечный in-process pub/sub, чтобы развязать модули. Например,
// dnssnoop публикует найденные IP, а ipblock/apply на них реагируют — без
// прямой связи между собой.
package bus

import "sync"

// Топики между модулями.
const (
	TopicIPDiscovered = "ip.discovered" // payload: netip.Addr (от dnssnoop)
	TopicReload       = "reload"        // payload: string-причина (триггер apply)
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

// Subscribe возвращает буферизованный канал с событиями топика.
func (b *Bus) Subscribe(topic string) <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	return ch
}

// Publish доставляет неблокирующе: медленный подписчик теряет события, а не
// тормозит публикатора.
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
