package instana

import (
	"context"
	"fmt"

	instana "github.com/instana/go-sensor"
	sensor "github.com/instana/go-sensor"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"go.unistack.org/micro/v3/metadata"
	"go.unistack.org/micro/v3/tracer"
	rutil "go.unistack.org/micro/v3/util/reflect"
)

var _ tracer.Tracer = (*Tracer)(nil)

type Tracer struct {
	opts   tracer.Options
	sensor sensor.Tracer
}

func (t *Tracer) Name() string {
	return t.opts.Name
}

func (t *Tracer) Flush(ctx context.Context) error {
	sensor.ShutdownSensor()
	return nil
}

func (t *Tracer) Init(opts ...tracer.Option) error {
	for _, o := range opts {
		o(&t.opts)
	}

	sensorOptions := sensor.DefaultOptions()
	if v, ok := t.opts.Context.Value(tracerOptionsKey{}).(*sensor.Options); ok && v != nil {
		sensorOptions = v
	}

	if sensorOptions.Service == "" {
		sensorOptions.Service = "micro"
	}

	var recorder instana.SpanRecorder
	recorder = &sensor.Recorder{}
	if v, ok := t.opts.Context.Value(recorderKey{}).(sensor.SpanRecorder); ok && v != nil {
		recorder = v
	}

	t.sensor = sensor.NewSensorWithTracer(sensor.NewTracerWithEverything(sensorOptions, recorder))

	sensor.StartMetrics(sensorOptions)
	return nil
}

type idStringer struct {
	s string
}

func (s idStringer) String() string {
	return s.s
}

type spanContext interface {
	TraceID() idStringer
	SpanID() idStringer
}

func (t *Tracer) Start(ctx context.Context, name string, opts ...tracer.SpanOption) (context.Context, tracer.Span) {
	options := tracer.NewSpanOptions(opts...)
	var span opentracing.Span
	switch options.Kind {
	case tracer.SpanKindInternal, tracer.SpanKindUnspecified:
		ctx, span = t.startSpanFromContext(ctx, name)
	case tracer.SpanKindClient, tracer.SpanKindProducer:
		ctx, span = t.startSpanFromOutgoingContext(ctx, name)
	case tracer.SpanKindServer, tracer.SpanKindConsumer:
		ctx, span = t.startSpanFromIncomingContext(ctx, name)
	}

	sp := &otSpan{topts: t.opts, span: span, opts: options, sensor: t.sensor, status: tracer.SpanStatusOK}

	spctx := span.Context()
	if v, ok := spctx.(spanContext); ok {
		sp.traceID = v.TraceID().String()
		sp.spanID = v.SpanID().String()
	} else {
		if val, err := rutil.StructFieldByName(spctx, "TraceID"); err == nil {
			sp.traceID = fmt.Sprintf("%v", val)
		}
		if val, err := rutil.StructFieldByName(spctx, "SpanID"); err == nil {
			sp.spanID = fmt.Sprintf("%v", val)
		}
	}

	return ctx, sp
}

type otSpan struct {
	span      opentracing.Span
	spanID    string
	traceID   string
	sensor    sensor.Tracer
	topts     tracer.Options
	opts      tracer.SpanOptions
	status    tracer.SpanStatus
	statusMsg string
}

func (os *otSpan) TraceID() string {
	return os.traceID
}

func (os *otSpan) SpanID() string {
	return os.spanID
}

func (os *otSpan) SetStatus(st tracer.SpanStatus, msg string) {
	switch st {
	case tracer.SpanStatusError:
		os.span.SetTag("error", true)
	}
	os.status = st
	os.statusMsg = msg
}

func (os *otSpan) Status() (tracer.SpanStatus, string) {
	return os.status, os.statusMsg
}

func (os *otSpan) Tracer() tracer.Tracer {
	return &Tracer{sensor: os.sensor, opts: os.topts}
}

func (os *otSpan) AddLogs(kv ...interface{}) {
	os.span.LogKV(kv...)
}

func (os *otSpan) Finish(opts ...tracer.SpanOption) {
	if len(os.opts.Labels)%2 != 0 {
		os.opts.Labels = os.opts.Labels[:len(os.opts.Labels)-1]
	}
	os.opts.Labels = tracer.UniqLabels(os.opts.Labels)
	for idx := 0; idx < len(os.opts.Labels); idx += 2 {
		switch os.opts.Labels[idx] {
		case "err":
			os.status = tracer.SpanStatusError
			os.statusMsg = fmt.Sprintf("%v", os.opts.Labels[idx+1])
		case "error":
			continue
		case "X-Request-Id", "x-request-id":
			os.span.SetTag("x-request-id", os.opts.Labels[idx+1])
		case "rpc.call", "rpc.call_type", "rpc.flavor", "rpc.service", "rpc.method",
			"sdk.database", "db.statement", "db.args", "db.query", "db.method",
			"messaging.destination.name", "messaging.source.name", "messaging.operation":
			os.span.SetTag(fmt.Sprintf("%v", os.opts.Labels[idx]), os.opts.Labels[idx+1])
		default:
			os.span.LogKV(os.opts.Labels[idx], os.opts.Labels[idx+1])
		}
	}
	if os.status == tracer.SpanStatusError {
		os.span.SetTag("error", true)
		os.span.LogKV("error", os.statusMsg)
	}
	os.span.SetTag("span.kind", os.opts.Kind)
	os.span.Finish()
}

