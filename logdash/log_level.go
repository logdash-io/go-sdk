package logdash

// logLevel represents the severity level of a log message.
type logLevel string

const (
	// logLevelError represents error messages.
	logLevelError logLevel = "error"
	// logLevelWarn represents warning messages.
	logLevelWarn logLevel = "warning"
	// logLevelInfo represents informational messages.
	logLevelInfo logLevel = "info"
	// logLevelHTTP represents HTTP-related messages.
	logLevelHTTP logLevel = "http"
	// logLevelVerbose represents verbose level messages.
	logLevelVerbose logLevel = "verbose"
	// logLevelDebug represents debug level messages.
	logLevelDebug logLevel = "debug"
	// logLevelSilly represents the lowest priority log level.
	logLevelSilly logLevel = "silly"
)
