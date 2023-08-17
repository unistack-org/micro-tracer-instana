package instana

import (
	sensor "github.com/instana/go-sensor"
	"go.unistack.org/micro/v3/tracer"
)

type tracerOptionsKey struct{}

func Options(opts *sensor.Options) tracer.Option {
	return tracer.SetOption(tracerOptionsKey{}, opts)
}
