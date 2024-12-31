package limiters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type QueuedRequest struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id"`
}

type QueueStatus struct {
	Position      int           `json:"position"`       // 대기열에서의 위치
	EstimatedWait time.Duration `json:"estimated_wait"` // 예상 대기 시간
	QueueLength   int           `json:"queue_length"`   // 전체 대기열 길이
}

type RateLimiterWithQueue struct {
	rdb       *redis.Client
	keyPrefix string
	rate      float64 // 초당 처리할 수 있는 요청 수
	capacity  float64 // 버킷 최대 용량
}

func NewRateLimiterWithQueue(rdb *redis.Client, keyPrefix string, rate, capacity float64) *RateLimiterWithQueue {
	return &RateLimiterWithQueue{
		rdb:       rdb,
		keyPrefix: keyPrefix,
		rate:      rate,
		capacity:  capacity,
	}
}

// ProcessRequest handles the incoming request with rate limiting
func (rl *RateLimiterWithQueue) ProcessRequest(ctx context.Context, userID string, req *QueuedRequest) (bool, *QueueStatus, error) {
	// 1. 토큰 확인
	tokens, err := rl.checkAndUpdateTokens(ctx, userID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to check tokens: %w", err)
	}

	// 토큰이 충분한 경우 즉시 처리
	if tokens >= 1 {
		return true, nil, nil
	}

	// 2. 토큰이 부족한 경우 큐에 추가
	queueKey := fmt.Sprintf("%s:queue:%s", rl.keyPrefix, userID)
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return false, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Redis 트랜잭션으로 큐에 추가하고 위치 확인
	pipe := rl.rdb.Pipeline()
	pipe.LPush(ctx, queueKey, reqJSON)
	queueLen := pipe.LLen(ctx, queueKey)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("failed to add to queue: %w", err)
	}

	// 3. 큐 상태 정보 생성
	position := int(queueLen.Val())
	estimatedWait := time.Duration(float64(position) / rl.rate * float64(time.Second))
	status := &QueueStatus{
		Position:      position,
		EstimatedWait: estimatedWait,
		QueueLength:   position,
	}

	return false, status, nil
}

// checkAndUpdateTokens checks and updates token bucket
func (rl *RateLimiterWithQueue) checkAndUpdateTokens(ctx context.Context, userID string) (float64, error) {
	key := fmt.Sprintf("%s:tokens:%s", rl.keyPrefix, userID)
	now := time.Now()

	// Redis 트랜잭션으로 토큰 업데이트
	pipe := rl.rdb.Pipeline()
	lastRefillCmd := pipe.HGet(ctx, key, "lastRefill")
	tokensCmd := pipe.HGet(ctx, key, "tokens")
	_, err := pipe.Exec(ctx)

	var tokens float64 = rl.capacity
	var lastRefill time.Time = now

	if err != redis.Nil {
		if lastRefillStr, err := lastRefillCmd.Result(); err == nil {
			lastRefill, _ = time.Parse(time.RFC3339, lastRefillStr)
		}
		if tokensStr, err := tokensCmd.Result(); err == nil {
			tokens, _ = strconv.ParseFloat(tokensStr, 64)
		}
	}

	// 토큰 리필
	elapsed := now.Sub(lastRefill).Seconds()
	tokens = min(rl.capacity, tokens+(elapsed*rl.rate))

	// 토큰이 충분한 경우 하나 사용
	if tokens >= 1 {
		tokens--
		pipe := rl.rdb.Pipeline()
		pipe.HSet(ctx, key, "tokens", tokens)
		pipe.HSet(ctx, key, "lastRefill", now.Format(time.RFC3339))
		_, err = pipe.Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to update tokens: %w", err)
		}
	}

	return tokens, nil
}
