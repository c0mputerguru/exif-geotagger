package logger

import (
	"log"
	"os"
)

var (
	InfoLogger  = log.New(os.Stdout, "INFO: ", log.LstdFlags)
	WarnLogger  = log.New(os.Stderr, "WARN: ", log.LstdFlags)
	ErrorLogger = log.New(os.Stderr, "ERROR: ", log.LstdFlags)
)

func Info(format string, v ...interface{}) {
	InfoLogger.Printf(format, v...)
}

func Warn(format string, v ...interface{}) {
	WarnLogger.Printf(format, v...)
}

func Error(format string, v ...interface{}) {
	ErrorLogger.Printf(format, v...)
}
