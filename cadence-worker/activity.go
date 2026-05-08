package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/cadence/activity"
	"go.uber.org/zap"
)

// HTTPCallActivity ส่ง HTTP GET ไปยัง URL ที่กำหนด แล้วคืน status code
func HTTPCallActivity(ctx context.Context, url string) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("HTTPCallActivity starting", zap.String("url", url))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("http call failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	result := fmt.Sprintf("status=%d body_preview=%s", resp.StatusCode, string(body))
	logger.Info("HTTPCallActivity done", zap.String("result", result))
	return result, nil
}

// DBWriteActivity จำลองการเขียนข้อมูลลง database
func DBWriteActivity(ctx context.Context, key string, value string) error {
	logger := activity.GetLogger(ctx)
	logger.Info("DBWriteActivity starting",
		zap.String("key", key),
		zap.String("value", value),
	)

	// จำลอง DB write latency
	time.Sleep(100 * time.Millisecond)

	logger.Info("DBWriteActivity done", zap.String("key", key))
	return nil
}
