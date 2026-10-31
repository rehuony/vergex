package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// Log is the global logger instance
	Log *logrus.Logger
)

// compactFormatter is a custom formatter for cleaner log output
type compactFormatter struct {
	logrus.TextFormatter
}

func (f *compactFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	level := strings.ToUpper(entry.Level.String())[0:4]
	timestamp := entry.Time.Format("01-02 15:04:05")

	// Skip frames to find actual caller (skip logrus + our wrapper functions)
	caller := ""
	for i := 3; i < 10; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		// Skip logrus internal and our logger.go
		if !strings.Contains(file, "logrus") && !strings.HasSuffix(file, "logger/logger.go") {
			// Get package name from path (e.g., "vergex/internal/manager/trader_manager.go" -> "manager")
			dir := filepath.Dir(file)
			pkg := filepath.Base(dir)
			caller = fmt.Sprintf("%s/%s:%d", pkg, filepath.Base(file), line)
			break
		}
	}

	msg := fmt.Sprintf("%s [%s] %s %s\n", timestamp, level, caller, entry.Message)
	return []byte(msg), nil
}

func init() {
	// Auto-initialize default logger to ensure it works before Init is called
	Log = logrus.New()
	Log.SetLevel(logrus.InfoLevel)
	Log.SetFormatter(&compactFormatter{})
	Log.SetOutput(os.Stdout)
}

// ============================================================================
// Initialization functions
// ============================================================================

// Init initializes the global logger
// If config is nil, uses default configuration (console output, info level)
func Init(cfg *Config) error {
	Log = logrus.New()

	// Use default values if no config provided
	if cfg == nil {
		cfg = &Config{Level: "info"}
	}

	// Set default values
	cfg.SetDefaults()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	Log.SetLevel(level)

	// Set compact formatter
	Log.SetFormatter(&compactFormatter{})

	// Setup log file output (write to both stdout and file)
	logDir := "data"
	if err := os.MkdirAll(logDir, 0755); err == nil {
		logFileName := filepath.Join(logDir, fmt.Sprintf("vergex_%s.log", time.Now().Format("2006-01-02")))
		f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			// Write to both stdout and file
			Log.SetOutput(io.MultiWriter(os.Stdout, f))
		} else {
			Log.SetOutput(os.Stdout)
		}
	} else {
		Log.SetOutput(os.Stdout)
	}

	Log.SetReportCaller(true)

	return nil
}

// ============================================================================
// Logging functions
// ============================================================================

func Info(args ...interface{}) {
	Log.Info(args...)
}

func Debugf(format string, args ...interface{}) {
	Log.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	Log.Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	Log.Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	Log.Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	Log.Fatalf(format, args...)
}

// ============================================================================
// LLM logger adapter
// ============================================================================

// LLMLogger adapter that allows the llm package to use the global logger
// Implements llm.Logger interface
type LLMLogger struct{}

// NewLLMLogger creates the llm log adapter
func NewLLMLogger() *LLMLogger {
	return &LLMLogger{}
}

func (l *LLMLogger) Debugf(format string, args ...any) {
	Log.Debugf(format, args...)
}

func (l *LLMLogger) Infof(format string, args ...any) {
	Log.Infof(format, args...)
}

func (l *LLMLogger) Warnf(format string, args ...any) {
	Log.Warnf(format, args...)
}

func (l *LLMLogger) Errorf(format string, args ...any) {
	Log.Errorf(format, args...)
}
