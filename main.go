package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

var (
	addr       = flag.String("web.listen-address", ":9115", "The address to listen on for HTTP requests.")
	configFile = flag.String("config.file", "blackbox.yml", "Blackbox exporter configuration file.")
)

var (
	probeLatencies = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "probe_latency_summary_millis",
			Help: "Latency of probes by type",
		},
		[]string{"module", "success"},
	)
	probeHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "probe_latency_histogram_millis",
			Help:    "Latency of probes by type",
			Buckets: prometheus.ExponentialBuckets(1, 2, 20),
		},
		[]string{"module", "success"},
	)
	probeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "probe_count",
			Help: "Latency of probes by type",
		},
		[]string{"module", "success"},
	)
)

func init() {
	prometheus.MustRegister(probeLatencies)
	prometheus.MustRegister(probeHistogram)
	prometheus.MustRegister(probeCounter)
}

type Config struct {
	Modules map[string]Module `yaml:"modules"`
}

type Module struct {
	Prober  string        `yaml:"prober"`
	Timeout time.Duration `yaml:"timeout"`
	HTTP    HTTPProbe     `yaml:"http"`
	TCP     TCPProbe      `yaml:"tcp"`
	ICMP    ICMPProbe     `yaml:"icmp"`
}

type HTTPProbe struct {
	// Defaults to 2xx.
	ValidStatusCodes       []int    `yaml:"valid_status_codes"`
	NoFollowRedirects      bool     `yaml:"no_follow_redirects"`
	FailIfSSL              bool     `yaml:"fail_if_ssl"`
	FailIfNotSSL           bool     `yaml:"fail_if_not_ssl"`
	Method                 string   `yaml:"method"`
	FailIfMatchesRegexp    []string `yaml:"fail_if_matches_regexp"`
	FailIfNotMatchesRegexp []string `yaml:"fail_if_not_matches_regexp"`
	Path                   string   `yaml:"path"`
}

type QueryResponse struct {
	Expect string `yaml:"expect"`
	Send   string `yaml:"send"`
}

type TCPProbe struct {
	QueryResponse []QueryResponse `yaml:"query_response"`
}

type ICMPProbe struct {
}

type Metric struct {
	Name       string
	FloatValue float64
}

var Probers = map[string]func(string, Module, chan<- Metric) bool{
	"http": probeHTTP,
	"tcp":  probeTCP,
	"icmp": probeICMP,
}

func probeHandler(w http.ResponseWriter, r *http.Request, config *Config) {
	params := r.URL.Query()
	target := params.Get("target")
	moduleName := params.Get("module")
	if target == "" {
		http.Error(w, "Target parameter is missing", 400)
		return
	}
	if moduleName == "" {
		moduleName = "http2xx"
	}
	module, ok := config.Modules[moduleName]
	if !ok {
		http.Error(w, fmt.Sprintf("Unkown module %s", moduleName), 400)
		return
	}
	prober, ok := Probers[module.Prober]
	if !ok {
		http.Error(w, fmt.Sprintf("Unkown prober %s", module.Prober), 400)
		return
	}

	// Warning: magic number here.  This must be big enough to collect all the metrics.
	metrics := make(chan Metric, 30)

	start := time.Now()
	success := prober(target, module, metrics)
	latency := float64(time.Now().Sub(start).Nanoseconds()) / 1e6

	metrics <- Metric{"probe_duration_seconds", latency / 1e3}
	var successString string
	if success {
		metrics <- Metric{"probe_success", 1}
		successString = "true"
	} else {
		metrics <- Metric{"probe_success", 0}
		successString = "false"
	}

	// Close the metric buffer and dump it.
	close(metrics)
	for metric := range metrics {
		fmt.Fprintf(w, "%s %f\n", metric.Name, metric.FloatValue)
	}

	probeLatencies.WithLabelValues(moduleName, successString).Observe(latency)
	probeHistogram.WithLabelValues(moduleName, successString).Observe(latency)
	probeCounter.WithLabelValues(moduleName, successString).Inc()
}

func main() {
	flag.Parse()

	yamlFile, err := ioutil.ReadFile(*configFile)

	if err != nil {
		log.Fatalf("Error reading config file: %s", err)
	}

	config := Config{}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %s", err)
	}
	log.Infof("Configuration loaded from: %s", *configFile)

	http.Handle("/metrics", prometheus.Handler())
	http.HandleFunc("/probe",
		func(w http.ResponseWriter, r *http.Request) {
			probeHandler(w, r, &config)
		})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
            <head><title>Blackbox Exporter</title></head>
            <body>
            <h1>Blackbox Exporter</h1>
            <p><a href="/probe?target=prometheus.io&module=http_2xx">Probe prometheus.io for http_2xx</a></p>
            <p><a href="/metrics">Metrics</a></p>
            </body>
            </html>`))
	})
	log.Infof("Listening for connections on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %s", err)
	}
}
