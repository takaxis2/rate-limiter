package worker

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/takaxis2/rate-limiter/internals/limiters"
)

type QueueWorker struct {
	rdb       *redis.Client
	keyPrefix string
	limiter   limiters.RateLimiter
	// rate      float64 //이거 필요하나?
	shutdown chan struct{}
}

func NewQueueWorker(rdb *redis.Client, keyPrefix string, limiter limiters.RateLimiter) *QueueWorker {
	return &QueueWorker{
		rdb:       rdb,
		keyPrefix: keyPrefix,
		limiter:   limiter,
	}
}

func (w *QueueWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.shutdown:
			return
		case <-ticker.C:
			clientId, err := w.rdb.LPop(ctx, w.keyPrefix+"queue").Result()
			if err != nil && err != redis.Nil {
				log.Printf("Error fetching from Redis: %v", err)
				continue
			}

			if clientId != "" {
				if w.limiter.Allow(1) {
					//채널, sse
				} else {
					// 토큰이 없으면 다시 맨 앞에 삽입
					w.rdb.LPush(ctx, w.keyPrefix+"queue", clientId)
				}
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
