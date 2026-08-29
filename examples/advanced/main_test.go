package main

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = original
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(out)
}

func TestMainExampleRuns(t *testing.T) {
	originalLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(originalLogOutput)
	})

	output := captureStdout(t, main)
	for _, want := range []string{
		"=== Advanced Error Handling and Logging Example ===",
		"Service A error:",
		"Service B error: [CODE:500]",
		"Type conversion error in Service A:",
		"Type conversion error in Service B: [CODE:400]",
		"Service A: analytics-service, Retries: 5",
		"Service B: notification-service, Log Level: debug",
		"Legacy config service name: advanced-service",
		"Legacy config uses global logger: false",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("main output = %q, want substring %q", output, want)
		}
	}
}

func TestCustomLoggerAndErrorWrapper(t *testing.T) {
	var logs strings.Builder
	originalLogOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(originalLogOutput)
	})

	logger := NewCustomLogger("TEST")
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	for _, want := range []string{"[TEST DEBUG] debug", "[TEST INFO] info", "[TEST WARN] warn", "[TEST ERROR] error"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs = %q, want substring %q", logs.String(), want)
		}
	}

	if got := CustomErrorWrapper(errors.New("boom"), 123, ""); !strings.Contains(got.Error(), "[CODE:123] boom") {
		t.Fatalf("CustomErrorWrapper(empty msg) = %q, want code and cause", got.Error())
	}
}
