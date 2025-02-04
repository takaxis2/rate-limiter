package limiters

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const LIMITER_CAPACITY = 1024

type RateLimiter interface {
	Allow(int) bool
	Stop()
}

type requestTokensCh struct {
	tokens int
	resCh  chan bool
}

type RateLimiterBase struct {
	allowCh  chan requestTokensCh
	stopFunc context.CancelFunc
	wg       sync.WaitGroup
	isClosed bool
	mu       sync.RWMutex
}

func (rlb *RateLimiterBase) Allow(tokens int) bool {
	if tokens <= 0 {
		return false
	}
	isClosed := false
	rlb.mu.RLock()
	isClosed = rlb.isClosed
	rlb.mu.RUnlock()
	if isClosed {
		return false
	}

	reqTokensCh := requestTokensCh{
		tokens: tokens,
		resCh:  make(chan bool, 1),
	}

	rlb.allowCh <- reqTokensCh
	return <-reqTokensCh.resCh
}

func (rlb *RateLimiterBase) Stop() {
	rlb.stopFunc()
	rlb.wg.Wait()
	rlb.mu.Lock()
	rlb.isClosed = true
	rlb.mu.Unlock()
	close(rlb.allowCh)
}

type TokenBucket struct {
	capacity        float32
	tokensPerSecond float32
	tokens          float32
	lastTime        time.Time
	*RateLimiterBase
}

func NewTokenBucket(ctx context.Context, capacity, tokensPerSecond, tokens float32) RateLimiter {
	allowCh := make(chan requestTokensCh, LIMITER_CAPACITY)
	ctx, cancelFunc := context.WithCancel(ctx)
	rlBase := &RateLimiterBase{
		allowCh:  allowCh,
		stopFunc: cancelFunc,
	}
	rl := &TokenBucket{
		RateLimiterBase: rlBase,
		capacity:        capacity,
		tokensPerSecond: tokensPerSecond,
		tokens:          tokens,
		lastTime:        time.Now(),
	}
	rl.refillTokens()
	rl.wg.Add(1)
	go rl.tokenBucketAlgorithm(ctx)

	return rl
}

func (rl *TokenBucket) tokenBucketAlgorithm(ctx context.Context) {
	// runs the token bucket algorithm in a separate goroutine and also checks for event(cancelling the context) to stop this goroutine
	defer rl.wg.Done()

	// tokenPerSecond를 적용한것
	// ticker := time.NewTicker(time.Duration(1e9 / int64(rl.tokensPerSecond)))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.refillTokens()
		case reqTokensCh := <-rl.allowCh:
			fmt.Printf("available tokens: %f \n", rl.tokens)

			resp := false

			if float32(reqTokensCh.tokens) <= rl.tokens {
				rl.tokens -= float32(reqTokensCh.tokens)
				resp = true
			}
			reqTokensCh.resCh <- resp
			close(reqTokensCh.resCh)
		}
	}
}

func (rl *TokenBucket) refillTokens() {
	// 이 방법은 tokenBucketAlgorithm에서 tokenPerSecond를 적용하지 않았을때
	// 초당 1번 불러지면 tokenPerSecond만큼의 토큰을 한번에 충전
	// tokenPerSecond가 tokenBucketAlgorithm에 적용 되었다면
	// 예를 들어 초당 5개의 토큰이 생성된다 가정했을때
	// 200ms마다 토큰을 하나씩 충전하는 형태가 된다

	// currentTime := time.Now()
	// timePassed := currentTime.Sub(rl.lastTime).Seconds()
	// temp := rl.tokens + int(timePassed)*rl.tokensPerSecond
	// rl.tokens = temp
	// if rl.capacity < rl.tokens {
	// 	rl.tokens = rl.capacity
	// }
	// rl.lastTime = currentTime

	//=========================================================

	newTokens := rl.tokensPerSecond
	rl.tokens += newTokens

	if rl.tokens > rl.capacity {
		rl.tokens = rl.capacity
	}

	fmt.Printf("total tokens: %d \n", rl.tokens)
}

type LeakyBucket struct {
	capacity int
	leakRate int
	tokens   int
	lastTime time.Time
	*RateLimiterBase
}

func NewLeakyBucket(capacity, leakRate int) RateLimiter {
	allowCh := make(chan requestTokensCh, LIMITER_CAPACITY)
	ctx, cancelFunc := context.WithCancel(context.Background())
	rlBase := &RateLimiterBase{
		allowCh:  allowCh,
		stopFunc: cancelFunc,
	}
	rl := &LeakyBucket{
		RateLimiterBase: rlBase,
		capacity:        capacity,
		leakRate:        leakRate,
		tokens:          capacity,
		lastTime:        time.Now(),
	}

	rl.wg.Add(1)
	go rl.leakyBucketAlgorithm(ctx)

	return rl
}

func (rl *LeakyBucket) leakyBucketAlgorithm(ctx context.Context) {
	// runs the leaky bucket algorithm in a separate goroutine and also checks for event(cancelling the context) to stop this goroutine

	defer rl.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case reqTokensCh := <-rl.allowCh:
			fmt.Printf("tokens requested: %d, available tokens: %d ", reqTokensCh.tokens, rl.tokens)
			currentTime := time.Now()
			timePassed := currentTime.Sub(rl.lastTime).Seconds()

			leakedTokens := int(timePassed) * rl.leakRate

			temp := rl.tokens - leakedTokens
			if temp < 0 {
				rl.tokens = 0
			} else {
				rl.tokens = temp
			}
			fmt.Printf("total new tokens: %d ", rl.tokens)

			rl.lastTime = currentTime
			resp := false

			if reqTokensCh.tokens <= (rl.capacity - rl.tokens) {
				rl.tokens += reqTokensCh.tokens
				resp = true
			} else {
				resp = false
			}
			reqTokensCh.resCh <- resp
			close(reqTokensCh.resCh)
		}
	}
}

