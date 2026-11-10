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
	f.gotLimit = limit
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
		wantCvp    int
	}{
		"valid prefix returns notices": {
			url: "/notices?cpv=45",
			notices: []noticeSummary{
				{ID: "test-1", Heading: "Road works"},
				{ID: "test-2", Heading: "Bridge repair"},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantCvp: 45,
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
			wantCvp: 45,
		},
		"no matches returns empty list": {
			url:        "/notices?cpv=99",
			wantStatus: http.StatusOK,
			wantCount:  0,
			wantCvp: 99,
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
		"parameter above max returns error":       {query: "days=366", wantErr: true},
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

func TestHandleClosingSoon(t *testing.T) {
	const dbErrorMessage = "database is down"
	tests := map[string]struct {
		url        string
		storeErr   error
		wantStatus int
		wantDays   int
		wantEmpty  bool
	}{
		"default days is 14": {
			url:        "/notices/closing",
			wantStatus: http.StatusOK,
			wantDays:   14,
		},
		"valid days return OK": {
			url:        "/notices/closing?days=30",
			wantStatus: http.StatusOK,
			wantDays:   30,
		},
		"invalid days return 400": {
			url:        "/notices/closing?days=abc",
			wantStatus: http.StatusBadRequest,
		},
		"store error return 500": {
			url:        "/notices/closing?days=10",
			wantStatus: http.StatusInternalServerError,
			storeErr:   errors.New(dbErrorMessage),
			wantDays:   10,
		},
		"no matches returns empty list": {
			url:        "/notices/closing?days=1",
			wantStatus: http.StatusOK,
			wantDays:   1,
			wantEmpty:  true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeStore{err: tt.storeErr}
			app := &server{store: fake}
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			app.routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if fake.gotDays != tt.wantDays {
				t.Errorf("store got days=%d, want %d", fake.gotDays, tt.wantDays)
			}

			body := rec.Body.String()

			if tt.storeErr != nil && strings.Contains(body, dbErrorMessage) {
				t.Fatalf("body contains internal error message: %s", body)
			}

			if tt.wantEmpty {
				if strings.TrimSpace(body) != "[]" {
					t.Errorf("want [], got %s", rec.Body.String())
				}
			}
		})
	}
}

func TestHandleTopBuyers(t *testing.T) {
	const dbErrorMessage = "database is down"
	tests := map[string]struct {
		url        string
		storeErr   error
		wantStatus int
		wantLimit  int
		wantEmpty  bool
	}{
		"default limit is 10": {
			url:        "/buyers/top",
			wantStatus: http.StatusOK,
			wantLimit:  10,
		},
		"valid limit return OK": {
			url:        "/buyers/top?limit=30",
			wantStatus: http.StatusOK,
			wantLimit:  30,
		},
		"invalid limit return 400": {
			url:        "/buyers/top?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		"store error return 500": {
			url:        "/buyers/top?limit=20",
			wantStatus: http.StatusInternalServerError,
			storeErr:   errors.New(dbErrorMessage),
			wantLimit:  20,
		},
		"no matches returns empty list": {
			url:        "/buyers/top?limit=1",
			wantStatus: http.StatusOK,
			wantLimit:  1,
			wantEmpty:  true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeStore{err: tt.storeErr}
			app := &server{store: fake}
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			app.routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if fake.gotLimit != tt.wantLimit {
				t.Errorf("store got limit=%d, want %d", fake.gotLimit, tt.wantLimit)
			}

			body := rec.Body.String()

			if tt.storeErr != nil && strings.Contains(body, dbErrorMessage) {
				t.Fatalf("body contains internal error message: %s", body)
			}

			if tt.wantEmpty {
				if strings.TrimSpace(body) != "[]" {
					t.Errorf("want [], got %s", rec.Body.String())
				}
			}
		})
	}
}

func TestHandleTopBuyersJSONFields(t *testing.T) {
	total := 5000000.0
	organizationID := "974767597"
	name := "Oslo kommune"
	noticeCount := 3
	fake := &fakeStore{buyers: []buyerTotal{
		{OrganizationID: organizationID, Name: name, TotalEstimated: &total, NoticeCount: noticeCount},
	}}
	app := &server{store: fake}
	req := httptest.NewRequest(http.MethodGet, "/buyers/top", nil)
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d buyers, want 1", len(got))
	}

	if got[0]["organizationId"] != organizationID {
		t.Errorf("got %v for organizationId should be %s", got[0]["organizationId"], organizationID)
	}
	if got[0]["name"] != name {
		t.Errorf("got %v for name should be %s", got[0]["name"], name)
	}
	if got[0]["totalEstimated"] != total {
		t.Errorf("got %v for total should be %f", got[0]["totalEstimated"], total)
	}
	if got[0]["noticeCount"] != float64(noticeCount) {
		t.Errorf("got %v for noticeCount should be %d", got[0]["noticeCount"], noticeCount)
	}
}
