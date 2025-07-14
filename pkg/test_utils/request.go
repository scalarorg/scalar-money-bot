package testutils

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/labstack/echo/v4"
)

type RequestOption struct {
	Method      string
	URL         string
	Body        interface{}
	QueryParams map[string]string
	ContentType string
}

func Request(opts *RequestOption) (*http.Request, *httptest.ResponseRecorder) {
	var buff bytes.Buffer
	err := json.NewEncoder(&buff).Encode(opts.Body)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(opts.Method, opts.URL, &buff)
	if opts.ContentType != "" {
		req.Header.Set(echo.HeaderContentType, opts.ContentType)
	} else {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if opts.QueryParams != nil {
		q := req.URL.Query()
		for key, val := range opts.QueryParams {
			q.Add(key, val)
		}
		req.URL.RawQuery = q.Encode()
	}
	rec := httptest.NewRecorder()
	return req, rec
}
