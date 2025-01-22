package storage

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type QueueManager struct {
	rdb      *redis.Client
	queueKey string
}

func NewQueueManager(rdb *redis.Client, queueKey string) *QueueManager {
	return &QueueManager{
		rdb: rdb,
	}
}

// AddClient 새로운 클라이언트를 대기열에 추가
func (qm *QueueManager) AddClient(ctx context.Context, clientID string) error {
	// 현재 timestamp를 score로 사용하여 자연스러운 순서 부여
	score := float64(time.Now().UnixNano())
	return qm.rdb.ZAdd(ctx, qm.queueKey, redis.Z{
		Score:  score,
		Member: clientID,
	}).Err()
}

// RemoveClient 클라이언트를 대기열에서 제거
func (qm *QueueManager) RemoveClient(ctx context.Context, clientID string) error {
	return qm.rdb.ZRem(ctx, qm.queueKey, clientID).Err()
}

// GetClientPosition 특정 클라이언트의 현재 대기 순서 조회 (0부터 시작)
func (qm *QueueManager) GetClientPosition(ctx context.Context, clientID string) (int64, error) {
	return qm.rdb.ZRank(ctx, qm.queueKey, clientID).Result()
}

// GetTotalClients 전체 대기 중인 클라이언트 수 조회
func (qm *QueueManager) GetTotalClients(ctx context.Context) (int64, error) {
	return qm.rdb.ZCard(ctx, qm.queueKey).Result()
}

// GetTopNClients 상위 N명의 클라이언트 목록 조회
func (qm *QueueManager) GetTopNClients(ctx context.Context, n int64) ([]string, error) {
	return qm.rdb.ZRange(ctx, qm.queueKey, 0, n-1).Result()
}

// GetNextClient 다음 순서의 클라이언트 조회 및 제거
func (qm *QueueManager) GetNextClient(ctx context.Context) (string, error) {
	// 트랜잭션으로 처리하여 원자성 보장
	txf := func(tx *redis.Tx) error {
		// 첫 번째 클라이언트 조회
		clients, err := tx.ZRange(ctx, qm.queueKey, 0, 0).Result()
		if err != nil {
			return err
		}
		if len(clients) == 0 {
			return redis.Nil
		}

		// 클라이언트 제거
		_, err = tx.ZRem(ctx, qm.queueKey, clients[0]).Result()
		return err
	}

	// 낙관적 락을 사용한 트랜잭션 실행
	for i := 0; i < 3; i++ {
		clientID := ""
		err := qm.rdb.Watch(ctx, txf, qm.queueKey)
		if err == nil {
			// 성공적으로 처리된 경우
			return clientID, nil
		}
		if err == redis.TxFailedErr {
			// 충돌이 발생한 경우 재시도
			continue
		}
		return "", err
	}
	return "", redis.TxFailedErr
}
