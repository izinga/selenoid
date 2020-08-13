package logger

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/aerokube/selenoid/server"
	log "github.com/sirupsen/logrus"
)

// responseWriter is a minimal wrapper for http.ResponseWriter that allows the
// written HTTP status code to be captured for logging.
type responseWriter struct {
	http.ResponseWriter
	status      int
	response    bytes.Buffer
	wroteHeader bool
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
func generateRandomString(size int) string {
	randBytes := make([]byte, size)
	rand.Read(randBytes)
	return hex.EncodeToString(randBytes)
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
			log.SetFormatter(&log.JSONFormatter{})
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
			if r.Method == "DELETE" {
				log.Info("User requested to close session ")
				server.StopServer(time.Duration(4))
			}
		}
		return fn
	}
}
