package instana

import (
	"context"
	"testing"

	sensor "github.com/instana/go-sensor"
	"go.unistack.org/micro/v3/metadata"
	"go.unistack.org/micro/v3/tracer"
)

func TestTraceID(t *testing.T) {
	md := metadata.New(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = metadata.NewIncomingContext(ctx, md)

	tr := NewTracer()
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}

	var sp tracer.Span

	ctx, sp = tr.Start(ctx, "test")
	if v := sp.TraceID(); v == "" {
		t.Fatalf("invalid span trace id %#+v", v)
	}
	if v := sp.SpanID(); v == "" {
		t.Fatalf("invalid span span id %#+v", v)
	}
}

func TestSpanPropogation(t *testing.T) {
	md := metadata.New(0)
	md.Set(
		"X-Instana-T", "0000000000002435",
		"X-Instana-S", "0000000000003546",
		"X-Instana-L", "1",
		"Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000003546-01",
		"Tracestate", "in=0000000000002435;0000000000003546,rojo=00f067aa0ba902b7",
	)

	ctx := metadata.NewIncomingContext(context.TODO(), md)
	var span tracer.Span
	r := sensor.NewTestRecorder()
	tr := NewTracer(Recorder(r))
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}
	ctx, span = tr.Start(ctx, "test", tracer.WithSpanKind(tracer.SpanKindServer))
	span.Finish()

	spans := r.GetQueuedSpans()
	for _, s := range spans {
		t.Logf("spans %#+v\n", s)
	}
	tr.Flush(ctx)
}
