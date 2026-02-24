package controlplane

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestEstimatePaginatedTotal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		limit    int
		offset   int
		returned int
		want     int64
	}{
		{"basic partial page", 10, 0, 5, 5},
		{"full page has more", 10, 0, 10, 11},
		{"with offset partial", 10, 20, 5, 25},
		{"with offset full page", 10, 20, 10, 31},
		{"zero limit", 0, 0, 5, 5},
		{"negative values clamped", -1, -1, -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := estimatePaginatedTotal(tt.limit, tt.offset, tt.returned)
			if got != tt.want {
				t.Errorf("estimatePaginatedTotal(%d, %d, %d) = %d, want %d",
					tt.limit, tt.offset, tt.returned, got, tt.want)
			}
		})
	}
}

func TestPaginateSliceWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		items     []int
		limit     int
		offset    int
		wantItems []int
		wantTotal int
	}{
		{"normal first page", []int{1, 2, 3, 4, 5}, 2, 0, []int{1, 2}, 5},
		{"with offset", []int{1, 2, 3, 4, 5}, 2, 2, []int{3, 4}, 5},
		{"offset beyond length", []int{1, 2, 3}, 2, 10, []int{}, 3},
		{"zero limit returns all", []int{1, 2, 3}, 0, 0, []int{1, 2, 3}, 3},
		{"negative offset clamped", []int{1, 2, 3}, 2, -1, []int{1, 2}, 3},
		{"empty items", []int{}, 10, 0, []int{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotItems, gotTotal := paginateSliceWindow(tt.items, tt.limit, tt.offset)
			if gotTotal != tt.wantTotal {
				t.Errorf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			if len(gotItems) != len(tt.wantItems) {
				t.Fatalf("len(items) = %d, want %d", len(gotItems), len(tt.wantItems))
			}
			for i := range gotItems {
				if gotItems[i] != tt.wantItems[i] {
					t.Errorf("items[%d] = %d, want %d", i, gotItems[i], tt.wantItems[i])
				}
			}
		})
	}
}

func TestWritePaginatedList(t *testing.T) {
	t.Parallel()

	type respShape struct {
		Items      []string           `json:"items"`
		Pagination paginationMetadata `json:"pagination"`
	}

	t.Run("basic response structure", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		writePaginatedList(w, 10, 0, 2, 2, []string{"a", "b"})

		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var resp respShape
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Items) != 2 {
			t.Errorf("items len = %d, want 2", len(resp.Items))
		}
		if resp.Pagination.Returned != 2 {
			t.Errorf("returned = %d, want 2", resp.Pagination.Returned)
		}
	})

	t.Run("has_more true when more results", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		writePaginatedList(w, 10, 0, 5, 20, []string{"x"})

		var resp respShape
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !resp.Pagination.HasMore {
			t.Error("has_more = false, want true")
		}
		if resp.Pagination.NextOffset == nil {
			t.Fatal("next_offset is nil, want non-nil")
		}
		if *resp.Pagination.NextOffset != 5 {
			t.Errorf("next_offset = %d, want 5", *resp.Pagination.NextOffset)
		}
	})

	t.Run("has_more false at end", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		writePaginatedList(w, 10, 15, 5, 20, []string{"y"})

		var resp respShape
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Pagination.HasMore {
			t.Error("has_more = true, want false")
		}
		if resp.Pagination.NextOffset != nil {
			t.Errorf("next_offset = %d, want nil", *resp.Pagination.NextOffset)
		}
	})
}

func TestParsePaginationParam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		raw      string
		fallback int
		max      int
		want     int
	}{
		{"empty returns fallback", "", 25, 100, 25},
		{"valid value", "50", 25, 100, 50},
		{"exceeds max capped", "200", 25, 100, 100},
		{"negative returns fallback", "-5", 25, 100, 25},
		{"non-numeric returns fallback", "abc", 25, 100, 25},
		{"zero max no cap", "50", 25, 0, 50},
		{"whitespace trimmed", " 42 ", 25, 100, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parsePaginationParam(tt.raw, tt.fallback, tt.max)
			if got != tt.want {
				t.Errorf("parsePaginationParam(%q, %d, %d) = %d, want %d",
					tt.raw, tt.fallback, tt.max, got, tt.want)
			}
		})
	}
}
