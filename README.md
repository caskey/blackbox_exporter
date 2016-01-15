# Blackbox exporter

Another blackbox exporter allows blackbox probing of endpoints over
HTTP, HTTPS, TCP and ICMP.

This is a fork of the prometheus/blackbox_prober by caskey with improvements
to boost throughput and latency.

## Performance Tests

As a crude validation of throughput, apache's 'ab.exe' was used to drive 1,000,000 requests
to a local http server (mongoose-free-6.1.exe).  Here are the current results:



### Baseline (original forked code base from 2016-01-13)


    This is ApacheBench, Version 2.3 <$Revision: 1706008 $>
    Copyright 1996 Adam Twiss, Zeus Technology Ltd, http://www.zeustech.net/
    Licensed to The Apache Software Foundation, http://www.apache.org/
    
    Benchmarking localhost (be patient)
    Completed 100000 requests
    Completed 200000 requests
    Completed 300000 requests
    Completed 400000 requests
    Completed 500000 requests
    Completed 600000 requests
    Completed 700000 requests
    Completed 800000 requests
    Completed 900000 requests
    Completed 1000000 requests
    Finished 1000000 requests
    
    
    Server Software:
    Server Hostname:        localhost
    Server Port:            9115
    
    Document Path:          /probe?target=127.0.0.1:8080&module=http_2xx
    Document Length:        144 bytes
    
    Concurrency Level:      50
    Time taken for tests:   2989.406 seconds
    Complete requests:      1000000
    Failed requests:        17926
       (Connect: 0, Receive: 0, Length: 17926, Exceptions: 0)
    Total transferred:      261946222 bytes
    HTML transferred:       143946222 bytes
    Requests per second:    334.51 [#/sec] (mean)
    Time per request:       149.470 [ms] (mean)
    Time per request:       2.989 [ms] (mean, across all concurrent requests)
    Transfer rate:          85.57 [Kbytes/sec] received
    
    Connection Times (ms)
                  min  mean[+/-sd] median   max
    Connect:        0    1   0.3      1      33
    Processing:     4  148 301.8     22    1550
    Waiting:        4  148 301.7     22    1547
    Total:          5  149 301.8     23    1551
    
    Percentage of the requests served within a certain time (ms)
      50%     23
      66%     26
      75%     30
      80%     36
      90%    608
      95%   1030
      98%   1199
      99%   1223
     100%   1551 (longest request)


### Results as of 92bf4bc committed 2016-01-14

    This is ApacheBench, Version 2.3 <$Revision: 1706008 $>
    Copyright 1996 Adam Twiss, Zeus Technology Ltd, http://www.zeustech.net/
    Licensed to The Apache Software Foundation, http://www.apache.org/
    
    Benchmarking localhost (be patient)
    Completed 100000 requests
    Completed 200000 requests
    Completed 300000 requests
    Completed 400000 requests
    Completed 500000 requests
    Completed 600000 requests
    Completed 700000 requests
    Completed 800000 requests
    Completed 900000 requests
    Completed 1000000 requests
    Finished 1000000 requests
    
    
    Server Software:
    Server Hostname:        localhost
    Server Port:            9115
    
    Document Path:          /probe?target=127.0.0.1:8080&module=http_2xx
    Document Length:        224 bytes
    
    Concurrency Level:      50
    Time taken for tests:   1804.070 seconds
    Complete requests:      1000000
    Failed requests:        414874
       (Connect: 0, Receive: 0, Length: 414874, Exceptions: 0)
    Total transferred:      271471420 bytes
    HTML transferred:       153886294 bytes
    Requests per second:    554.30 [#/sec] (mean)
    Time per request:       90.204 [ms] (mean)
    Time per request:       1.804 [ms] (mean, across all concurrent requests)
    Transfer rate:          146.95 [Kbytes/sec] received
    
    Connection Times (ms)
                  min  mean[+/-sd] median   max
    Connect:        0    1   0.4      1      26
    Processing:     1   89  53.3     80    1550
    Waiting:        1   89  53.4     80    1549
    Total:          2   90  53.3     81    1552
    
    Percentage of the requests served within a certain time (ms)
      50%     81
      66%    100
      75%    112
      80%    123
      90%    154
      95%    186
      98%    228
      99%    262
     100%   1552 (longest request)


## Building and running

### Local Build

    go build
    ./blackbox_exporter <flags>

Visiting [http://localhost:9115/probe?target=google.com&module=http_2xx](http://localhost:9115/probe?target=google.com&module=http_2xx)
will return metrics for a HTTP probe against google.com.

## Configuration

A configuration showing all options is below:
```
modules:
  http_2xx:
    prober: http
    timeout: 5s
    http:
      valid_status_codes: []  # Defaults to 2xx
      method: GET
      no_follow_redirects: false
      fail_if_ssl: false
      fail_if_not_ssl: false
      fail_if_matches_regexp:
      - "Could not connect to database"
      fail_if_not_matches_regexp:
      - "Download the latest version here"
      path: /
  tcp_connect:
    prober: tcp
    timeout: 5s
  ssh_banner:
    prober: tcp
    timeout: 5s
    tcp:
      query_response:
      - expect: "^SSH-2.0-"
  irc_banner:
    prober: tcp
    timeout: 5s
    tcp:
      query_response:
      - send: "NICK prober"
      - send: "USER prober prober prober :prober"
      - expect: "PING :([^ ]+)"
        send: "PONG ${1}"
      - expect: "^:[^ ]+ 001"
  icmp:
    prober: icmp
    timeout: 5s
```

HTTP, HTTPS (via the `http` prober), TCP socket and ICMP (v4 only, requires privileged access) are currently supported.
Additional modules can be defined to meet your needs.


## Prometheus Configuration

The blackbox exporter needs to be passed the target as a parameter, this can be
done with relabelling.

Example config:
```
scrape_configs:
  - job_name: 'blackbox'
    metrics_path: /probe
    params:
      module: [http_2xx]  # Look for a HTTP 200 response.
    target_groups:
      - targets:
        - prometheus.io   # Target to probe
    relabel_configs:
      - source_labels: [__address__]
        regex: (.*)(:80)?
        target_label: __param_target
        replacement: ${1}
      - source_labels: [__param_target]
        regex: (.*)
        target_label: instance
        replacement: ${1}
      - source_labels: []
        regex: .*
        target_label: __address__
        replacement: 127.0.0.1:9115  # Blackbox exporter.
```
