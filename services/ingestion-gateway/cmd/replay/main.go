package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"log/slog"
	"teampulsebridge/services/ingestion-gateway/internal/apperr"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/replay"
)

func main() {
	var (
		filePath       string
		eventID        string
		sourceOverride string
		dryRun         bool
		timeout        time.Duration
		headerFlags    kvFlags
	)

	flag.StringVar(&filePath, "file", "", "Path to replay input JSON file")
	flag.StringVar(&eventID, "event-id", "", "Failed-event ID to replay from failed event store")
	flag.StringVar(&sourceOverride, "source", "", "Source override (required for raw payload replay)")
	flag.BoolVar(&dryRun, "dry-run", false, "Validate input and print summary without publishing")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "Publish timeout")
	flag.Var(&headerFlags, "header", "Header override in the form Key=Value (repeatable)")
	flag.Parse()

	filePath = strings.TrimSpace(filePath)
	eventID = strings.TrimSpace(eventID)
	if (filePath == "" && eventID == "") || (filePath != "" && eventID != "") {
		exitf("provide exactly one input source: -file or -event-id")
	}
	if timeout <= 0 {
		exitf("timeout must be > 0")
	}

	cfg := config.LoadFromEnv()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var (
		event replay.Event
		err   error
	)
	switch {
	case filePath != "":
		event, err = loadEventFromFile(filePath, sourceOverride, map[string]string(headerFlags))
	case eventID != "":
		event, err = loadEventFromStore(cfg, eventID, map[string]string(headerFlags))
	}
	if err != nil {
		exitWithAppError(err)
	}

	if dryRun {
		fmt.Printf("dry-run ok: source=%s bytes=%d headers=%d\n", event.Source, len(event.Body), len(event.Headers))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pub, err := buildPublisher(ctx, cfg, logger)
	if err != nil {
		exitWithAppError(apperr.New("cmd.replay.buildPublisher", apperr.CodeReplayConfigInvalid, "build publisher failed", err))
	}
	defer func() {
		_ = pub.Close()
	}()

	if err := pub.Publish(ctx, event.Source, event.Body, event.Headers); err != nil {
		exitWithAppError(apperr.New("cmd.replay.publish", apperr.CodeReplayPublishFailed, "publish replay event failed", err))
	}

	logger.Info("replay published",
		"source", event.Source,
		"bytes", len(event.Body),
		"headers", len(event.Headers),
		"backend", cfg.QueueBackend,
	)
}

func loadEventFromFile(path, sourceOverride string, headerOverrides map[string]string) (replay.Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return replay.Event{}, apperr.New("cmd.replay.loadFile", apperr.CodeReplayReadFailed, "read replay file failed", err)
	}
	event, err := replay.ParseInput(raw, sourceOverride, headerOverrides)
	if err != nil {
		return replay.Event{}, apperr.New("cmd.replay.loadFile", apperr.CodeReplayInputInvalid, "invalid replay input", err)
	}
	return event, nil
}

func loadEventFromStore(cfg config.Config, eventID string, headerOverrides map[string]string) (replay.Event, error) {
	path := strings.TrimSpace(cfg.FailedStorePath)
	if path == "" {
		path = "data/failed-events.jsonl"
	}
	store, err := failstore.NewFileStore(path)
	if err != nil {
		return replay.Event{}, apperr.New("cmd.replay.loadStore", apperr.CodeReplayConfigInvalid, "invalid failed event store configuration", err)
	}

	record, err := store.GetByID(context.Background(), eventID)
	if errors.Is(err, failstore.ErrNotFound) {
		return replay.Event{}, apperr.New("cmd.replay.loadStore", apperr.CodeReplayEventNotFound, "failed event not found", err)
	}
	if err != nil {
		return replay.Event{}, apperr.New("cmd.replay.loadStore", apperr.CodeReplayReadFailed, "read failed event failed", err)
	}

	event := replay.Event{
		Source:  record.Source,
		Headers: record.Headers,
		Body:    append([]byte(nil), record.Body...),
	}
	if event.Headers == nil {
		event.Headers = make(map[string]string)
	}
	for k, v := range headerOverrides {
		event.Headers[k] = v
	}
	return event, nil
}

func buildPublisher(ctx context.Context, cfg config.Config, logger *slog.Logger) (queue.Publisher, error) {
	switch cfg.QueueBackend {
	case "", "log":
		return queue.NewLogPublisher(logger), nil
	case "pubsub":
		if strings.TrimSpace(cfg.PubSubProjectID) == "" || strings.TrimSpace(cfg.PubSubTopicID) == "" {
			return nil, fmt.Errorf("pubsub replay requires PUBSUB_PROJECT_ID and PUBSUB_TOPIC_ID")
		}
		return queue.NewPubSubPublisher(ctx, cfg.PubSubProjectID, cfg.PubSubTopicID, logger)
	default:
		return nil, fmt.Errorf("unsupported queue backend %q", cfg.QueueBackend)
	}
}

type kvFlags map[string]string

func (f *kvFlags) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", map[string]string(*f))
}

func (f *kvFlags) Set(value string) error {
	if *f == nil {
		*f = make(map[string]string)
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid header %q: expected Key=Value", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("invalid header %q: key must not be empty", value)
	}
	(*f)[key] = parts[1]
	return nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "replay: "+format+"\n", args...)
	os.Exit(1)
}

func exitWithAppError(err error) {
	code := apperr.CodeOf(err)
	if code == "" {
		exitf("%v", err)
	}
	fmt.Fprintf(os.Stderr, "replay: [%s] %v\n", code, err)
	os.Exit(1)
}
