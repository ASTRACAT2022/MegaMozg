package api

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

func withMiddleware(handler http.Handler) http.Handler {
	return recoverMiddleware(requestLogger(handler))
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)

		log.Printf("method=%s path=%s status=%d duration=%s remote=%s", r.Method, r.URL.Path, rr.status, time.Since(start), r.RemoteAddr)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic path=%s error=%v\n%s", r.URL.Path, rec, string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
