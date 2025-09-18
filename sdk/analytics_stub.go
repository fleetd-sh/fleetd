package sdk

// Stub types for SDK until proto definitions are available

type TimeRange struct {
	StartTime int64
	EndTime   int64
}

type LogLevel int32

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type AggregationType int32

const (
	AggregationTypeAvg AggregationType = iota
	AggregationTypeSum
	AggregationTypeMin
	AggregationTypeMax
)

type ReportType int32

const (
	ReportTypeSummary ReportType = iota
	ReportTypeDetailed
)

type ReportFormat int32

const (
	ReportFormatPDF ReportFormat = iota
	ReportFormatCSV
	ReportFormatJSON
)

type AlertSeverity int32

const (
	AlertSeverityInfo AlertSeverity = iota
	AlertSeverityWarning
	AlertSeverityCritical
)