package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/domain"
)

// applyParamMappings delegates to the shared domain implementation.
func applyParamMappings(
	payload json.RawMessage,
	r *http.Request,
	pathParams map[string]string,
	mappings []domain.ParamMapping,
) (json.RawMessage, error) {
	return domain.ApplyParamMappings(payload, r, pathParams, mappings)
}
