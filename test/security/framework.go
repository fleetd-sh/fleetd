package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/stretchr/testify/require"
)

// SecurityTestFramework provides a comprehensive security testing framework
type SecurityTestFramework struct {
	Config    *TestConfig
	Results   *TestResults
	Reporter  *SecurityReporter
	Scanner   *VulnerabilityScanner
	Fuzzer    *APIFuzzer
	Penetest  *PenetrationTester
	mutex     sync.RWMutex
}

// TestConfig holds configuration for security tests
type TestConfig struct {
	TargetURL       string            `json:"target_url"`
	APIKeys         map[string]string `json:"api_keys"`
	Certificates    *CertConfig       `json:"certificates"`
	RateLimit       *RateLimitConfig  `json:"rate_limit"`
	Timeout         time.Duration     `json:"timeout"`
	MaxPayloadSize  int64             `json:"max_payload_size"`
	TLSMinVersion   uint16            `json:"tls_min_version"`
	EnabledTests    []string          `json:"enabled_tests"`
	ComplianceLevel string            `json:"compliance_level"` // "basic", "standard", "strict"
}

// CertConfig holds certificate testing configuration
type CertConfig struct {
	CAPath      string `json:"ca_path"`
	CertPath    string `json:"cert_path"`
	KeyPath     string `json:"key_path"`
	ValidateCN  bool   `json:"validate_cn"`
	CheckExpiry bool   `json:"check_expiry"`
}

// RateLimitConfig holds rate limiting test configuration
type RateLimitConfig struct {
	RequestsPerWindow int           `json:"requests_per_window"`
	WindowDuration    time.Duration `json:"window_duration"`
	BurstAllowed      int           `json:"burst_allowed"`
}

// TestResults holds all security test results
type TestResults struct {
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	TotalTests    int                    `json:"total_tests"`
	PassedTests   int                    `json:"passed_tests"`
	FailedTests   int                    `json:"failed_tests"`
	SkippedTests  int                    `json:"skipped_tests"`
	Vulnerabilities []*Vulnerability    `json:"vulnerabilities"`
	Categories    map[string]*CategoryResult `json:"categories"`
	ComplianceScore float64              `json:"compliance_score"`
	SecurityGrade string                 `json:"security_grade"`
	Recommendations []string             `json:"recommendations"`
}

// CategoryResult holds results for a security test category
type CategoryResult struct {
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	TotalTests     int              `json:"total_tests"`
	PassedTests    int              `json:"passed_tests"`
	FailedTests    int              `json:"failed_tests"`
	SkippedTests   int              `json:"skipped_tests"`
	Vulnerabilities []*Vulnerability `json:"vulnerabilities"`
	Score          float64          `json:"score"`
	Risk           string           `json:"risk"`
}

// Vulnerability represents a security vulnerability
type Vulnerability struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Severity      string    `json:"severity"`      // "Critical", "High", "Medium", "Low", "Info"
	CVSSScore     float64   `json:"cvss_score"`
	CVSSVector    string    `json:"cvss_vector"`
	Category      string    `json:"category"`
	Confidence    string    `json:"confidence"`    // "Certain", "High", "Medium", "Low"
	References    []string  `json:"references"`
	Remediation   string    `json:"remediation"`
	ImpactVector  string    `json:"impact_vector"`
	Evidence      string    `json:"evidence"`
	Timestamp     time.Time `json:"timestamp"`
	TestMethod    string    `json:"test_method"`
	AffectedURL   string    `json:"affected_url"`
	PayloadUsed   string    `json:"payload_used,omitempty"`
}

// TestCase represents a single security test case
type TestCase struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Severity    string                 `json:"severity"`
	OWASP       string                 `json:"owasp"`        // OWASP Top 10 mapping
	CWE         string                 `json:"cwe"`          // CWE ID
	Tags        []string               `json:"tags"`
	Parameters  map[string]interface{} `json:"parameters"`
	Enabled     bool                   `json:"enabled"`
	Timeout     time.Duration          `json:"timeout"`
	TestFunc    func(*SecurityTestFramework, *TestCase) *TestResult `json:"-"`
}

