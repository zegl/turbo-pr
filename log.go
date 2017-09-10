package main

import (
	"context"
	"log"

	"cloud.google.com/go/logging"
	"golang.org/x/oauth2/google"
)

func logger(event string, data interface{}) {
	ctx := context.Background()

	_, err := google.DefaultClient(ctx, logging.WriteScope)
	if err != nil {
		log.Println(err)
		return
	}

	// Creates a client.
	client, err := logging.NewClient(ctx, *flagGoogleCloudProjectID)
	if err != nil {
		log.Println(err)
		return
	}

	// Selects the log to write to.
	logger := client.Logger("turbo-pr-access")

	// Adds an entry to the log buffer.
	logger.Log(logging.Entry{
		Payload: data,
		Labels: map[string]string{
			"github-event": event,
		},
	})

	// Closes the client and flushes the buffer to the Stackdriver Logging
	// service.
	if err := client.Close(); err != nil {
		log.Fatalf("Failed to close client: %v", err)
	}
}
