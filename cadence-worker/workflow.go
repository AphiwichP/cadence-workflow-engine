package main

import (
	"time"

	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
)

// PlatformWorkflowInput คือ input ที่รับเข้ามาตอน start workflow
type PlatformWorkflowInput struct {
	URL      string
	DBKey    string
	DBValue  string
}

// PlatformWorkflow orchestrate HTTPCallActivity แล้วตามด้วย DBWriteActivity
func PlatformWorkflow(ctx workflow.Context, input PlatformWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("PlatformWorkflow started", zap.String("url", input.URL))

	activityOpts := workflow.ActivityOptions{
		ScheduleToStartTimeout: 1 * time.Minute,
		StartToCloseTimeout:    30 * time.Second,
		HeartbeatTimeout:       10 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOpts)

	// Step 1: HTTP call
	var httpResult string
	if err := workflow.ExecuteActivity(ctx, HTTPCallActivity, input.URL).Get(ctx, &httpResult); err != nil {
		logger.Error("HTTPCallActivity failed", zap.Error(err))
		return err
	}
	logger.Info("HTTP call complete", zap.String("result", httpResult))

	// Step 2: เขียนผล HTTP call ลง DB
	if err := workflow.ExecuteActivity(ctx, DBWriteActivity, input.DBKey, httpResult).Get(ctx, nil); err != nil {
		logger.Error("DBWriteActivity failed", zap.Error(err))
		return err
	}

	logger.Info("PlatformWorkflow completed")
	return nil
}
