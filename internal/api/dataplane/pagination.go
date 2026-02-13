package dataplane

import (
	"encoding/json"
	"net/http"
)

type paginationMetadata struct {
	Limit      int   `json:"limit"`
	Offset     int   `json:"offset"`
	Returned   int   `json:"returned"`
	Total      int64 `json:"total"`
	HasMore    bool  `json:"has_more"`
	NextOffset *int  `json:"next_offset,omitempty"`
}

type paginatedListResponse struct {
	Items      interface{}        `json:"items"`
	Pagination paginationMetadata `json:"pagination"`
}

type paginatedListWithSummaryResponse struct {
	Items      interface{}        `json:"items"`
	Pagination paginationMetadata `json:"pagination"`
	Summary    interface{}        `json:"summary,omitempty"`
}

func estimatePaginatedTotal(limit, offset, returned int) int64 {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	if returned < 0 {
		returned = 0
	}
	total := offset + returned
	if limit > 0 && returned >= limit {
		total++
	}
	return int64(total)
}

func writePaginatedList(w http.ResponseWriter, limit, offset, returned int, total int64, items interface{}) {
	writePaginatedListWithSummary(w, limit, offset, returned, total, items, nil)
}

func writePaginatedListWithSummary(w http.ResponseWriter, limit, offset, returned int, total int64, items interface{}, summary interface{}) {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	if returned < 0 {
		returned = 0
	}
	if total < 0 {
		total = int64(returned)
	}

	hasMore := int64(offset)+int64(returned) < total
	var nextOffset *int
	if hasMore {
		next := offset + returned
		nextOffset = &next
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(paginatedListWithSummaryResponse{
		Items: items,
		Pagination: paginationMetadata{
			Limit:      limit,
			Offset:     offset,
			Returned:   returned,
			Total:      total,
			HasMore:    hasMore,
			NextOffset: nextOffset,
		},
		Summary: summary,
	})
}
