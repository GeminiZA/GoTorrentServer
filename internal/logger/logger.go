package logger

import (
	"fmt"
	"log"
	"os"
)

type Logger struct {
	DebugLogger *log.Logger
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
	ErrorLogger *log.Logger
	LogLevel    byte
}

func New(LogLevel string, packageStr string) *Logger {
	flags := log.LstdFlags | log.Lshortfile
	logger := &Logger{
		DebugLogger: log.New(os.Stdout, fmt.Sprintf("%s: DEBUG: ", packageStr), flags),
		InfoLogger:  log.New(os.Stdout, fmt.Sprintf("%s: INFO: ", packageStr), flags),
		WarnLogger:  log.New(os.Stdout, fmt.Sprintf("%s: WARN: ", packageStr), flags),
		ErrorLogger: log.New(os.Stdout, fmt.Sprintf("%s: ERROR: ", packageStr), flags),
	}

	switch LogLevel {
	case "DEBUG":
		logger.LogLevel = 4
	case "INFO":
		logger.LogLevel = 3
	case "WARN":
		logger.LogLevel = 2
	case "ERROR":
		logger.LogLevel = 1
	default:
		fmt.Printf("Unknown log level: %s; Must be: DEBUG / INFO / WARN / ERROR\n", LogLevel)
		logger.LogLevel = 4
	}
	return logger
}

func (lg *Logger) Debug(message string) {
	if lg.LogLevel <= 4 {
		lg.DebugLogger.Print(message)
	}
}

func (lg *Logger) Info(message string) {
	if lg.LogLevel <= 3 {
		lg.InfoLogger.Print(message)
	}
}

func (lg *Logger) Warn(message string) {
	if lg.LogLevel <= 2 {
		lg.WarnLogger.Print(message)
	}
}

func (lg *Logger) Error(message string) {
	if lg.LogLevel <= 1 {
		lg.ErrorLogger.Print(message)
	}
}

func (lg *Logger) Fatal(message string) {
	lg.ErrorLogger.Fatal(message)
}

func (lg *Logger) Panic(message string) {
	lg.ErrorLogger.Panic(message)
}
