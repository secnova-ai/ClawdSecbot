// Package logging provides a thread-safe logger with file rotation support.
// It is designed to work with Flutter FFI to share the same log directory.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxFileSize           = 20 * 1024 * 1024 // 20MB per file
	maxBackupFiles        = 2
	goLogFileName         = "go_proxy.log"
	goHistoryLogFileName  = "go_history.log"
	goShepherdGateLogFile = "go_shepherdgate.log"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
)

var levelNames = []string{"DEBUG", "INFO", "WARNING", "ERROR"}

// Logger is a thread-safe logger with file rotation support
type Logger struct {
	file         *os.File
	mu           sync.Mutex
	logPath      string
	logDir       string
	level        LogLevel
	bytesWritten int64
}

var (
	defaultLogger      *Logger
	historyLogger      *Logger
	shepherdGateLogger *Logger
	loggerMu           sync.RWMutex
	historyMu          sync.RWMutex
	shepherdGateMu     sync.RWMutex
)

// InitLogger initializes the global logger with the given log directory
func InitLogger(logDir string, level LogLevel) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if defaultLogger != nil {
		// Close existing logger
		defaultLogger.Close()
	}

	logger, err := NewLogger(logDir, level)
	if err != nil {
		return err
	}

	defaultLogger = logger
	return nil
}

// InitHistoryLogger initializes the history logger with the given log directory.
func InitHistoryLogger(logDir string, level LogLevel) error {
	historyMu.Lock()
	defer historyMu.Unlock()

	if historyLogger != nil {
		historyLogger.Close()
	}

	logger, err := NewNamedLogger(logDir, level, goHistoryLogFileName)
	if err != nil {
		return err
	}

	historyLogger = logger
	return nil
}

// InitShepherdGateLogger initializes the ShepherdGate logger with the given log directory.
func InitShepherdGateLogger(logDir string, level LogLevel) error {
	shepherdGateMu.Lock()
	defer shepherdGateMu.Unlock()

	if shepherdGateLogger != nil {
		shepherdGateLogger.Close()
	}

	logger, err := NewNamedLogger(logDir, level, goShepherdGateLogFile)
	if err != nil {
		return err
	}

	shepherdGateLogger = logger
	return nil
}

// NewLogger creates a new logger instance
func NewLogger(logDir string, level LogLevel) (*Logger, error) {
	return NewNamedLogger(logDir, level, goLogFileName)
}

// NewNamedLogger creates a new logger instance with a custom file name.
func NewNamedLogger(logDir string, level LogLevel, fileName string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, fileName)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	return &Logger{
		file:         file,
		logPath:      logPath,
		logDir:       logDir,
		level:        level,
		bytesWritten: stat.Size(),
	}, nil
}

// SetLevel changes the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log writes a log message at the specified level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format(time.RFC3339)
	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("[%s] [%s] %s\n", timestamp, levelNames[level], message)

	n, err := l.file.WriteString(logLine)
	if err != nil {
		// Fallback to stderr if file write fails
		fmt.Fprintf(os.Stderr, "Logger write error: %v, message: %s", err, logLine)
		return
	}

	l.bytesWritten += int64(n)

	// Check if rotation is needed
	if l.bytesWritten >= maxFileSize {
		l.rotateUnlocked()
	}
}

// rotateUnlocked performs log rotation (must be called with lock held)
func (l *Logger) rotateUnlocked() {
	// Close current file
	l.file.Sync()
	l.file.Close()

	// Rotate backup files: .2 -> delete, .1 -> .2
	for i := maxBackupFiles; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.logPath, i)
		if i == maxBackupFiles {
			os.Remove(oldPath)
		} else {
			newPath := fmt.Sprintf("%s.%d", l.logPath, i+1)
			os.Rename(oldPath, newPath)
		}
	}

	// Rename current file to .1
	os.Rename(l.logPath, l.logPath+".1")

	// Create new log file
	file, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create new log file after rotation: %v\n", err)
		return
	}

	l.file = file
	l.bytesWritten = 0
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warning logs a warning message
func (l *Logger) Warning(format string, args ...interface{}) {
	l.log(WARNING, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Close closes the logger and releases resources
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Sync()
		l.file.Close()
		l.file = nil
	}
}

// Global convenience functions

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	loggerMu.RLock()
	logger := defaultLogger
	loggerMu.RUnlock()

	if logger != nil {
		logger.Debug(format, args...)
	}
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	loggerMu.RLock()
	logger := defaultLogger
	loggerMu.RUnlock()

	if logger != nil {
		logger.Info(format, args...)
	}
}

// Warning logs a warning message using the default logger
func Warning(format string, args ...interface{}) {
	loggerMu.RLock()
	logger := defaultLogger
	loggerMu.RUnlock()

	if logger != nil {
		logger.Warning(format, args...)
	}
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	loggerMu.RLock()
	logger := defaultLogger
	loggerMu.RUnlock()

	if logger != nil {
		logger.Error(format, args...)
	}
}

// Close closes the default logger
func Close() {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if defaultLogger != nil {
		defaultLogger.Close()
		defaultLogger = nil
	}
}

// HistoryInfo logs an info message using the history logger.
func HistoryInfo(format string, args ...interface{}) {
	historyMu.RLock()
	logger := historyLogger
	historyMu.RUnlock()

	if logger != nil {
		logger.Info(format, args...)
	}
}

// ShepherdGateInfo logs an info message using the ShepherdGate logger.
func ShepherdGateInfo(format string, args ...interface{}) {
	shepherdGateMu.RLock()
	logger := shepherdGateLogger
	shepherdGateMu.RUnlock()

	if logger != nil {
		logger.Info(format, args...)
	}
}

// ShepherdGateDebug logs a debug message using the ShepherdGate logger.
func ShepherdGateDebug(format string, args ...interface{}) {
	shepherdGateMu.RLock()
	logger := shepherdGateLogger
	shepherdGateMu.RUnlock()

	if logger != nil {
		logger.Debug(format, args...)
	}
}

// ShepherdGateWarning logs a warning message using the ShepherdGate logger.
func ShepherdGateWarning(format string, args ...interface{}) {
	shepherdGateMu.RLock()
	logger := shepherdGateLogger
	shepherdGateMu.RUnlock()

	if logger != nil {
		logger.Warning(format, args...)
	}
}

// ShepherdGateError logs an error message using the ShepherdGate logger.
func ShepherdGateError(format string, args ...interface{}) {
	shepherdGateMu.RLock()
	logger := shepherdGateLogger
	shepherdGateMu.RUnlock()

	if logger != nil {
		logger.Error(format, args...)
	}
}

// CloseHistory closes the history logger.
func CloseHistory() {
	historyMu.Lock()
	defer historyMu.Unlock()

	if historyLogger != nil {
		historyLogger.Close()
		historyLogger = nil
	}
}

// CloseShepherdGate closes the ShepherdGate logger.
func CloseShepherdGate() {
	shepherdGateMu.Lock()
	defer shepherdGateMu.Unlock()

	if shepherdGateLogger != nil {
		shepherdGateLogger.Close()
		shepherdGateLogger = nil
	}
}

// IsInitialized returns true if the logger has been initialized
func IsInitialized() bool {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return defaultLogger != nil
}
