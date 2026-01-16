package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CurrentHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "etherflow_current_height",
		Help: "The current block number being processed by the indexer",
	})

	ChainHead = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "etherflow_chain_head",
		Help: "The latest block number available from the RPC source",
	})

	ReorgCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "etherflow_reorg_count",
		Help: "Total number of chain reorganizations detected",
	})

	ProcessingLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "etherflow_processing_lag",
		Help: "Difference between chain head and current processed height",
	})
)
