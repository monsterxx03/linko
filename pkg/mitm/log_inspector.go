package mitm

import (
	"log/slog"
)

type LogInspector struct {
	*BaseInspector
	logger *slog.Logger
	opts   LogInspectorOptions
}

type LogInspectorOptions struct {
	MaxBodySize int64
}

func NewLogInspector(logger *slog.Logger, hostname string, opts LogInspectorOptions) *LogInspector {
	if opts.MaxBodySize == 0 {
		opts.MaxBodySize = DefaultMaxBodySize
	}
	return &LogInspector{
		BaseInspector: NewBaseInspector("log-inspector", hostname),
		logger:        logger,
		opts:          opts,
	}
}

func (l *LogInspector) Inspect(direction Direction, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	displayData := data
	if int64(len(displayData)) > l.opts.MaxBodySize {
		displayData = displayData[:l.opts.MaxBodySize]
	}

	text := string(data)
	if text != "" {
		if len(text) > int(l.opts.MaxBodySize) {
			text = text[:l.opts.MaxBodySize]
		}
		l.logger.Debug("MITM traffic",
			"direction", direction,
			"preview", text,
		)
	}

	return data, nil
}
