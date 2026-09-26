package httpapi

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	if err := writeJSON(
		w,
		http.StatusOK,
		healthResponse{
			Status: "ok",
		},
	); err != nil {
		return
	}
}
