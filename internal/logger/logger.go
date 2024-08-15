package logger

import (
	"fmt"
	"log"
	"os"
)

const (
	DEBUG = 4
	INFO  = 3
	WARN  = 2
	ERROR = 1
)

type Logger struct {
	DebugLogger *log.Logger
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
	ErrorLogger *log.Logger
	LogLevel    byte
}

func New(LogLevel byte, packageStr string) *Logger {
	flags := log.LstdFlags | log.Lshortfile
	logger := &Logger{
		DebugLogger: log.New(os.Stdout, fmt.Sprintf("%s: DEBUG: ", packageStr), flags),
		InfoLogger:  log.New(os.Stdout, fmt.Sprintf("%s: INFO: ", packageStr), flags),
		WarnLogger:  log.New(os.Stdout, fmt.Sprintf("%s: WARN: ", packageStr), flags),
		ErrorLogger: log.New(os.Stdout, fmt.Sprintf("%s: ERROR: ", packageStr), flags),
	}

	if LogLevel < 1 || LogLevel > 4 {
		fmt.Printf("Unknown log level: %d; Must be: DEBUG(4) / INFO(3) / WARN(2) / ERROR(1)\n", LogLevel)
		logger.LogLevel = DEBUG
	} else {
		logger.LogLevel = LogLevel
	}
	fmt.Printf("Started logger for %s: Level: %d\n", packageStr, logger.LogLevel)
	return logger
}

func (lg *Logger) Debug(message string) {
	if lg.LogLevel >= DEBUG {
		lg.DebugLogger.Print(message)
	}
}

func (lg *Logger) Info(message string) {
	if lg.LogLevel >= INFO {
		lg.InfoLogger.Print(message)
	}
}

func (lg *Logger) Warn(message string) {
	if lg.LogLevel >= WARN {
		lg.WarnLogger.Print(message)
	}
}

func (lg *Logger) Error(message string) {
	lg.ErrorLogger.Print(message)
}

func (lg *Logger) Fatal(message string) {
	lg.ErrorLogger.Fatal(message)
}

func (lg *Logger) Panic(message string) {
	lg.ErrorLogger.Panic(message)
}
