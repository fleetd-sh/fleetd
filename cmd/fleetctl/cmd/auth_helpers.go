package cmd

import (
	"fmt"
	"net/http"
)

// addAuthHeaders adds authentication headers to HTTP requests
func addAuthHeaders(req *http.Request) error {
	token, err := GetAuthToken()
	if err != nil {
		// For now, skip auth if no token
		// TODO: Make this required once auth is fully implemented
		return nil
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}