package instana

import (
	instana "github.com/instana/go-sensor"
	sensor "github.com/instana/go-sensor"
	"go.unistack.org/micro/v3/tracer"
)

type tracerOptionsKey struct{}

func Options(opts *sensor.Options) tracer.Option {
	return tracer.SetOption(tracerOptionsKey{}, opts)
}

type recorderKey struct{}

func Recorder(r instana.SpanRecorder) tracer.Option {
	return tracer.SetOption(recorderKey{}, r)
}
