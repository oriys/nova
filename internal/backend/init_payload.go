package backend

import (
	"encoding/json"

	"github.com/oriys/nova/internal/domain"
)

// BuildInitPayload builds a backend-agnostic agent init payload so all
// backends send consistent function startup parameters.
func BuildInitPayload(fn *domain.Function) *InitPayload {
	volumeMounts := make([]VolumeMountInfo, 0, len(fn.ResolvedMounts))
	for _, rm := range fn.ResolvedMounts {
		volumeMounts = append(volumeMounts, VolumeMountInfo{
			MountPath: rm.MountPath,
			ReadOnly:  rm.ReadOnly,
		})
	}

	return &InitPayload{
		Runtime:         string(fn.Runtime),
		Handler:         fn.Handler,
		EnvVars:         fn.EnvVars,
		Command:         fn.RuntimeCommand,
		Extension:       fn.RuntimeExtension,
		Mode:            string(fn.Mode),
		FunctionName:    fn.Name,
		FunctionVersion: fn.Version,
		MemoryMB:        fn.MemoryMB,
		TimeoutS:        fn.TimeoutS,
		LayerCount:      len(fn.LayerPaths),
		VolumeMounts:    volumeMounts,
	}
}

// MarshalInitPayload returns the canonical JSON init payload for the agent.
func MarshalInitPayload(fn *domain.Function) (json.RawMessage, error) {
	payload, err := json.Marshal(BuildInitPayload(fn))
	if err != nil {
		return nil, err
	}
	return payload, nil
}
