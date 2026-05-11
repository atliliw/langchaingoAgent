package models

// TestCase — Search precision test case
type TestCase struct {
	Document       string `json:"document"`
	Query          string `json:"query"`
	ExpectedInTopK int    `json:"expected_in_top_k"`
	Description    string `json:"description"`
}

// TestResult — One test case result
type TestResult struct {
	TestCase TestCase `json:"test_case"`
	Found    bool     `json:"found"`
	Position *int     `json:"position,omitempty"`
	Score    *float32 `json:"score,omitempty"`
	Passed   bool     `json:"passed"`
}

// PrecisionReport — Full precision test report
type PrecisionReport struct {
	TotalTests     int          `json:"total_tests"`
	PassedTests    int          `json:"passed_tests"`
	PrecisionScore float32      `json:"precision_score"`
	AveragePosition float32     `json:"average_position"`
	Results        []TestResult `json:"results"`
}

// PrecisionTestQuery — Precision test query params
type PrecisionTestQuery struct {
	CustomCases bool `json:"custom_cases"`
}
