package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nduyhai/gopulse"
	"github.com/nduyhai/gopulse/healths"
	"log"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	aggregator := gopulse.NewHealthAggregator(ctx,
		gopulse.WithAutoUpdate(10*time.Second),   // Check every 10 seconds
		gopulse.WithInitialDelay(2*time.Second),  // Wait 2 seconds before first check
		gopulse.WithBackoff(60*time.Second, 1.5), // Max backoff 60s, factor 1.5
		gopulse.WithExpiryTime(30*time.Second),
		gopulse.WithUpdateBuffer(100),
		gopulse.WithStatusChangeCallback(func(name string, status *gopulse.HealthStatus) {
			log.Printf("Health status changed for %s: Liveness=%v, Readiness=%v",
				name, status.Liveness, status.Readiness)
		}),
	)

	// Start the aggregator (auto-updates will begin)
	aggregator.Start()
	defer aggregator.Stop()

	// Register health checks
	noop := &healths.Noop{}
	aggregator.RegisterHealthCheck(noop, gopulse.PriorityCritical)

	down := &healths.Down{}
	aggregator.RegisterHealthCheck(down, gopulse.PriorityCritical)

	// Register the handler function for the root path "/"
	http.HandleFunc("/readiness", func(w http.ResponseWriter, r *http.Request) {
		readiness, errors := aggregator.GetReadiness()
		w.WriteHeader(http.StatusOK)
		if readiness {
			strB, _ := json.Marshal(gopulse.NewUpStatus())
			_, _ = w.Write(strB)
			return
		}
		strB, _ := json.Marshal(gopulse.NewDownStatus(errors))
		_, _ = w.Write(strB)
		return

	})

	// Register the handler function for the root path "/"
	http.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {
		readiness, errors := aggregator.GetLiveness()
		w.WriteHeader(http.StatusOK)
		if readiness {
			strB, _ := json.Marshal(gopulse.NewUpStatus())
			_, _ = w.Write(strB)
			return
		}

		strB, _ := json.Marshal(gopulse.NewDownStatus(errors))
		_, _ = w.Write(strB)
		return

	})

	// Start the server on port 8080
	fmt.Println("Server starting on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}

}
