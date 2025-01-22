package worker

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/takaxis2/rate-limiter/internals/broker"
	"github.com/takaxis2/rate-limiter/internals/limiters"
	"github.com/takaxis2/rate-limiter/internals/storage"
)

type QueueWorker struct {
	qm       *storage.QueueManager
	key      string
	limiter  limiters.RateLimiter
	shutdown chan struct{}
	eb       *broker.EventBroker
}

func NewQueueWorker(qm *storage.QueueManager, key string, limiter limiters.RateLimiter, eb *broker.EventBroker) *QueueWorker {
	return &QueueWorker{
		qm:      qm,
		key:     key,
		limiter: limiter,
		eb:      eb,
	}
}

func (w *QueueWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(5000 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.shutdown:
			return
		case <-ticker.C:
			clients, err := w.qm.GetTopNClients(ctx, 1)
			if err != nil && err != redis.Nil {
				log.Printf("Error fetching from Redis: %v", err)
				continue
			}

			if len(clients) == 0 {
				continue
			}

			if clients[0] != "" {
				if w.limiter.Allow(1) {
					//채널, sse
					w.eb.Publish(clients[0])
					w.qm.RemoveClient(ctx, clients[0])
				}
				// else {
				// 	// 토큰이 없으면 다시 맨 앞에 삽입
				// 	// w.rdb.LPush(ctx, w.keyPrefix+"queue", clientId)
				// }
			}

			// default:
			// 	clientId, err := w.rdb.LPop(ctx,w.keyPrefix+"queue").Result()
			// 	if err != nil && err != redis.Nil{
			// 		log.Printf("Error fetching from Redis: %v", err)
			// 		continue
			// 	}

			// 	if clientId != "" {
			// 		if w.limiter.Allow(1){
			// 			//채널, sse
			// 		} else{
			// 			// 토큰이 없으면 다시 맨 앞에 삽입
			// 			w. rdb.LPush(ctx, w.keyPrefix+"queue", clientId)
			// 		}
			// 	}
			// 	time.Sleep(100 * time.Millisecond) // 대기시간
			// }
		}
	}
}
