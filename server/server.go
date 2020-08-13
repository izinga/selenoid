package server

import (
	"context"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// Server  server
var Server = &http.Server{}

// StopServer sever
func StopServer(gracefulPeriod time.Duration) {
	log.Info("SHUTTING_DOWN", gracefulPeriod)
	ctx, cancel := context.WithTimeout(context.Background(), gracefulPeriod)
	defer cancel()
	if err := Server.Shutdown(ctx); err != nil {
		log.Error("SHUTTING_DOWN ", err)
	}

}
