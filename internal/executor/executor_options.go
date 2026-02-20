package executor

import (
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/secrets"
)

type Option func(*Executor)

// WithLogger sets the logger
func WithLogger(logger *logging.Logger) Option {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithSecretsResolver sets the secrets resolver for $SECRET: reference resolution
func WithSecretsResolver(resolver *secrets.Resolver) Option {
	return func(e *Executor) {
		e.secretsResolver = resolver
	}
}

// WithLogBatcherConfig sets the log batcher configuration
func WithLogBatcherConfig(cfg LogBatcherConfig) Option {
	return func(e *Executor) {
		e.logBatcherConfig = cfg
	}
}

// WithLogSink sets the log sink for invocation log persistence.
// When set, logs are routed through the sink instead of directly to PostgreSQL.
func WithLogSink(sink logsink.LogSink) Option {
	return func(e *Executor) {
		e.logSink = sink
	}
}

// WithPayloadPersistence controls whether full invocation payloads/stdout/stderr are stored.
func WithPayloadPersistence(enabled bool) Option {
	return func(e *Executor) {
		e.persistPayloads = enabled
	}
}

// WithTransportCipher sets the transport cipher used to encrypt secret
// values before they are sent to the guest agent in the Init message.
// When nil, secrets are sent as plaintext (legacy behaviour).
func WithTransportCipher(tc *secrets.TransportCipher) Option {
	return func(e *Executor) {
		e.transportCipher = tc
	}
}

// safeGo runs f in a new goroutine with panic recovery so that a failure
// in fire-and-forget background work never crashes the process.
func safeGo(f func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Op().Error("recovered panic in async task", "panic", r)
			}
		}()
		f()
	}()
}

// resolveVolumeMounts builds the resolved mount list by matching each
// VolumeMount.VolumeID to its Volume metadata to obtain the host-side
// image path. Unresolved mounts (volume not found) are silently skipped.
func resolveVolumeMounts(mounts []domain.VolumeMount, volumes []*domain.Volume) []domain.ResolvedMount {
	if len(mounts) == 0 || len(volumes) == 0 {
		return nil
	}
	volMap := make(map[string]*domain.Volume, len(volumes))
	for _, v := range volumes {
		volMap[v.ID] = v
	}
	var resolved []domain.ResolvedMount
	for _, m := range mounts {
		vol, ok := volMap[m.VolumeID]
		if !ok || vol.ImagePath == "" {
			continue
		}
		resolved = append(resolved, domain.ResolvedMount{
			ImagePath: vol.ImagePath,
			MountPath: m.MountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	return resolved
}
