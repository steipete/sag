package cmd

import (
	"net/http"
	"net/http/httptest"
)

// handlerRoundTripper adapts an http.Handler to an http.RoundTripper without binding sockets.
type handlerRoundTripper struct {
	handler http.Handler
}

// RoundTrip satisfies http.RoundTripper by invoking the handler directly.
func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rr := httptest.NewRecorder()
	rt.handler.ServeHTTP(rr, req)
	res := rr.Result()
	res.Request = req
	return res, nil
}