type FixedWindow struct {
	tokens     int
	windowSize int
	capacity   int
	lastTime   time.Time
	*RateLimiterBase
}

func NewFixedWindow(windowSize, capacity int) RateLimiter {
	allowCh := make(chan requestTokensCh, LIMITER_CAPACITY)
	ctx, cancelFunc := context.WithCancel(context.Background())
	rlBase := &RateLimiterBase{
		allowCh:  allowCh,
		stopFunc: cancelFunc,
	}
	rl := &FixedWindow{
		RateLimiterBase: rlBase,
		tokens:          capacity,
		capacity:        capacity,
		windowSize:      windowSize,
		lastTime:        time.Now(),
	}

	rl.wg.Add(1)
	go rl.fixedWindowAlgorithm(ctx)

	return rl
}

func (rl *FixedWindow) fixedWindowAlgorithm(ctx context.Context) {
	// runs the fixed window algorithm in a separate goroutine and also checks for event(cancelling the context) to stop this goroutine

	defer rl.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case reqTokensCh := <-rl.allowCh:
			fmt.Printf("tokens requested: %d, available tokens: %d ", reqTokensCh.tokens, rl.tokens)
			currentTime := time.Now()
			timePassed := int(currentTime.Sub(rl.lastTime).Seconds())

			resp := false
			if timePassed >= rl.windowSize {
				rl.lastTime = currentTime
				rl.tokens = rl.capacity - reqTokensCh.tokens
				if rl.tokens < 0 {
					rl.tokens = rl.capacity
					resp = false
				} else {
					resp = true
				}
			} else {
				if rl.tokens >= reqTokensCh.tokens {
					rl.tokens -= reqTokensCh.tokens
					resp = true
				} else {
					resp = false
				}
			}
			fmt.Printf("total new tokens: %d ", rl.tokens)
			reqTokensCh.resCh <- resp
			close(reqTokensCh.resCh)
		}
	}
}

type SlidingWindow struct {
	limit      int
	windowSize time.Duration
	timeStamps []time.Time
	*RateLimiterBase
}

func NewSlidingWindow(limit int, windowSize time.Duration) RateLimiter {
	allowCh := make(chan requestTokensCh, LIMITER_CAPACITY)
	ctx, cancelFunc := context.WithCancel(context.Background())
	rlBase := &RateLimiterBase{
		allowCh:  allowCh,
		stopFunc: cancelFunc,
	}
	rl := &SlidingWindow{
		RateLimiterBase: rlBase,
		limit:           limit,
		windowSize:      windowSize,
		timeStamps:      make([]time.Time, 0),
	}

	rl.wg.Add(1)
	go rl.slidingWindowAlgorithm(ctx)

	return rl
}

func (rl *SlidingWindow) slidingWindowAlgorithm(ctx context.Context) {
	// runs the sliding window algorithm in a separate goroutine and also checks for event(cancelling the context) to stop this goroutine

	defer rl.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case reqTokensCh := <-rl.allowCh:
			currentTime := time.Now()
			// append as many entries as tokens requested
			for i := 0; i < reqTokensCh.tokens; i++ {
				rl.timeStamps = append(rl.timeStamps, currentTime)
			}
			fmt.Printf("total requests: %d, limit: %d ", reqTokensCh.tokens, rl.limit)

			for len(rl.timeStamps) > 0 && rl.timeStamps[0].Before(currentTime.Add(-rl.windowSize)) {
				rl.timeStamps = rl.timeStamps[1:]
			}
			fmt.Printf("total requests after sliding: %d ", len(rl.timeStamps))
			resp := false
			totalTokensInWindow := len(rl.timeStamps)
			if totalTokensInWindow <= rl.limit {
				resp = true
			} else {
				// roll back the tokens if the request can't be fulfilled
				rl.timeStamps = rl.timeStamps[:totalTokensInWindow-reqTokensCh.tokens]
			}
			reqTokensCh.resCh <- resp
			close(reqTokensCh.resCh)
		}
	}
}

func main() {
	// rl := NewTokenBucket(10, 5, 5)
	// var ok bool
	// for i := 0; i < 10; i++ {
	// 	ok = rl.Allow(1)
	// 	if ok {
	// 		fmt.Println("access granted")
	// 	} else {
	// 		fmt.Println("access denied")
	// 		time.Sleep(1 * time.Second)
	// 	}

	// }
	// rl.Stop()

	rl := NewLeakyBucket(10, 200)
	var ok bool
	for i := 0; i < 10; i++ {
		ok = rl.Allow(2)
		if ok {
			fmt.Println("access granted")
			time.Sleep(5 * time.Second)
		} else {
			fmt.Println("access denied")
			time.Sleep(3 * time.Second)
		}

	}
	rl.Stop()

	// rl := NewFixedWindow(1, 5)
	// var ok bool
	// for i := 0; i < 10; i++ {
	// 	ok = rl.Allow(1)
	// 	if ok {
	// 		fmt.Println("access granted")
	// 	} else {
	// 		fmt.Println("access denied")
	// 		time.Sleep(1 * time.Second)
	// 	}

	// }
	// rl.Stop()

	// rl := NewSlidingWindow(5, 1)
	// var ok bool
	// for i := 0; i < 10; i++ {
	// 	ok = rl.Allow(1)
	// 	if ok {
	// 		fmt.Println("access granted")
	// 	} else {
	// 		fmt.Println("access denied")
	// 		time.Sleep(1 * time.Second)
	// 	}

	// }
	// rl.Stop()
}
