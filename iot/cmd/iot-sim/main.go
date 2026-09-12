// Command iot-sim runs a fleet of virtual classrooms against a real broker.
// It is the development stand-in for hardware and the executable sample of
// iot/spec/spec.md: publish canonical state, inject occasional alarms, answer
// commands with acks — everything a driver must do.
//
// Typical use with the dev overlay:
//
//	docker compose -f docker-compose.yml -f docker-compose.iot.yml up -d mosquitto
//	go run ./cmd/iot-sim --broker tcp://localhost:1883 --classrooms 2
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ocm-iot/mqttc"
	"ocm-iot/sim"
)

func main() {
	var (
		broker     = flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
		username   = flag.String("username", "", "broker username")
		password   = flag.String("password", "", "broker password")
		site       = flag.String("site", "main", "topic site segment")
		source     = flag.String("source", "sim01", "topic sourceId segment (one credential)")
		classrooms = flag.Int("classrooms", 1, "number of virtual classrooms")
		prefix     = flag.String("prefix", "C", "classroom name prefix")
		start      = flag.Int("start", 301, "first classroom number")
		interval   = flag.Duration("interval", 5*time.Second, "state publish interval")
		alarmP     = flag.Float64("alarm", 0.02, "per-classroom per-tick alarm probability")
		buffer     = flag.String("buffer", "", "event/ack offline buffer file (empty = memory only)")
	)
	flag.Parse()

	slog.Info("OCM IoT simulator", "broker", *broker, "site", *site, "source", *source,
		"classrooms", *classrooms, "interval", *interval)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := mqttc.Connect(ctx, mqttc.Options{
		BrokerURL:  *broker,
		Username:   *username,
		Password:   *password,
		Site:       *site,
		SourceID:   *source,
		BufferPath: *buffer,
	})
	if err != nil {
		slog.Error("iot-sim", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := sim.Run(ctx, client, sim.Options{
		Classrooms: *classrooms,
		Prefix:     *prefix,
		StartIndex: *start,
		Interval:   *interval,
		AlarmP:     *alarmP,
	}, slog.Default()); err != nil {
		slog.Error("iot-sim", "err", err)
		os.Exit(1)
	}
}
