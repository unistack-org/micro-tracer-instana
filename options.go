package instana

import (
	sensor "github.com/instana/go-sensor"
	"go.unistack.org/micro/v4/options"
)

type tracerOptionsKey struct{}

func Options(opts *sensor.Options) options.Option {
	return options.ContextOption(tracerOptionsKey{}, opts)
}
