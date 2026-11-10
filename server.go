package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

type noticeStore interface {
	NoticesByCPV(ctx context.Context, prefix string) ([]noticeSummary, error)
	ClosingSoon(ctx context.Context, days int) ([]noticeSummary, error)
	TopBuyers(ctx context.Context, limit int) ([]buyerTotal, error)
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
		log.Printf("encode notices: %v", err)
	}

}

func intParam(r *http.Request, name string, def, min, max int) (int, error) {
	param := r.URL.Query().Get(name)
	if param == "" {
		return def, nil 
	}
	intValue, err := strconv.Atoi(param)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	if intValue <= min || intValue > max {
		return 0, fmt.Errorf("%s has to be between %d and %d", name, min, max)
	}
	return intValue, nil
}

func (s *server) handleClosingSoon(w http.ResponseWriter, r *http.Request) {
	days, err := intParam(r, "days", 14, 1, 365)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	notices, err := s.store.ClosingSoon(r.Context(), days)
	if err != nil {
		log.Printf("store error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if notices == nil {
		notices = []noticeSummary{}
	}
	if err := json.NewEncoder(w).Encode(notices); err != nil {
		log.Printf("encode notices: %v", err)
	}
}

func (s *server) handleTopBuyers(w http.ResponseWriter, r *http.Request) {
	limit, err := intParam(r, "limit", 10, 1, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	buyers, err := s.store.TopBuyers(r.Context(), limit)
	if err != nil {
		log.Printf("store error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if buyers == nil {
		buyers = []buyerTotal{}
	}
	if err := json.NewEncoder(w).Encode(buyers); err != nil {
		log.Printf("encode buyers: %v", err)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notices", s.handleNoticesByCPV)
	mux.HandleFunc("GET /notices/closing", s.handleClosingSoon)
	mux.HandleFunc("GET /buyers/top", s.handleTopBuyers)
	return mux
}