// TestResult holds the result of a single test
type TestResult struct {
	TestCase      *TestCase       `json:"test_case"`
	Status        string          `json:"status"`      // "PASS", "FAIL", "SKIP", "ERROR"
	StartTime     time.Time       `json:"start_time"`
	EndTime       time.Time       `json:"end_time"`
	Duration      time.Duration   `json:"duration"`
	Message       string          `json:"message"`
	Details       string          `json:"details"`
	Evidence      string          `json:"evidence"`
	Vulnerability *Vulnerability  `json:"vulnerability,omitempty"`
	HTTPRequests  []*HTTPRequest  `json:"http_requests,omitempty"`
	TLSInfo       *TLSInfo        `json:"tls_info,omitempty"`
}

// HTTPRequest holds information about HTTP requests made during testing
type HTTPRequest struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body,omitempty"`
	Response    *HTTPResponse     `json:"response,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Duration    time.Duration     `json:"duration"`
}

// HTTPResponse holds HTTP response information
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body,omitempty"`
	TLSInfo    *TLSInfo          `json:"tls_info,omitempty"`
}

// TLSInfo holds TLS connection information
type TLSInfo struct {
	Version            uint16   `json:"version"`
	CipherSuite        uint16   `json:"cipher_suite"`
	ServerCertificates []string `json:"server_certificates"`
	PeerCertificates   []string `json:"peer_certificates"`
	HandshakeComplete  bool     `json:"handshake_complete"`
	DidResume          bool     `json:"did_resume"`
	NegotiatedProtocol string   `json:"negotiated_protocol"`
}

// NewSecurityTestFramework creates a new security testing framework
func NewSecurityTestFramework(config *TestConfig) *SecurityTestFramework {
	return &SecurityTestFramework{
		Config:   config,
		Results:  &TestResults{
			Categories:      make(map[string]*CategoryResult),
			Vulnerabilities: make([]*Vulnerability, 0),
		},
		Reporter: NewSecurityReporter(),
		Scanner:  NewVulnerabilityScanner(),
		Fuzzer:   NewAPIFuzzer(),
		Penetest: NewPenetrationTester(),
	}
}

// RunSecurityTests executes all enabled security tests
func (f *SecurityTestFramework) RunSecurityTests(ctx context.Context) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.Results.StartTime = time.Now()
	defer func() {
		f.Results.EndTime = time.Now()
		f.Results.Duration = f.Results.EndTime.Sub(f.Results.StartTime)
		f.calculateSecurityScore()
	}()

	// Define all test categories
	categories := []string{
		"authentication",
		"authorization",
		"injection",
		"api_security",
		"tls_security",
		"rate_limiting",
		"input_validation",
		"session_management",
		"cryptography",
		"configuration",
	}

	// Initialize category results
	for _, category := range categories {
		f.Results.Categories[category] = &CategoryResult{
			Name:            category,
			Description:     getCategoryDescription(category),
			Vulnerabilities: make([]*Vulnerability, 0),
		}
	}

	// Get all test cases
	testCases := f.getAllTestCases()

	// Filter enabled tests
	enabledTests := f.filterEnabledTests(testCases)

	f.Results.TotalTests = len(enabledTests)

	// Run tests with timeout
	for _, testCase := range enabledTests {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			result := f.runTestCase(ctx, testCase)
			f.processTestResult(result)
		}
	}

	// Run vulnerability scanning
	if f.isTestEnabled("vulnerability_scan") {
		vulns, err := f.Scanner.ScanTarget(ctx, f.Config.TargetURL)
		if err != nil {
			return fmt.Errorf("vulnerability scanning failed: %w", err)
		}
		f.Results.Vulnerabilities = append(f.Results.Vulnerabilities, vulns...)
	}

	// Run penetration testing
	if f.isTestEnabled("penetration_test") {
		penTestResults, err := f.Penetest.RunPenetrationTests(ctx, f.Config.TargetURL)
		if err != nil {
			return fmt.Errorf("penetration testing failed: %w", err)
		}
		f.Results.Vulnerabilities = append(f.Results.Vulnerabilities, penTestResults...)
	}

	// Generate compliance report
	f.generateComplianceReport()

	return nil
}

