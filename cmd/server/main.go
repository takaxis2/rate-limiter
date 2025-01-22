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
	rl := limiters.NewTokenBucket(ctx, 1, 1, 1)

	eb := broker.NewEventBroker()
	sm := handler.NewHandlers(rl, qm, eb)
	// sm := http.NewServeMux()

	// sm.HandleFunc("/api/request", func(w http.ResponseWriter, r *http.Request) {
	// 	queueLen, err := qm.GetTotalClients(ctx)
	// 	if err != nil {
	// 		http.Error(w, "Queue error", http.StatusInternalServerError)
	// 	}
	// 	if queueLen == 0 {
	// 		if rl.Allow(1) {
	// 			fmt.Fprintf(w, "목표 페이지로 리다이랙트")
	// 			// http.Redirect(w, r, "https://www.naver.com", http.StatusSeeOther)
	// 			// http.SetCookie(w, &http.Cookie{
	// 			// 	Name:  "UserId",
	// 			// 	Value: uuid.New().String(),
	// 			// })
	// 			// http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
	// 		} else {
	// 			// reqData := handler.RequestData{
	// 			// 	UserID:      uuid.New().String(),
	// 			// 	RequestType: "queue",
	// 			// }

	// 			// data, _ := json.Marshal(reqData)
	// 			// score := float64(time.Now().UnixNano())
	// 			clientID := uuid.New().String()

	// 			// rdb.ZAdd(ctx, "queue", redis.Z{
	// 			// 	Score:  score,
	// 			// 	Member: clientID,
	// 			// })

	// 			qm.AddClient(ctx, clientID)
	// 			// http.Error(w, "큐는 비었지만 토큰 x, 요청이 거부되었습니다", http.StatusTooManyRequests)

	// 			http.SetCookie(w, &http.Cookie{
	// 				Name:  "UserId",
	// 				Value: clientID,
	// 			})
	// 			http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
	// 		}
	// 	} else {
	// 		clientID := uuid.New().String()
	// 		qm.AddClient(ctx, clientID)
	// 		http.SetCookie(w, &http.Cookie{
	// 			Name:  "UserId",
	// 			Value: clientID,
	// 		})
	// 		http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
	// 	}

	// })
	// sm.HandleFunc("/api/wait", func(w http.ResponseWriter, r *http.Request) {
	// 	tmpl, err := template.ParseFiles("./static/index.html")
	// 	if err != nil {
	// 		http.Error(w, "템플릿 로드 실패", http.StatusInternalServerError)
	// 		return
	// 	}
	// 	data := map[string]interface{}{
	// 		"WaitingNumber": "100",
	// 	}

	// 	if err := tmpl.Execute(w, data); err != nil {
	// 		http.Error(w, "템플릿 렌더링 실패", http.StatusInternalServerError)
	// 	}
	// })
	// sm.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
	// 	w.Header().Set("Content-Type", "text/event-stream")
	// 	w.Header().Set("Cache-Control", "no-cache")
	// 	w.Header().Set("Connection", "keep-alive")
	// 	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 	flusher, ok := w.(http.Flusher)
	// 	if !ok {
	// 		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
	// 		return
	// 	}

	// 	events := eb.Subsribe()
	// 	ctx := r.Context()

	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case userID := <-events:
	// 			event := map[string]string{
	// 				"type":    "processed",
	// 				"user_id": userID,
	// 			}
	// 			data, _ := json.Marshal(event)
	// 			fmt.Fprintf(w, "data: %s\n\n", string(data))
	// 			// w.Write([]byte("data:" + string(data) + "\n\n"))
	// 			flusher.Flush()
	// 		}
	// 	}

	// })
	// sm.HandleFunc("/api/position", func(w http.ResponseWriter, r *http.Request) {

	// })

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
