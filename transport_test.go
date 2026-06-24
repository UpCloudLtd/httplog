package httplog_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/UpCloudLtd/httplog"
)

type uaRoundTripper struct {
	UserAgent string
}

func (rt *uaRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", rt.UserAgent)
	return http.DefaultTransport.RoundTrip(req) //nolint:wrapcheck // passthrough error from underlying transport
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClientRequestLogging(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		w.Header().Set("Date", "Fri, 11 Oct 2024 23:58:00 GMT")
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"method": "%s", "path": "%s", "body": "%s", "user-agent": "%s"}`, r.Method, r.URL.Path, base64.StdEncoding.EncodeToString(body), r.Header.Get("User-Agent")) //gosec:disable G705 -- test server handler uses only trusted request metadata
		}
	}))
	t.Cleanup(func() {
		srv.Close()
	})

	var output strings.Builder
	logFn := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Replace time with a static value
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue("2 Minutes to Midnight")
			}

			// Replace URL with a static value as port is random
			if a.Key == "url" {
				re := regexp.MustCompile(`127\.0\.0\.1:\d+`)
				a.Value = slog.StringValue(re.ReplaceAllString(a.Value.String(), "server"))
			}
			return a
		},
	})).DebugContext

	client := &http.Client{
		Transport: &httplog.LoggingTransport{
			Logger: httplog.NewLogger(logFn),
			Transport: &uaRoundTripper{
				UserAgent: "httplog-test/2026.06.01",
			},
		},
	}

	req, err := http.NewRequest("GET", srv.URL+"/test", nil)
	requireNoError(t, err)

	req.Header.Set("Authorization", "at_1234567890abcdef")
	_, err = client.Do(req)
	requireNoError(t, err)

	req, err = http.NewRequest("POST", srv.URL+"/test", io.NopCloser(bytes.NewReader([]byte(`{"name": "test"}`))))
	requireNoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer at_1234567890abcdef")
	_, err = client.Do(req)
	requireNoError(t, err)

	req, err = http.NewRequest("DELETE", srv.URL+"/test", nil)
	requireNoError(t, err)

	req.Header.Set("Authorization", "Basic dXNlcm5hbWU6cGFzc3dvcmQK")
	_, err = client.Do(req)
	requireNoError(t, err)

	expected := `{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Sending request to 127.0.0.1","url":"http://server/test","method":"GET","headers":{"Authorization":["[REDACTED]"]},"body":""}
{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Received response from 127.0.0.1","url":"http://server/test","status":"200 OK","headers":{"Content-Length":["87"],"Content-Type":["application/json"],"Date":["Fri, 11 Oct 2024 23:58:00 GMT"]},"body":"{\n  \"method\": \"GET\",\n  \"path\": \"/test\",\n  \"body\": \"\",\n  \"user-agent\": \"httplog-test/2026.06.01\"\n}"}
{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Sending request to 127.0.0.1","url":"http://server/test","method":"POST","headers":{"Authorization":["Bearer [REDACTED]"],"Content-Type":["application/json"]},"body":"{\n  \"name\": \"test\"\n}"}
{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Received response from 127.0.0.1","url":"http://server/test","status":"200 OK","headers":{"Content-Length":["112"],"Content-Type":["application/json"],"Date":["Fri, 11 Oct 2024 23:58:00 GMT"]},"body":"{\n  \"method\": \"POST\",\n  \"path\": \"/test\",\n  \"body\": \"eyJuYW1lIjogInRlc3QifQ==\",\n  \"user-agent\": \"httplog-test/2026.06.01\"\n}"}
{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Sending request to 127.0.0.1","url":"http://server/test","method":"DELETE","headers":{"Authorization":["Basic [REDACTED]"]},"body":""}
{"time":"2 Minutes to Midnight","level":"DEBUG","msg":"Received response from 127.0.0.1","url":"http://server/test","status":"204 No Content","headers":{"Date":["Fri, 11 Oct 2024 23:58:00 GMT"]},"body":""}
`
	if output.String() != expected {
		t.Errorf("output does not match expected output\n\nExpected:\n\n%s\n\nGot:\n\n%s\n", expected, output.String())
	}
}
