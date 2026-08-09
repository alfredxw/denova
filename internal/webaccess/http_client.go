package webaccess

import (
	"net"
	"net/http"

	"denova/internal/publicnet"
)

func isPublicFetchPolicyError(err error) bool {
	return publicnet.IsPolicyError(err)
}

func newUnboundedHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{}).DialContext
	transport.TLSHandshakeTimeout = 0
	transport.ResponseHeaderTimeout = 0
	transport.ExpectContinueTimeout = 0
	return &http.Client{Transport: transport}
}

func newPublicHTTPClient() *http.Client {
	return publicnet.NewHTTPClient()
}
