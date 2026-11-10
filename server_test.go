package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeStore struct {
	notices  []noticeSummary
	buyers   []buyerTotal
	err      error
	gotDays  int
	gotLimit int
}

func (f *fakeStore) NoticesByCPV(ctx context.Context, prefix string) ([]noticeSummary, error) {
	return f.notices, f.err
}
func (f *fakeStore) ClosingSoon(ctx context.Context, days int) ([]noticeSummary, error) {
	f.gotDays = days
	return f.notices, f.err
}
func (f *fakeStore) TopBuyers(ctx context.Context, limit int) ([]buyerTotal, error) {
	return f.buyers, f.err
}

func TestHandleNoticesByCPV(t *testing.T) {
	const dbErrorMessage = "database is down"
	tests := map[string]struct {
		url        string
		notices    []noticeSummary
		storeErr   error
		wantStatus int
		wantCount  int
	}{
		"valid prefix returns notices": {
			url: "/notices?cpv=45",
			notices: []noticeSummary{
				{ID: "test-1", Heading: "Road works"},
				{ID: "test-2", Heading: "Bridge repair"},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		"missing parameter returns 400": {
			url:        "/notices",
			wantStatus: http.StatusBadRequest,
		},
		"prefix contains not digits returns 400": {
			url:        "/notices?cpv=4a",
			wantStatus: http.StatusBadRequest,
		},
		"store fails returns 500": {
			url:        "/notices?cpv=45",
			wantStatus: http.StatusInternalServerError,
			storeErr:   errors.New(dbErrorMessage),
		},
		"no matches returns empty list": {
			url:        "/notices?cpv=99",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			app := &server{store: &fakeStore{notices: tt.notices, err: tt.storeErr}}
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			app.routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus != http.StatusOK {
				if tt.storeErr != nil && strings.Contains(rec.Body.String(), dbErrorMessage) {
					t.Fatalf("body contains internal error message: %s", rec.Body.String())
				}
				return
			}

			var got []noticeSummary
			body := rec.Body.String()
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("got %d notices, want %d", len(got), tt.wantCount)
			}
			if len(got) == 0 {
				if strings.TrimSpace(body) != "[]" {
					t.Errorf("body is not empty: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestIntParam(t *testing.T) {
	tests := map[string]struct {
		query   string
		want    int
		wantErr bool
	}{
		"missing uses default":                    {query: "", want: 14},
		"parameter above max returns error":       {query: "days=400", wantErr: true},
		"parameter below min returns error":       {query: "days=0", wantErr: true},
		"parameter exactly min no error":          {query: "days=1", want: 1},
		"parameter exactly max no error":          {query: "days=365", want: 365},
		"parameter is not a number returns error": {query: "days=abc", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x?"+tt.query, nil)

			got, err := intParam(req, "days", 14, 1, 365)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got value %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
