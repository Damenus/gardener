// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Server is a HTTP(S) server.
type Server struct {
	log          logr.Logger
	bindAddress  string
	port         int
	tlsCertPath  *string
	tlsKeyPath   *string
	handlers     map[string]http.Handler
	handlerFuncs map[string]http.HandlerFunc
}

// Start starts the server. If the TLS cert and key paths are provided then it will start it as HTTPS server.
func (s *Server) Start(ctx context.Context) {
	var (
		log           = s.log.WithName("server")
		listenAddress = fmt.Sprintf("%s:%d", s.bindAddress, s.port)
		serverMux     = http.NewServeMux()
		server        = &http.Server{Addr: listenAddress, Handler: serverMux}
	)

	// Add handlers to HTTPS server and start it.
	for pattern, handler := range s.handlers {
		serverMux.Handle(pattern, handler)
	}
	for pattern, handlerFunc := range s.handlerFuncs {
		serverMux.HandleFunc(pattern, handlerFunc)
	}

	// Server startup logic.
	go func() {
		if s.tlsCertPath != nil && s.tlsKeyPath != nil {
			log.Info("Starting new HTTPS server", "listenAddress", listenAddress)
			if err := server.ListenAndServeTLS(*s.tlsCertPath, *s.tlsKeyPath); err != http.ErrServerClosed {
				log.Error(err, "Could not start HTTPS server")
			}
			return
		}

		log.Info("Starting new HTTP server", "listenAddress", listenAddress)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Error(err, "Could not start HTTP server")
		}
	}()

	// Server shutdown logic.
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error(err, "Error shutting down server")
	}
	log.Info("Server stopped")
}