// runTestCase executes a single test case
func (f *SecurityTestFramework) runTestCase(ctx context.Context, testCase *TestCase) *TestResult {
	result := &TestResult{
		TestCase:     testCase,
		StartTime:    time.Now(),
		HTTPRequests: make([]*HTTPRequest, 0),
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)

		if r := recover(); r != nil {
			result.Status = "ERROR"
			result.Message = fmt.Sprintf("Test panicked: %v", r)
		}
	}()

	// Set timeout context
	testCtx, cancel := context.WithTimeout(ctx, testCase.Timeout)
	defer cancel()

	// Check if context is already cancelled
	select {
	case <-testCtx.Done():
		result.Status = "SKIP"
		result.Message = "Test cancelled due to timeout"
		return result
	default:
	}

	// Execute test function
	if testCase.TestFunc != nil {
		testResult := testCase.TestFunc(f, testCase)
		return testResult
	}

	result.Status = "SKIP"
	result.Message = "No test function defined"
	return result
}

// processTestResult processes and stores test results
func (f *SecurityTestFramework) processTestResult(result *TestResult) {
	category := result.TestCase.Category
	categoryResult := f.Results.Categories[category]

	categoryResult.TotalTests++

	switch result.Status {
	case "PASS":
		f.Results.PassedTests++
		categoryResult.PassedTests++
	case "FAIL":
		f.Results.FailedTests++
		categoryResult.FailedTests++
		if result.Vulnerability != nil {
			f.Results.Vulnerabilities = append(f.Results.Vulnerabilities, result.Vulnerability)
			categoryResult.Vulnerabilities = append(categoryResult.Vulnerabilities, result.Vulnerability)
		}
	case "SKIP":
		f.Results.SkippedTests++
		categoryResult.SkippedTests++
	case "ERROR":
		f.Results.FailedTests++
		categoryResult.FailedTests++
	}
}

// calculateSecurityScore calculates overall security score
func (f *SecurityTestFramework) calculateSecurityScore() {
	if f.Results.TotalTests == 0 {
		f.Results.ComplianceScore = 0.0
		f.Results.SecurityGrade = "F"
		return
	}

	// Base score from passed tests
	baseScore := float64(f.Results.PassedTests) / float64(f.Results.TotalTests) * 100

	// Penalty for vulnerabilities
	vulnPenalty := 0.0
	for _, vuln := range f.Results.Vulnerabilities {
		switch vuln.Severity {
		case "Critical":
			vulnPenalty += 20.0
		case "High":
			vulnPenalty += 10.0
		case "Medium":
			vulnPenalty += 5.0
		case "Low":
			vulnPenalty += 1.0
		}
	}

	f.Results.ComplianceScore = baseScore - vulnPenalty
	if f.Results.ComplianceScore < 0 {
		f.Results.ComplianceScore = 0
	}

	// Assign security grade
	switch {
	case f.Results.ComplianceScore >= 95:
		f.Results.SecurityGrade = "A+"
	case f.Results.ComplianceScore >= 90:
		f.Results.SecurityGrade = "A"
	case f.Results.ComplianceScore >= 85:
		f.Results.SecurityGrade = "A-"
	case f.Results.ComplianceScore >= 80:
		f.Results.SecurityGrade = "B+"
	case f.Results.ComplianceScore >= 75:
		f.Results.SecurityGrade = "B"
	case f.Results.ComplianceScore >= 70:
		f.Results.SecurityGrade = "B-"
	case f.Results.ComplianceScore >= 65:
		f.Results.SecurityGrade = "C+"
	case f.Results.ComplianceScore >= 60:
		f.Results.SecurityGrade = "C"
	case f.Results.ComplianceScore >= 55:
		f.Results.SecurityGrade = "C-"
	case f.Results.ComplianceScore >= 50:
		f.Results.SecurityGrade = "D"
	default:
		f.Results.SecurityGrade = "F"
	}

	// Calculate category scores
	for _, category := range f.Results.Categories {
		if category.TotalTests > 0 {
			category.Score = float64(category.PassedTests) / float64(category.TotalTests) * 100

			// Assign risk level based on failures and vulnerabilities
			if len(category.Vulnerabilities) > 0 {
				criticalCount := 0
				highCount := 0
				for _, vuln := range category.Vulnerabilities {
					switch vuln.Severity {
					case "Critical":
						criticalCount++
					case "High":
						highCount++
					}
				}

				if criticalCount > 0 {
					category.Risk = "Critical"
				} else if highCount > 0 {
					category.Risk = "High"
				} else {
					category.Risk = "Medium"
				}
			} else if category.FailedTests > 0 {
				category.Risk = "Low"
			} else {
				category.Risk = "Minimal"
			}
		}
	}
}

