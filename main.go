package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type searchResponse struct {
	NumHitsTotal int      `json:"numHitsTotal"`
	Hits         []notice `json:"hits"`
}

type notice struct {
	ID             string    `json:"id"`
	Heading        string    `json:"heading"`
	Buyers         []buyer   `json:"buyer"`
	Status         string    `json:"status"`
	IssueDate      time.Time `json:"issueDate"`
	Deadline       time.Time `json:"deadline"`
	EstimatedValue *money    `json:"estimatedValue"`
	CPVCodes       []string  `json:"cpvCodes"`
	Type           string    `json:"type"`
}

type buyer struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
}

type money struct {
	CurrencyCode string  `json:"currencyCode"`
	Amount       float64 `json:"amount"`
}

func (m *money) String() string {
	if m == nil {
		return "No amount listed"
	}
	return fmt.Sprintf("%.0f %s", m.Amount, m.CurrencyCode)
}

func searchNotices(ctx context.Context, client *http.Client, apiKey string, pageSize int, baseURL string) (searchResponse, error) {
	params := url.Values{}
	params.Set("numHitsPerPage", strconv.Itoa(pageSize))
	searchURL := baseURL + "/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return searchResponse{}, fmt.Errorf("build search request: %w", err)
	}

	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return searchResponse{}, fmt.Errorf("send search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return searchResponse{}, fmt.Errorf("status search request not OK: %d", resp.StatusCode)
	}

	received := searchResponse{}

	if err := json.NewDecoder(resp.Body).Decode(&received); err != nil {
		return searchResponse{}, fmt.Errorf("response decoding failed: %w", err)
	}

	return received, nil
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: tenderwatch <ingest|serve>")
	}

	baseURL := "https://api.doffin.no/public/v2"
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("no Database URL found")
	}
	apiKey := os.Getenv("DOFFIN_API_KEY")
	if apiKey == "" {
		log.Fatal("no API key found")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	pool, err := connectDB(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer pool.Close()

	if err := applySchema(context.Background(), pool); err != nil {
		log.Fatal(err)
	}
	log.Print("schema applied")

	switch os.Args[1] {
	case "ingest":
		result, err := searchNotices(context.Background(), client, apiKey, 50, baseURL)
		if err != nil {
			log.Fatalf("search notices: %v", err)
		}
		var saved int
		for _, n := range result.Hits {
			if err := saveNotice(context.Background(), pool, n); err != nil {
				log.Printf("upsert failed for notice with ID %s: %v", n.ID, err)
				continue
			}
			saved++
		}
		log.Printf("saved %d of %d notices", saved, len(result.Hits))

	case "serve":
		app := &server{store: &pgStore{pool: pool}}
		srv := &http.Server{
			Addr:         ":8080",
			Handler:      app.routes(),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		log.Printf("listening on %s", srv.Addr)
		log.Fatal(srv.ListenAndServe())
	default:
		log.Fatalf("unknown command %q", os.Args[1])
	}
}
