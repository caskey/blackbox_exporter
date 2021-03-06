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
		metrics := NewMetricSink()
		defer close(metrics)
		result := probeHTTP(ts.URL,
			Module{HTTP: HTTPProbe{ValidStatusCodes: test.ValidStatusCodes}}, metrics)
		if result != test.ShouldSucceed {
			t.Fatalf("Test %d (status code %d) expected result %t, got %t", i, test.StatusCode, test.ShouldSucceed, result)
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

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL, Module{HTTP: HTTPProbe{Path: pathToSend}}, metrics)
	if !result {
		t.Error()
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
	metrics := make(chan Metric)
	defer close(metrics)
	go func() {
		var redirectMetricFound = false
		for m := range metrics {
			if m.Name == "probe_http_redirects" {
				if m.FloatValue != 1.0 {
					t.Fatalf("Unexpected number of redirects found: %f", m.FloatValue)
				}
			}
		}
		if !redirectMetricFound {
			t.Fatalf("Redirect count metric not found.")
		}
	}()

	result := probeHTTP(ts.URL, Module{HTTP: HTTPProbe{}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/noredirect", http.StatusFound)
	}))
	defer ts.Close()

	// Follow redirect, should succeed with 200.
	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{NoFollowRedirects: true, ValidStatusCodes: []int{302}}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestPost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{Method: "POST"}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestFailIfNotSSL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	metrics := make(chan Metric)
	defer close(metrics)
	go func() {
		for m := range metrics {
			if m.Name == "probe_http_ssl" && m.FloatValue > 0 {
				t.Fatalf("Did not expect ssl metric set on non-ssl connection")
			}
		}
	}()
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfNotSSL: true}}, metrics)
	if result {
		t.Fail()
	}
}

func TestFailIfMatchesRegexpShouldFailOnMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string in the body"}}}, metrics)
	if result {
		t.Fail()
	}
}

func TestFailIfMatchesRegexpShouldNotFailOnNoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body"}}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestFailIfMatchesRegexpShouldFailOnAnyMatch(t *testing.T) {
	// With multiple regexps configured, verify that any matching regexp causes
	// the probe to fail, but probes succeed when no regexp matches.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body", "string in the body"}}}, metrics)
	if result {
		t.Fail()
	}
}

func TestFailIfMatchesRegexpShouldNotFailOnNoMatches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfMatchesRegexp: []string{"string NOT in the body", "string also NOT in the body"}}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestFailIfNotMatchesRegexpShouldFailOnNoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string NOT in the body"}}}, metrics)
	if result {
		t.Fail()
	}
}

func TestFailIfNotMatchesRegexpShouldNotFailOnMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the body"}}}, metrics)
	if !result {
		t.Fail()
	}
}

func TestFailIfNotMatchesRegexpShouldFailOnAnyNonMatches(t *testing.T) {
	// With multiple regexps configured, verify that any non-matching regexp
	// causes the probe to fail, but probes succeed when all regexps match.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the body", "string NOT in the body"}}}, metrics)
	if result {
		t.Fail()
	}
}

func TestFailIfNotMatchesRegexpShouldNotFailOnAllMatches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "A string in the body of the http response.")
	}))
	defer ts.Close()

	metrics := NewMetricSink()
	defer close(metrics)
	result := probeHTTP(ts.URL,
		Module{HTTP: HTTPProbe{FailIfNotMatchesRegexp: []string{"string in the", "body of the"}}}, metrics)
	if !result {
		t.Fail()
	}
}
