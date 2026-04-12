package health

import (
	"fmt"
	"net/http"
	"sentinel/logger"
	"time"
)

// SmokeTest defines a smoke test for a container
type SmokeTest struct {
	URL            string
	ExpectedStatus int
	Timeout        int
	Retries        int
}

// SmokeResult holds smoke test result
type SmokeResult struct {
	Passed     bool
	StatusCode int
	Error      string
	Duration   time.Duration
}

// Runner runs smoke tests
type Runner struct {
	HTTPClient *http.Client
}

// NewRunner creates a new smoke test runner
func NewRunner(timeout int) *Runner {
	return &Runner{
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Run executes a smoke test
func (r *Runner) Run(test SmokeTest) SmokeResult {
	result := SmokeResult{}

	if test.ExpectedStatus == 0 {
		test.ExpectedStatus = http.StatusOK
	}
	if test.Retries == 0 {
		test.Retries = 3
	}

	for attempt := 1; attempt <= test.Retries; attempt++ {
		logger.Log.Debugf("Smoke test attempt %d/%d: %s",
			attempt,
			test.Retries,
			test.URL,
		)

		result = r.doRequest(test)

		if result.Passed {
			logger.Log.Infof("Smoke test passed: %s (%s)",
				test.URL,
				result.Duration,
			)
			return result
		}

		if attempt < test.Retries {
			logger.Log.Warnf("Smoke test failed attempt %d - retrying...", attempt)
			time.Sleep(2 * time.Second)
		}
	}

	logger.Log.Errorf("Smoke test failed after %d attempts: %s",
		test.Retries,
		result.Error,
	)

	return result
}

// RunAll runs multiple smoke tests
func (r *Runner) RunAll(tests []SmokeTest) (bool, []SmokeResult) {
	allPassed := true
	results := make([]SmokeResult, 0, len(tests))

	for _, test := range tests {
		result := r.Run(test)
		results = append(results, result)

		if !result.Passed {
			allPassed = false
		}
	}

	return allPassed, results
}

// doRequest performs the actual HTTP request
func (r *Runner) doRequest(test SmokeTest) SmokeResult {
	start := time.Now()
	result := SmokeResult{}

	resp, err := r.HTTPClient.Get(test.URL)
	result.Duration = time.Since(start)

	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Warnf("Failed to close smoke test response body: %v", err)
		}
	}()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode != test.ExpectedStatus {
		result.Passed = false
		result.Error = fmt.Sprintf(
			"expected status %d got %d",
			test.ExpectedStatus,
			resp.StatusCode,
		)
		return result
	}

	result.Passed = true
	return result
}

// BuildSmokeTest builds a smoke test from container labels
func BuildSmokeTest(labels map[string]string) *SmokeTest {
	url, ok := labels["sentinel.smoke-test.url"]
	if !ok {
		return nil
	}

	return &SmokeTest{
		URL:            url,
		ExpectedStatus: http.StatusOK,
		Timeout:        10,
		Retries:        3,
	}
}