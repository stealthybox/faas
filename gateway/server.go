package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	faasHandlers "github.com/alexellis/faas/gateway/handlers"
	"github.com/alexellis/faas/gateway/metrics"
	"github.com/docker/docker/client"

	"fmt"

	"github.com/gorilla/mux"
)

func main() {
	logger := logrus.Logger{}
	logrus.SetFormatter(&logrus.TextFormatter{})

	var dockerClient *client.Client
	var err error
	dockerClient, err = client.NewEnvClient()
	if err != nil {
		log.Fatal("Error with Docker client.")
	}
	dockerVersion, err := dockerClient.ServerVersion(context.Background())
	if err != nil {
		log.Fatal("Error with Docker server.\n", err)
	}
	log.Printf("API version: %s, %s\n", dockerVersion.APIVersion, dockerVersion.Version)

	metricsOptions := metrics.BuildMetricsOptions()
	metrics.RegisterMetrics(metricsOptions)

	r := mux.NewRouter()
	// r.StrictSlash(false)	// This didn't work, so register routes twice.

	functionHandler := faasHandlers.MakeProxy(metricsOptions, true, dockerClient, &logger)
	r.HandleFunc("/function/{name:[-a-zA-Z_0-9]+}", functionHandler)
	r.HandleFunc("/function/{name:[-a-zA-Z_0-9]+}/", functionHandler)

	r.HandleFunc("/system/alert", faasHandlers.MakeAlertHandler(dockerClient))
	r.HandleFunc("/system/functions", faasHandlers.MakeFunctionReader(metricsOptions, dockerClient)).Methods("GET")
	r.HandleFunc("/system/functions", faasHandlers.MakeNewFunctionHandler(metricsOptions, dockerClient)).Methods("POST")
	r.HandleFunc("/system/functions", faasHandlers.MakeDeleteFunctionHandler(metricsOptions, dockerClient)).Methods("DELETE")

	fs := http.FileServer(http.Dir("./assets/"))
	r.PathPrefix("/ui/").Handler(http.StripPrefix("/ui", fs)).Methods("GET")

	r.HandleFunc("/", faasHandlers.MakeProxy(metricsOptions, false, dockerClient, &logger)).Methods("POST")

	metricsHandler := metrics.PrometheusHandler()
	r.Handle("/metrics", metricsHandler)

	// This could exist in a separate process - records the replicas of each swarm service.
	functionLabel := "function"
	metrics.AttachSwarmWatcher(dockerClient, metricsOptions, functionLabel)

	r.Handle("/", http.RedirectHandler("/ui/", http.StatusMovedPermanently)).Methods("GET")

	readTimeout := 8 * time.Second
	writeTimeout := 8 * time.Second
	tcpPort := 8080

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", tcpPort),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes, // 1MB - can be overridden by setting Server.MaxHeaderBytes.
		Handler:        r,
	}

	log.Fatal(s.ListenAndServe())
}
