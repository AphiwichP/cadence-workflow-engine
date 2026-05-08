package main

import (
	"os"

	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/cadence/worker"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/zap"
)

const (
	domain   = "platform-workflows"
	taskList = "platform-task-list"
)

func cadenceFrontendHost() string {
	if h := os.Getenv("CADENCE_FRONTEND_HOST"); h != "" {
		return h
	}
	return "cadence-frontend.ai-platform.svc.cluster.local:7833"
}

func buildServiceClient(logger *zap.Logger) workflowserviceclient.Interface {
	host := cadenceFrontendHost()
	logger.Info("connecting to Cadence frontend", zap.String("host", host))

	transport := grpc.NewTransport()
	outbound := transport.NewSingleOutbound(host)

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "cadence-worker",
		Outbounds: yarpc.Outbounds{
			"cadence-frontend": {Unary: outbound},
		},
	})

	if err := dispatcher.Start(); err != nil {
		logger.Fatal("failed to start yarpc dispatcher", zap.Error(err))
	}

	return workflowserviceclient.New(dispatcher.ClientConfig("cadence-frontend"))
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	svc := buildServiceClient(logger)

	w := worker.New(svc, domain, taskList, worker.Options{
		Logger: logger,
	})

	w.RegisterWorkflow(PlatformWorkflow)
	w.RegisterActivity(HTTPCallActivity)
	w.RegisterActivity(DBWriteActivity)

	logger.Info("starting Cadence worker",
		zap.String("domain", domain),
		zap.String("taskList", taskList),
	)

	if err := w.Run(); err != nil {
		logger.Fatal("worker stopped", zap.Error(err))
	}
}
