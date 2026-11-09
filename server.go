package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

type noticeStore interface {
	NoticesByCPV(ctx context.Context, prefix string) ([]noticeSummary, error)
}

type server struct{ store noticeStore }

func (s *server) handleNoticesByCPV(w http.ResponseWriter, r *http.Request) {
	cpv := r.URL.Query().Get("cpv")
	if cpv == "" {
		http.Error(w, "cpv query parameter is required", http.StatusBadRequest)
		return
	}
	for _, p := range cpv {
		if p < '0' || p > '9' {
			http.Error(w, "cpv must contain only digits", http.StatusBadRequest)
			return
		}
	}
	if len(cpv) > 8 {
		http.Error(w, "cpv must not be longer than 8 digits", http.StatusBadRequest)
		return
	}

	notices, err := s.store.NoticesByCPV(r.Context(), cpv)
	if err != nil {
		log.Printf("store error: %v", err)
		http.Error(w, "Error while storing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if notices == nil {
		notices = []noticeSummary{}
	}
	if err := json.NewEncoder(w).Encode(notices); err != nil {
		log.Printf("encode products: %v", err)
	}

}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notices", s.handleNoticesByCPV)
	return mux
}
