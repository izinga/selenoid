package logger

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/aerokube/selenoid/server"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

var (
	NerveServer   string
	TestsessionID string
	ResultID      string
	restyClient   *resty.Client
)

// logMessage formate
type logMessage struct {
	Target  string `json:"target"`
	Method  string `json:"method"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Params  struct {
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}
}

// responseWriter is a minimal wrapper for http.ResponseWriter that allows the
// written HTTP status code to be captured for logging.
type responseWriter struct {
	http.ResponseWriter
	status      int
	response    bytes.Buffer
	wroteHeader bool
}

func init() {
	restyClient = resty.New()
	restyClient.SetHeader("Accept", "application/json")
	restyClient.SetTimeout(60 * time.Second)

}

// Setup set nerve related data
func Setup(nerveServer, testsessionID, resultID string) {
	NerveServer = nerveServer
	TestsessionID = testsessionID
	ResultID = resultID
	if ResultID == "" {
		ResultID = generateRandomString(10)
	}

	log.SetFormatter(&log.JSONFormatter{})
	// file, err := os.OpenFile(fmt.Sprintf("%s_%s.log", TestsessionID, ResultID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err == nil {
	// 	log.SetOutput(file)
	// }
}

func generateRandomString(size int) string {
	randBytes := make([]byte, size)
	rand.Read(randBytes)
	return hex.EncodeToString(randBytes)
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true

	return
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	rw.response.Write(data)
	return rw.ResponseWriter.Write(data)
}

func getUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}

// LoggingMiddleware logs the incoming HTTP request & its duration.
func LoggingMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		fn := func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			dump, _ := httputil.DumpRequest(r, true)
			ID := generateRandomString(14)
			start := time.Now()
			entry := log.WithFields(log.Fields{
				"identifier": ID,
				"status":     "receive",
				"method":     r.Method,
				"path":       r.URL.EscapedPath(),
				"ip":         getUserIP(r),
				"body":       string(dump),
			})
			entry.Info()
			go sendLogToNerve(logMessage{
				Target:  r.URL.EscapedPath(),
				Method:  r.Method,
				Level:   "INFO",
				Message: fmt.Sprintf("--> %s %s \n %s ", r.Method, r.URL.EscapedPath(), string(dump)),
			})
			wrapped := wrapResponseWriter(w)
			next.ServeHTTP(wrapped, r)
			latency := time.Since(start)

			entry = log.WithFields(log.Fields{
				"identifier": ID,
				"status":     wrapped.status,
				"method":     r.Method,
				"path":       r.URL.EscapedPath(),
				"ip":         getUserIP(r),
				"latency":    latency,
				"response":   wrapped.response.String(),
			})
			entry.Info()
			go sendLogToNerve(logMessage{
				Target:  r.URL.EscapedPath(),
				Method:  r.Method,
				Level:   "INFO",
				Message: fmt.Sprintf("<-- %s %s %d %d ms\n %s ", r.Method, r.URL.EscapedPath(), wrapped.status, latency.Milliseconds(), wrapped.response.String()),
			})
			if r.Method == "DELETE" {
				log.Info("User requested to close session ")
				server.StopServer(time.Duration(4))
			}
		}
		return fn
	}
}

func sendLogToNerve(logData logMessage) {
	payload, err := json.Marshal(logData)
	if err == nil {
		url := fmt.Sprintf("%s/v2/appiumlog/%s/%s", NerveServer, TestsessionID, ResultID)
		fmt.Printf("appium log url %s \n", url)
		resp, err := restyClient.R().SetBody(payload).Post(url)
		if err != nil {
			log.Error("Unable to push selenium log to nerve ", err.Error())
		} else if resp.StatusCode() != http.StatusOK {
			err = fmt.Errorf("Unable to push selenium log to nerve, status code  %d", resp.StatusCode())
		}
	} else {
		log.Error("Unable to marshal log data ", err.Error())
	}
}
