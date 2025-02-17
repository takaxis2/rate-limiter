package handler

//토근 발급으로 해야하나?
//그럼 레디스
import (
	// "encoding/json"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/takaxis2/rate-limiter/internals/broker"
	"github.com/takaxis2/rate-limiter/internals/limiters"
	"github.com/takaxis2/rate-limiter/internals/storage"
	// "time"
)

// type Handlers struct {
// 	rl limiters.RateLimiter
// 	qm *storage.QueueManager
// 	eb *broker.EventBroker
// }

type RequestData struct {
	UserID      string `json:"user_id"`
	RequestType string `json:"request_type"`
}

type QueueStatus struct {
	Position      int           `json:"position"`       // 대기열에서의 위치
	EstimatedWait time.Duration `json:"estimated_wait"` // 예상 대기 시간
	QueueLength   int           `json:"queue_length"`   // 전체 대기열 길이
}

type TokenBucketConfig struct {
	Capacity   float32 `json:"capacity"`
	RefillRate float32 `json:"refillRate"`
}

type UserStatus string

const (
	StatusQueued    UserStatus = "queued"
	StatusProcessed UserStatus = "processed"
)

type UserInfo struct {
	ID     string     `json:"id"`
	Status UserStatus `json:"status"`
}

func NewHandlers(rl limiters.RateLimiter, qm *storage.QueueManager, eb *broker.EventBroker) *http.ServeMux {

	sm := http.NewServeMux()
	sm.HandleFunc("/api/request", RequestHandler(qm, rl)) // 핸들러 함수로 변경
	sm.HandleFunc("/api/wait", WaitHandler(qm))           // 핸들러 함수로 변경
	sm.HandleFunc("/api/events", EventsHandler(eb))       // 핸들러 함수로 변경
	sm.HandleFunc("/api/position", func(w http.ResponseWriter, r *http.Request) {})
	sm.Handle("/metric", promhttp.Handler())
	sm.HandleFunc("/config/tb", TokenBucketConfigHandler(rl))

	return sm
}

func RequestHandler(qm *storage.QueueManager, rl limiters.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		//request에서 도메인값을 가져온다
		queueLen, err := qm.GetTotalClients(ctx)
		if err != nil {
			http.Error(w, "Queue error", http.StatusInternalServerError)
			return
		}
		// 대기자가 없을경우
		// if queueLen == 0 {
		// 	// 대기자가 없지만 토큰은 있을때
		// 	if rl.Allow(1) {
		// 		fmt.Fprintf(w, "목표 페이지로 리다이랙트")
		// 	} else { // 대기자도 토큰도 없을때
		// 		clientID := uuid.New().String()
		// 		qm.AddClient(ctx, clientID)
		// 		http.SetCookie(w, &http.Cookie{
		// 			Name:  "UserId",
		// 			Value: clientID,
		// 		})
		// 		http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
		// 	}
		// } else { // 대기자가 있으면 토큰이 있든 없든 댜기열에 추가가
		// 	clientID := uuid.New().String()
		// 	qm.AddClient(ctx, clientID)
		// 	http.SetCookie(w, &http.Cookie{
		// 		Name:  "UserId",
		// 		Value: clientID,
		// 	})
		// 	http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
		// }

		clientID := uuid.New().String()
		userInfo := UserInfo{
			ID: clientID,
		}
		// 대기자가 없고 토큰이 있는 경우에만 즉시 리다이렉트
		if queueLen == 0 && rl.Allow(1) {
			fmt.Fprintf(w, "목표 레이지로 리다이렉트")
			userInfo.Status = StatusProcessed
		} else
		// 그 외의 경우에는 무조건 대기열에 추가
		// 대기열이 있거나, 토큰이 없거나, 둘다 해당되거나
		{
			userInfo.Status = StatusQueued

			userInfoJson, err := json.Marshal(userInfo)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}

			qm.AddClient(ctx, clientID)
			http.SetCookie(w, &http.Cookie{
				Name:  "UserInfo",
				Value: base64.StdEncoding.EncodeToString(userInfoJson),
				// HttpOnly: true,
			})
			http.Redirect(w, r, "/api/wait", http.StatusSeeOther)
		}
	}
}

func WaitHandler(qm *storage.QueueManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tmpl, err := template.ParseFiles("server/static/index.html")
		if err != nil {
			fmt.Print(err)
			http.Error(w, "템플릿 로드 실패", http.StatusInternalServerError)
			return
		}
		userInfoCookie, err := r.Cookie("UserInfo")
		if err != nil {
			http.Error(w, "유저 정보가 없습니다", http.StatusInternalServerError)
			return
		}
		// base64 디코딩
		userInfoBytes, err := base64.StdEncoding.DecodeString(userInfoCookie.Value)
		if err != nil {
			http.Error(w, "Invalid user info format", http.StatusInternalServerError)
			return
		}

		// JSON 디코딩
		var userInfo UserInfo
		if err := json.Unmarshal(userInfoBytes, &userInfo); err != nil {
			http.Error(w, "Invalid user info data", http.StatusInternalServerError)
			return
		}

		wnum, err := qm.GetClientPosition(ctx, userInfo.ID)
		if err != nil {
			http.Error(w, "유저 정보가 없습니다", http.StatusInternalServerError)
			return
		}

		data := map[string]interface{}{
			"WaitingNumber": wnum,
		}

		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "템플릿 렌더링 실패", http.StatusInternalServerError)
		}
	}
}

func EventsHandler(eb *broker.EventBroker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		events := eb.Subsribe()
		ctx := r.Context()

		for {
			select {
			case <-ctx.Done():
				return
			case userID := <-events:
				event := map[string]string{
					"type":    "processed",
					"user_id": userID,
				}
				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				// w.Write([]byte("data:" + string(data) + "\n\n"))
				flusher.Flush()
			}
		}
	}
}

func TokenBucketConfigHandler(rl limiters.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tmpl, err := template.ParseFiles("./static/config.html")
			if err != nil {
				http.Error(w, "템플릿 로드 실패", http.StatusInternalServerError)
				return
			}
			tmpl.Execute(w, nil)

		case http.MethodPut:
			var config TokenBucketConfig
			if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// 값 유효성 검사
			if config.Capacity <= 0 || config.RefillRate <= 0 {
				http.Error(w, "Invalid values: capacity and refillRate must be positive", http.StatusBadRequest)
				return
			}

			// TokenBucket으로 타입 변환
			tokenBucket, ok := rl.(*limiters.TokenBucket)
			if !ok {
				http.Error(w, "Rate limiter is not a token bucket", http.StatusInternalServerError)
				return
			}

			// 설정 업데이트
			tokenBucket.UpdateConfig(config.Capacity, config.RefillRate)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Token bucket configuration updated successfully",
			})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
