package httplog

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

type LoggingTransport struct {
	Logger    *Logger
	Transport http.RoundTripper
}

func getReqBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}

	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	return body, nil
}

func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Transport == nil {
		t.Transport = http.DefaultTransport
	}

	if t.Logger != nil {
		body, err := getReqBody(req)
		if err != nil {
			return nil, err
		}

		t.Logger.LogRequest(req, body)
	}

	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return resp, err //nolint:wrapcheck // passthrough error from underlying transport
	}

	if t.Logger != nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp, fmt.Errorf("failed to read response body: %w", err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))

		t.Logger.LogResponse(resp, body)
	}

	return resp, err //nolint:wrapcheck // passthrough error from underlying transport
}
