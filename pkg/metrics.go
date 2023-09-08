package be

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var bridgeRequests = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "bridge_requests_total",
		Help: "Number of bridge requests, partitioned by status, asset, fromChain, bridgeTo",
	},
	[]string{"status", "asset", "fromChain", "bridgeTo"},
)

var bridgeRequestsDuration = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "bridge_requests_duration_seconds",
		Help: "Duration in seconds of each bridge request",
	},
	[]string{"asset", "fromChain", "bridgeTo"},
)

func BridgeRequestsInc(status string, awrr AccountWatchRequestResult) {
	bridgeRequests.WithLabelValues(
		status,
		awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency,
		awrr.AccountWatchRequest.Chain,
		awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo,
	).Inc()
}

func BridgeRequestsDurationSet(awrr AccountWatchRequestResult) {
	duration := time.Now().Sub(awrr.AccountWatchRequest.CreatedTime)

	bridgeRequestsDuration.WithLabelValues(
		awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency,
		awrr.AccountWatchRequest.Chain,
		awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo,
	).Set(duration.Seconds())
}
