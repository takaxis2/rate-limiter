package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/takaxis2/rate-limiter/cmd/server/handler"
	"github.com/takaxis2/rate-limiter/internals/broker"
	"github.com/takaxis2/rate-limiter/internals/config"
	"github.com/takaxis2/rate-limiter/internals/limiters"
	worker "github.com/takaxis2/rate-limiter/internals/service"
	"github.com/takaxis2/rate-limiter/internals/storage"
)

func main() {
	//설정 불러오기 db든 yaml이든
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	//db 커넥션 설정하기
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	qm := storage.NewQueueManager(rdb, "domain")

	//서버 설정하기
	//라우터 포함
	//나중에 config 파일로 대체할 것
	rl := limiters.NewTokenBucket(ctx, 3, 0.1, 1)

	eb := broker.NewEventBroker()
	sm := handler.NewHandlers(rl, qm, eb)

	server := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: sm,
	}

	//서버시작
	//그레이스풀 셧다운
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started on %v", cfg.Server.Address)

	//워커 등록
	wkr := worker.NewQueueWorker(qm, "domain", rl, eb)
	wkr.Start(ctx)

	//종료 신호 대기
	<-shutdown
	log.Println("Shutting down server...")

	//그레이스풀 셧다운 실행
	shutdownCtx, cnacel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cnacel()

	//워커 종료

	//서버 종료
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped gracefully")
}
