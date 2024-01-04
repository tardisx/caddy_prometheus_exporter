package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type CaddyLogline struct {
	Level   string  `json:"level"`
	Ts      float64 `json:"ts"`
	Logger  string  `json:"logger"`
	Msg     string  `json:"msg"`
	Request struct {
		RemoteIP   string `json:"remote_ip"`
		RemotePort string `json:"remote_port"`
		ClientIP   string `json:"client_ip"`
		Proto      string `json:"proto"`
		Method     string `json:"method"`
		Host       string `json:"host"`
		URI        string `json:"uri"`
		Headers    struct {
			SecFetchSite            []string `json:"Sec-Fetch-Site"`
			Accept                  []string `json:"Accept"`
			AcceptLanguage          []string `json:"Accept-Language"`
			Connection              []string `json:"Connection"`
			UpgradeInsecureRequests []string `json:"Upgrade-Insecure-Requests"`
			SecFetchMode            []string `json:"Sec-Fetch-Mode"`
			UserAgent               []string `json:"User-Agent"`
			SecFetchDest            []string `json:"Sec-Fetch-Dest"`
			AcceptEncoding          []string `json:"Accept-Encoding"`
		} `json:"headers"`
	} `json:"request"`
	BytesRead   int     `json:"bytes_read"`
	UserID      string  `json:"user_id"`
	Duration    float64 `json:"duration"`
	Size        int     `json:"size"`
	Status      int     `json:"status"`
	RespHeaders struct {
		Server        []string `json:"Server"`
		Etag          []string `json:"Etag"`
		ContentType   []string `json:"Content-Type"`
		LastModified  []string `json:"Last-Modified"`
		AcceptRanges  []string `json:"Accept-Ranges"`
		ContentLength []string `json:"Content-Length"`
	} `json:"resp_headers"`
}

func main() {
	requestsCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "",
		Subsystem:   "",
		Name:        "requests",
		Help:        "",
		ConstLabels: map[string]string{},
	}, []string{"method", "status_code", "host"})

	requestsDuration := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "",
		Subsystem:   "",
		Name:        "requests_duration",
		Help:        "",
		ConstLabels: map[string]string{},
	}, []string{"method", "status_code", "host"})

	requestsSize := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "",
		Subsystem:   "",
		Name:        "requests_size",
		Help:        "",
		ConstLabels: map[string]string{},
	}, []string{"method", "status_code", "host"})

	prometheus.MustRegister(requestsCounter, requestsDuration, requestsSize)

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe("127.0.0.1:8191", nil))
	}()

	i := len(os.Args)
	if i < 2 {
		panic("need names")
	}
	agg := make(chan *tail.Line)

	for _, fn := range os.Args[1:] {
		slog.With("file", fn).Info("opening")
		t, err := tail.TailFile(fn, tail.Config{
			Follow:        true,
			ReOpen:        true,
			CompleteLines: true,
			Location: &tail.SeekInfo{
				Offset: 0,
				Whence: io.SeekEnd,
			},
		})
		if err != nil {
			panic(err)
		}
		go func(c chan *tail.Line) {
			for msg := range c {
				agg <- msg
			}
		}(t.Lines)
	}

	for line := range agg {
		js := CaddyLogline{}
		err := json.Unmarshal([]byte(line.Text), &js)
		if err != nil {
			slog.With(err).Error("could not unmarshal")
			continue
		}

		if js.Msg != "handled request" {
			continue
		}

		requestsCounter.WithLabelValues(js.Request.Method, fmt.Sprint(js.Status), js.Request.Host).Inc()
		requestsDuration.WithLabelValues(js.Request.Method, fmt.Sprint(js.Status), js.Request.Host).Add(js.Duration)
		requestsSize.WithLabelValues(js.Request.Method, fmt.Sprint(js.Status), js.Request.Host).Add(float64(js.Size))

	}
}
