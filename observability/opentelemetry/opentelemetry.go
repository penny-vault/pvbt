// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opentelemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/penny-vault/pv-api/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

const (
	Name = "github.com/penny-vault/pv-api"
)

func Setup() (func(context.Context) error, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("pvapi"),
			// service version
			semconv.ServiceVersionKey.String(common.CurrentVersion.String()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Set up a trace exporter
	var client otlptrace.Client

	if viper.GetBool("oltp.http") {
		log.Info("Using HTTP(s) for OLTP connection")
		client = otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(viper.GetString("otlp.endpoint")),
			otlptracehttp.WithHeaders(viper.GetStringMapString("otlp.headers")),
		)
	} else {
		// use gRPC
		client = otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(viper.GetString("otlp.endpoint")),
			otlptracegrpc.WithHeaders(viper.GetStringMapString("otlp.headers")),
		)
	}

	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Shutdown will flush any remaining spans and shut down the exporter.
	return tracerProvider.Shutdown, nil
}

func SpanAttributesFromFiber(c *fiber.Ctx) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(string(semconv.HTTPClientIPKey), c.IP()),
		attribute.String(string(semconv.HTTPMethodKey), c.Method()),
		attribute.String(string(semconv.HTTPUserAgentKey), string(c.Context().UserAgent())),
	}
}
