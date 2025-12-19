package logger

import (
	"log"
	"os"
)

var (
	infoLogger  = log.New(os.Stdout, "INFO:  ", log.Ldate|log.Ltime)
	warnLogger  = log.New(os.Stderr, "WARN:  ", log.Ldate|log.Ltime)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime)
	fatalLogger = log.New(os.Stderr, "FATAL: ", log.Ldate|log.Ltime)
)

// Info logs informational messages to stdout
func Info(format string, v ...interface{}) {
	infoLogger.Printf(format, v...)
}

// Warn logs warning messages to stderr
func Warn(format string, v ...interface{}) {
	warnLogger.Printf(format, v...)
}

// Error logs error messages to stderr
func Error(format string, v ...interface{}) {
	errorLogger.Printf(format, v...)
}

// Fatal logs fatal error messages to stderr and exits with status 1
func Fatal(format string, v ...interface{}) {
	fatalLogger.Printf(format, v...)
	os.Exit(1)
}
