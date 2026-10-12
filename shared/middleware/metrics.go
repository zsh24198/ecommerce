package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var httpRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	},
	[]string{"path", "method", "status"},
)

var httpDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	},
	[]string{"path", "method"},
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpDuration)
}

func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.FullPath() == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		defer func() {
			path := c.FullPath()
			if path == "" {
				path = "UNMATCHED"
			}
			status := strconv.Itoa(c.Writer.Status())
			httpRequestsTotal.WithLabelValues(path, c.Request.Method, status).Inc()
			httpDuration.WithLabelValues(path, c.Request.Method).Observe(time.Since(start).Seconds())
		}()
		c.Next()
	}
}
