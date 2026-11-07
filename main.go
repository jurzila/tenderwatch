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

func searchNotices(ctx context.Context, client *http.Client, apiKey string, pageSize int) (searchResponse, error) {
	params := url.Values{}
	params.Set("numHitsPerPage", strconv.Itoa(pageSize))
	searchURL := "https://api.doffin.no/public/v2/search?" + params.Encode()

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
	apiKey := os.Getenv("DOFFIN_API_KEY")
	if apiKey == "" {
		log.Fatal("No API key found")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	result, err := searchNotices(context.Background(), client, apiKey, 2)
	if err != nil {
		log.Fatalf("search notices: %v", err)
	}
	for _, n := range result.Hits {
		fmt.Printf("%+v\n", n)
	}
}
