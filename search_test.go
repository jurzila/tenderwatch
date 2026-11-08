package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchNotices(t *testing.T) {
	tests := map[string]struct {
		status             int
		body               string
		wantErr            bool
		wantHits           int
		wantSecondValueNil bool
	}{
		"two notices, one without value": {
			status: http.StatusOK,
			body: `{"numHitsTotal": 2, "hits": [
				{"id": "1", "estimatedValue": {"currencyCode": "NOK", "amount": 100}},
				{"id": "2", "estimatedValue": null}
			]}`,
			wantErr:            false,
			wantHits:           2,
			wantSecondValueNil: true,
		},
		"server answers 401": {
			status: http.StatusUnauthorized,
			body: `{"statusCode": 401, 
			"message": "Access denied due to missing subscription key. Make sure to include subscription key when making requests to an API."}`,
			wantErr: true,
		},
		"server answers 200 but body is html": {
			status: http.StatusOK,
			body: `<!DOCTYPE html>
				<html lang="en">
				<head>
				<title>Service Unavailable - Maintenance</title>
				</head>
				<body>
				<div class="container">
					<h1>We'll be back soon!</h1>
				</div>
				</body>
				</html>`,
			wantErr: true,
		},
		"server answers 200 with valid JSON but wrong type": {
			status: http.StatusOK,
			body: `{"numHitsTotal": 2, "hits": [
				{"id": "1", "estimatedValue": 100}
			]}`,
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "test-key" {
					t.Errorf("Ocp-Apim-Subscription-Key = %q, want %q", got, "test-key")
				}
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			got, err := searchNotices(context.Background(), srv.Client(), "test-key", 2, srv.URL)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got.Hits) != tt.wantHits {
				t.Fatalf("got %d hits, want %d", len(got.Hits), tt.wantHits)
			}

			if tt.wantSecondValueNil {
				if got.Hits[1].EstimatedValue != nil {
					t.Fatalf("expected nil, got a value: %v", got.Hits[1].EstimatedValue)
				}
			}
		})
	}
}
