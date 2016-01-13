// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPStatusCodes(t *testing.T) {
	tests := []struct {
		StatusCode       int
		ValidStatusCodes []int
		ShouldSucceed    bool
	}{
		{200, []int{}, true},
		{201, []int{}, true},
		{299, []int{}, true},
		{300, []int{}, false},
		{404, []int{}, false},
		{404, []int{200, 404}, true},
		{200, []int{200, 404}, true},
		{201, []int{200, 404}, false},
		{404, []int{404}, true},
		{200, []int{404}, false},
	}

	for i, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.StatusCode)
		}))
		defer ts.Close()
		recorder := httptest.NewRecorder()
		result := probeHTTP(ts.URL, recorder,
			Module{HTTP: HTTPProbe{ValidStatusCodes: test.ValidStatusCodes}})
		body := recorder.Body.String()
		if result != test.ShouldSucceed {
			t.Fatalf("Test %d had unexpected result: %s", i, body)
		}
	}
}

func TestConfiguredPathSentInRequest(t *testing.T) {
	var pathToSend = "/path/to/send?query=string"
	var pathFound string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathFound = r.URL.Path
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder, Module{HTTP: HTTPProbe{Path: pathToSend}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Fetch test failed unexpectedly, got %s", body)
	}
	// The path parameter received by the server should be the first part of the path+query string.
	if strings.Index(pathToSend, pathFound) != 0 {
		t.Fatalf("Path received was %s, expected %s", pathFound, pathToSend)
	}
}

func TestRedirectFollowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/noredirect", http.StatusFound)
		}
	}))
	defer ts.Close()

	// Follow redirect, should succeed with 200.
	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder, Module{HTTP: HTTPProbe{}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Redirect test failed unexpectedly, got %s", body)
	}
	if !strings.Contains(body, "probe_http_redirects 1\n") {
		t.Fatalf("Expected one redirect, got %s", body)
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/noredirect", http.StatusFound)
	}))
	defer ts.Close()

	// Follow redirect, should succeed with 200.
	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{NoFollowRedirects: true, ValidStatusCodes: []int{302}}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Redirect test failed unexpectedly, got %s", body)
	}
}

func TestPost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{Method: "POST"}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Post test failed unexpectedly, got %s", body)
	}
}

func TestFailIfNotSSL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfNotSSL: true}})
	body := recorder.Body.String()
	if result {
		t.Fatalf("Fail if not SSL test suceeded unexpectedly, got %s", body)
	}
	if !strings.Contains(body, "probe_http_ssl 0\n") {
		t.Fatalf("Expected HTTP without SSL, got %s", body)
	}
}

func TestFailIfMatchesRegexpShouldFailOnMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string in the body"}}})
	body := recorder.Body.String()
	if result {
		t.Fatalf("Regexp test succeeded unexpectedly, got %s", body)
	}
}

func TestFailIfMatchesRegexpShouldNotFailOnNoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body"}}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Regexp test failed unexpectedly, got %s", body)
	}
}

func TestFailIfMatchesRegexpShouldFailOnAnyMatch(t *testing.T) {
	// With multiple regexps configured, verify that any matching regexp causes
	// the probe to fail, but probes succeed when no regexp matches.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body", "string in the body"}}})
	body := recorder.Body.String()
	if result {
		t.Fatalf("Regexp test succeeded unexpectedly, got %s", body)
	}
}

func TestFailIfMatchesRegexpShouldNotFailOnNoMatches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body", "string also NOT in the body"}}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Regexp test failed unexpectedly, got %s", body)
	}
}

func TestFailIfNotMatchesRegexpShouldFailOnNoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string NOT in the body"}}})
	body := recorder.Body.String()
	if result {
		t.Fatalf("Regexp test succeeded unexpectedly, got %s", body)
	}
}

func TestFailIfNotMatchesRegexpShouldNotFailOnMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the body"}}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Regexp test failed unexpectedly, got %s", body)
	}
}

func TestFailIfNotMatchesRegexpShouldFailOnAnyNonMatches(t *testing.T) {
	// With multiple regexps configured, verify that any non-matching regexp
	// causes the probe to fail, but probes succeed when all regexps match.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the body", "string NOT in the body"}}})
	body := recorder.Body.String()
	if result {
		t.Fatalf("Regexp test succeeded unexpectedly, got %s", body)
	}
}

func TestFailIfNotMatchesRegexpShouldNotFailOnAllMatches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	recorder := httptest.NewRecorder()
	result := probeHTTP(ts.URL, recorder,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the", "body of the"}}})
	body := recorder.Body.String()
	if !result {
		t.Fatalf("Regexp test failed unexpectedly, got %s", body)
	}
}
