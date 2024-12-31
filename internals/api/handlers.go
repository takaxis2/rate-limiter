package api

//토근 발급으로 해야하나?
//그럼 레디스
import (
	// "encoding/json"
	"html/template"
	// "net/http"
	// "time"
)

type Handler struct {
	// limiter      *ratelimit.RateLimiterWithQueue
	waitTemplate *template.Template
}

func NewHandler(limiter *ratelimit.RateLimiterWithQueue) *Handler {
	// 대기 페이지 템플릿 초기화
	tmpl := template.Must(template.New("wait").Parse(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Request Queued</title>
            <meta http-equiv="refresh" content="5">
            <style>
                body { font-family: Arial, sans-serif; margin: 40px; }
                .info { margin: 20px 0; }
            </style>
        </head>
        <body>
            <h1>Your request has been queued</h1>
            <div class="info">
                <p>Your position in queue: {{.Position}}</p>
                <p>Estimated wait time: {{.EstimatedWait.Seconds}} seconds</p>
                <p>Total queue length: {{.QueueLength}}</p>
            </div>
            <p>This page will refresh automatically every 5 seconds.</p>
        </body>
        </html>
    `))

	return &Handler{
		// limiter:      limiter,
		waitTemplate: tmpl,
	}
}

// func (h *Handler) HandleRequest(w http.ResponseWriter, r *http.Request) {
// 	userID := getUserID(r) // 실제 구현에서는 인증 시스템에서 가져옴

// 	req := &ratelimit.QueuedRequest{
// 		ID:        generateRequestID(), // UUID 등으로 구현
// 		Path:      r.URL.Path,
// 		Method:    r.Method,
// 		Timestamp: time.Now(),
// 		UserID:    userID,
// 	}

// 	canProcess, status, err := h.limiter.ProcessRequest(r.Context(), userID, req)
// 	if err != nil {
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	if canProcess {
// 		// 요청 즉시 처리
// 		h.processRequest(w, r)
// 		return
// 	}

// 	// Accept 헤더 확인하여 응답 포맷 결정
// 	if r.Header.Get("Accept") == "application/json" {
// 		w.Header().Set("Content-Type", "application/json")
// 		w.WriteHeader(http.StatusAccepted) // 202 Accepted
// 		json.NewEncoder(w).Encode(status)
// 		return
// 	}

// 	// HTML 응답
// 	w.Header().Set("Content-Type", "text/html")
// 	w.WriteHeader(http.StatusAccepted)
// 	h.waitTemplate.Execute(w, status)
// }
