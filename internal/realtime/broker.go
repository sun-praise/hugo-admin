// Package realtime 提供 SSE 广播，替代 flask-socketio 的服务端推送
// （tts 进度、server_log、文章导入进度等事件）。
package realtime

import (
	"encoding/json"
	"sync"
)

// Event 是一次 SSE 推送：事件名 + 已序列化为 JSON 的数据。
type Event struct {
	Name string
	Data string
}

// Broker 维护 SSE 订阅者并广播事件；慢订阅者丢帧不阻塞广播。
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[chan Event]struct{})}
}

func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, 32)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
	return ch
}

func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
}

// Broadcast 向所有订阅者推事件；v 序列化为 JSON。
func (b *Broker) Broadcast(name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- Event{Name: name, Data: string(data)}:
		default:
		}
	}
}
