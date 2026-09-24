// Command laadpalen-api is the laadpalen example's own backend - the PEP. The browser only ever talks to this
// server; this is the one place that calls the real PDP (the Eamerald authorizer), never the frontend.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	defaultAuthorizerURL = "https://localhost:8383"
	readHeaderTimeout    = 5 * time.Second
)

// RequestLaadpaal is the business request the frontend sends: someone requesting to use a laadpaal at an address.
// This is the API's own contract - callers don't need to know or care that it's answered by calling a PDP.
type RequestLaadpaal struct {
	User       string `json:"user"`
	Postcode   string `json:"postcode"`
	Huisnummer string `json:"huisnummer"`
}

// RequestLaadpaalResponse is the API's own business-shaped answer - never the PDP's response shape directly. Debug
// carries the exact PDP call purely for the GUI's "show me the raw API call" inspector panel; it's not part of the
// business contract above, and callers that only care about the decision can ignore it entirely.
type RequestLaadpaalResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	Error   string `json:"error,omitempty"`
	Debug   struct {
		Request  pdpEvaluationRequest `json:"request"`
		Response any                  `json:"response,omitempty"`
	} `json:"debug"`
}

// pdpEvaluationRequest is the AuthZEN Access Evaluation API request body - the PDP's own wire format, built from a
// RequestLaadpaal only where it's actually needed: right before calling the PDP. It never leaks into the API's own
// request contract above.
type pdpEvaluationRequest struct {
	Subject struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"subject"`
	Action struct {
		Name string `json:"name"`
	} `json:"action"`
	Resource struct {
		Type       string `json:"type"`
		Properties struct {
			Postcode   string `json:"postcode"`
			Huisnummer int    `json:"huisnummer"`
		} `json:"properties"`
	} `json:"resource"`
	Context struct {
		Doelbinding string `json:"doelbinding"`
	} `json:"context"`
}

// pdpEvaluationResponse is the AuthZEN Access Evaluation API response body - just the two fields this API actually
// reads back out of it.
type pdpEvaluationResponse struct {
	Decision bool `json:"decision"`
	Context  struct {
		Reason string `json:"reason"`
	} `json:"context"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}

	authorizerURL := os.Getenv("AUTHORIZER_URL")
	if authorizerURL == "" {
		authorizerURL = defaultAuthorizerURL
	}

	client := &http.Client{
		Transport: &http.Transport{
			// Self-signed dev cert - fine here, since this handshake happens server-side and never involves a
			// browser's own trust store.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	http.HandleFunc("/api/request-laadpaal", handleRequestLaadpaal(client, authorizerURL))

	//nolint:gosec // G706: port is an operator-set env var, not user input
	log.Printf("laadpalen api listening on http://localhost:%s", port)

	srv := &http.Server{Addr: ":" + port, ReadHeaderTimeout: readHeaderTimeout}
	log.Fatal(srv.ListenAndServe())
}

func handleRequestLaadpaal(client *http.Client, authorizerURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		var in RequestLaadpaal
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		huisnummer, err := strconv.Atoi(in.Huisnummer)
		if err != nil {
			http.Error(w, "invalid huisnummer: "+err.Error(), http.StatusBadRequest)
			return
		}

		var out RequestLaadpaalResponse

		out.Debug.Request.Subject.Type = "user"
		out.Debug.Request.Subject.ID = in.User
		out.Debug.Request.Action.Name = "request_laadpaal"
		out.Debug.Request.Resource.Type = "adres" //nolint:misspell // Dutch for "address" - the manifest type name
		out.Debug.Request.Resource.Properties.Postcode = in.Postcode
		out.Debug.Request.Resource.Properties.Huisnummer = huisnummer
		out.Debug.Request.Context.Doelbinding = "laadpalen"

		decision, rawResponse, err := evaluate(r.Context(), client, authorizerURL, out.Debug.Request)
		if err != nil {
			out.Error = err.Error()
			writeJSON(w, http.StatusBadGateway, out)

			return
		}

		out.Allowed = decision.Decision
		out.Reason = decision.Context.Reason
		out.Debug.Response = rawResponse
		writeJSON(w, http.StatusOK, out)
	}
}

// evaluate calls the PDP and returns both the decoded decision and the raw response body, so the caller can show
// the exact bytes the PDP returned in its debug panel without evaluate needing to know that's what it's for.
func evaluate(
	ctx context.Context, client *http.Client, authorizerURL string, req pdpEvaluationRequest,
) (pdpEvaluationResponse, any, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return pdpEvaluationResponse{}, nil, err
	}

	//nolint:gosec // G704: authorizerURL is an operator-set env var, not user input
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, authorizerURL+"/access/v1/evaluation", bytes.NewReader(body))
	if err != nil {
		return pdpEvaluationResponse{}, nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	//nolint:gosec // G704: authorizerURL is an operator-set env var, not user input
	resp, err := client.Do(httpReq)
	if err != nil {
		return pdpEvaluationResponse{}, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return pdpEvaluationResponse{}, nil, err
	}

	var decision pdpEvaluationResponse
	if err := json.Unmarshal(raw, &decision); err != nil {
		return pdpEvaluationResponse{}, nil, err
	}

	var rawAny any
	if err := json.Unmarshal(raw, &rawAny); err != nil {
		return pdpEvaluationResponse{}, nil, err
	}

	return decision, rawAny, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
