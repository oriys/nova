package controlplane

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

func paginateSliceWindow[T any](items []T, limit, offset int) ([]T, int) {
	if offset < 0 {
		offset = 0
	}
	total := len(items)
	if limit <= 0 {
		limit = total
	}
	if offset >= total {
		return []T{}, total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end], total
}

func writePaginatedList(w http.ResponseWriter, limit, offset, returned int, total int64, items interface{}) {
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
	_ = json.NewEncoder(w).Encode(paginatedListResponse{
		Items: items,
		Pagination: paginationMetadata{
			Limit:      limit,
			Offset:     offset,
			Returned:   returned,
			Total:      total,
			HasMore:    hasMore,
			NextOffset: nextOffset,
		},
	})
}