func (os *otSpan) AddEvent(name string, opts ...tracer.EventOption) {
	os.span.LogFields(log.Event(name))
}

func (os *otSpan) Context() context.Context {
	return opentracing.ContextWithSpan(context.Background(), os.span)
}

func (os *otSpan) SetName(name string) {
	os.span = os.span.SetOperationName(name)
}

func (os *otSpan) SetLabels(labels ...interface{}) {
	os.opts.Labels = labels
}

func (os *otSpan) Kind() tracer.SpanKind {
	return os.opts.Kind
}

func (os *otSpan) AddLabels(labels ...interface{}) {
	os.opts.Labels = append(os.opts.Labels, labels...)
}

func NewTracer(opts ...tracer.Option) *Tracer {
	options := tracer.NewOptions(opts...)
	return &Tracer{opts: options}
}

func spanFromContext(ctx context.Context) (opentracing.Span, bool) {
	return sensor.SpanFromContext(ctx)
}

func (t *Tracer) startSpanFromAny(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (context.Context, opentracing.Span) {
	if tracerSpan, ok := tracer.SpanFromContext(ctx); ok && tracerSpan != nil {
		return t.startSpanFromContext(ctx, name, opts...)
	}

	if otSpan := opentracing.SpanFromContext(ctx); otSpan != nil {
		return t.startSpanFromContext(ctx, name, opts...)
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok && md != nil {
		return t.startSpanFromIncomingContext(ctx, name, opts...)
	}

	if md, ok := metadata.FromOutgoingContext(ctx); ok && md != nil {
		return t.startSpanFromOutgoingContext(ctx, name, opts...)
	}

	return t.startSpanFromContext(ctx, name, opts...)
}

func (t *Tracer) startSpanFromContext(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (context.Context, opentracing.Span) {
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
	}

	md := metadata.New(1)

	sp := t.sensor.StartSpan(name, opts...)
	if err := sp.Tracer().Inject(sp.Context(), opentracing.TextMap, opentracing.TextMapCarrier(md)); err != nil {
		return nil, nil
	}

	ctx = opentracing.ContextWithSpan(ctx, sp)

	return ctx, sp
}

func (t *Tracer) startSpanFromOutgoingContext(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (context.Context, opentracing.Span) {
	var parentCtx opentracing.SpanContext

	md, ok := metadata.FromOutgoingContext(ctx)
	if ok && md != nil {
		if spanCtx, err := t.sensor.Extract(opentracing.TextMap, opentracing.TextMapCarrier(md)); err == nil && ok {
			parentCtx = spanCtx
		}
	}

	if parentCtx != nil {
		opts = append(opts, opentracing.ChildOf(parentCtx))
	}

	nmd := metadata.Copy(md)

	sp := t.sensor.StartSpan(name, opts...)
	if err := sp.Tracer().Inject(sp.Context(), opentracing.TextMap, opentracing.TextMapCarrier(nmd)); err != nil {
		return nil, nil
	}

	ctx = metadata.NewOutgoingContext(opentracing.ContextWithSpan(ctx, sp), nmd)

	return ctx, sp
}

func (t *Tracer) startSpanFromIncomingContext(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (context.Context, opentracing.Span) {
	var parentCtx opentracing.SpanContext

	md, ok := metadata.FromIncomingContext(ctx)
	if ok && md != nil {
		if spanCtx, err := t.sensor.Extract(opentracing.TextMap, opentracing.TextMapCarrier(md)); err == nil {
			parentCtx = spanCtx
		}
	}

	if parentCtx != nil {
		opts = append(opts, opentracing.ChildOf(parentCtx))
	}

	nmd := metadata.Copy(md)

	sp := t.sensor.StartSpan(name, opts...)
	// fmt.Printf("StartSpan %#+v\n", sp)
	if err := sp.Tracer().Inject(sp.Context(), opentracing.TextMap, opentracing.TextMapCarrier(nmd)); err != nil {
		return nil, nil
	}

	ctx = metadata.NewIncomingContext(opentracing.ContextWithSpan(ctx, sp), nmd)

	return ctx, sp
}
