package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/takaxis2/rate-limiter/internals/config"
	"github.com/takaxis2/rate-limiter/internals/limiters"
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

	//서버 설정하기
	//라우터 포함
	//나중에 config 파일로 대체할 것
	rl := limiters.NewTokenBucket(ctx, 5, 1, 1)

	sm := http.NewServeMux()
	sm.HandleFunc("/api/request", func(w http.ResponseWriter, r *http.Request) {
		queueLen, err := rdb.LLen(ctx, "").Result()
		if err != nil {
			http.Error(w, "Queue error", http.StatusInternalServerError)
		}
		if queueLen == 0 {
			if rl.Allow(1) {
				fmt.Fprintf(w, "목표 페이지로 리다이랙트")
				// http.Redirect(w, r, "https://www.naver.com", http.StatusSeeOther)
			} else {
				http.Error(w, "요청이 거부되었습니다", http.StatusTooManyRequests)
			}
		} else {
			http.Error(w, "요청이 거부되었습니다", http.StatusTooManyRequests)
		}

	})
	sm.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()

		for {
			select {
			case <-ctx.Done():
				return
			default:

			}
		}

	})
	sm.HandleFunc("/api/position", func(w http.ResponseWriter, r *http.Request) {

	})

	server := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: sm,
	}

	//워커 등록

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