// generateComplianceReport generates compliance recommendations
func (f *SecurityTestFramework) generateComplianceReport() {
	recommendations := make([]string, 0)

	// Check OWASP Top 10 compliance
	owaspIssues := f.checkOWASPCompliance()
	recommendations = append(recommendations, owaspIssues...)

	// Check security headers
	headerIssues := f.checkSecurityHeaders()
	recommendations = append(recommendations, headerIssues...)

	// Check TLS configuration
	tlsIssues := f.checkTLSConfiguration()
	recommendations = append(recommendations, tlsIssues...)

	// Check authentication issues
	authIssues := f.checkAuthenticationSecurity()
	recommendations = append(recommendations, authIssues...)

	f.Results.Recommendations = recommendations
}

// isTestEnabled checks if a specific test is enabled
func (f *SecurityTestFramework) isTestEnabled(testName string) bool {
	if len(f.Config.EnabledTests) == 0 {
		return true // All tests enabled by default
	}

	for _, enabled := range f.Config.EnabledTests {
		if enabled == testName || enabled == "all" {
			return true
		}
	}
	return false
}

// filterEnabledTests filters test cases based on configuration
func (f *SecurityTestFramework) filterEnabledTests(testCases []*TestCase) []*TestCase {
	if len(f.Config.EnabledTests) == 0 {
		return testCases // All tests enabled
	}

	enabled := make([]*TestCase, 0)
	for _, testCase := range testCases {
		if f.isTestEnabled(testCase.ID) || f.isTestEnabled(testCase.Category) {
			enabled = append(enabled, testCase)
		}
	}
	return enabled
}

// Helper functions

func getCategoryDescription(category string) string {
	descriptions := map[string]string{
		"authentication": "Tests for authentication bypass, weak credentials, and session management",
		"authorization":  "Tests for privilege escalation, access control bypass, and RBAC issues",
		"injection":      "Tests for SQL, NoSQL, Command, LDAP, and other injection vulnerabilities",
		"api_security":   "Tests for API-specific vulnerabilities including XXE, SSRF, and mass assignment",
		"tls_security":   "Tests for TLS/SSL configuration, certificate validation, and cipher strength",
		"rate_limiting":  "Tests for rate limiting bypass and DoS protection",
		"input_validation": "Tests for input validation, output encoding, and data sanitization",
		"session_management": "Tests for session fixation, hijacking, and timeout issues",
		"cryptography":   "Tests for weak encryption, key management, and cryptographic implementation",
		"configuration":  "Tests for security misconfigurations and default credentials",
	}

	if desc, exists := descriptions[category]; exists {
		return desc
	}
	return "Security tests for " + category
}

// GenerateTestCertificate generates a test certificate for testing
func GenerateTestCertificate(cn string) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return cert, priv, nil
}

// CreateTestHTTPClient creates an HTTP client configured for security testing
func (f *SecurityTestFramework) CreateTestHTTPClient() *http.Client {
	return &http.Client{
		Timeout: f.Config.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         f.Config.TLSMinVersion,
				InsecureSkipVerify: true, // For testing only
			},
		},
	}
}

// RecordHTTPRequest records an HTTP request for analysis
func (f *SecurityTestFramework) RecordHTTPRequest(req *http.Request, resp *http.Response, duration time.Duration) *HTTPRequest {
	httpReq := &HTTPRequest{
		Method:    req.Method,
		URL:       req.URL.String(),
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Duration:  duration,
	}

	// Copy headers
	for k, v := range req.Header {
		if len(v) > 0 {
			httpReq.Headers[k] = v[0]
		}
	}

	// Record response if available
	if resp != nil {
		httpReq.Response = &HTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    make(map[string]string),
		}

		for k, v := range resp.Header {
			if len(v) > 0 {
				httpReq.Response.Headers[k] = v[0]
			}
		}

		// Record TLS info if available
		if resp.TLS != nil {
			httpReq.Response.TLSInfo = &TLSInfo{
				Version:            resp.TLS.Version,
				CipherSuite:        resp.TLS.CipherSuite,
				HandshakeComplete:  resp.TLS.HandshakeComplete,
				DidResume:          resp.TLS.DidResume,
				NegotiatedProtocol: resp.TLS.NegotiatedProtocol,
			}
		}
	}

	return httpReq
}