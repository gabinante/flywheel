package policy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// HTTPCheckChecker verifies that an HTTP endpoint returns the expected status code.
type HTTPCheckChecker struct {
	Client *http.Client
}

// Check implements RequirementChecker.
func (c *HTTPCheckChecker) Check(ctx context.Context, req GateRequirement, _ CheckContext) GateRequirementStatus {
	now := time.Now().UTC()

	urlStr, _ := req.Config["url"].(string)
	if urlStr == "" {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "http_check requires a 'url' in config",
			CheckedAt:   now,
		}
	}

	method, _ := req.Config["method"].(string)
	if method == "" {
		method = "GET"
	}

	expectedStatus := 200
	if v, ok := req.Config["expected_status"].(float64); ok {
		expectedStatus = int(v)
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "invalid request: " + err.Error(),
			CheckedAt:   now,
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "request failed: " + err.Error(),
			CheckedAt:   now,
		}
	}
	resp.Body.Close()

	if resp.StatusCode == expectedStatus {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   true,
			Reason:      fmt.Sprintf("endpoint returned %d", resp.StatusCode),
			CheckedAt:   now,
		}
	}

	return GateRequirementStatus{
		Requirement: req,
		Satisfied:   false,
		Reason:      fmt.Sprintf("endpoint returned %d, expected %d", resp.StatusCode, expectedStatus),
		CheckedAt:   now,
	}
}
