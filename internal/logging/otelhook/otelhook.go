package otelhook

import (
	"context"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/bridges/otellogrus"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func NewOtelHook(ctx context.Context, res *resource.Resource) *otellogrus.Hook {
	exporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil
	}

	lp := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(
			log.NewBatchProcessor(exporter),
		),
	)

	return otellogrus.NewHook("ETOS Execution Space Provider",
		otellogrus.WithLoggerProvider(lp),
		otellogrus.WithLevels(logrus.AllLevels),
	)
}