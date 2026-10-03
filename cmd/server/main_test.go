package main

import (
	"net/http"
	"testing"
)

func TestMusicHTTPServerUsesLoopbackAndStreamingSafeTimeouts(t *testing.T) {
	server := newMusicHTTPServer(http.NotFoundHandler())
	if server.Addr != "127.0.0.1:9000" {
		t.Errorf("music listener address = %q, want 127.0.0.1:9000", server.Addr)
	}
	if server.WriteTimeout != 0 {
		t.Errorf("music write timeout = %s, want disabled for long streams", server.WriteTimeout)
	}
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Error("music server must set bounded header, request, and idle timeouts")
	}
}
