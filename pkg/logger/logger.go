package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type Logger struct {
	*logrus.Logger
}

func NewLogger() *Logger {
	log := logrus.New()
	
	// 设置日志格式
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})
	
	// 设置输出到标准输出
	log.SetOutput(os.Stdout)
	
	// 设置日志级别
	log.SetLevel(logrus.InfoLevel)

	return &Logger{Logger: log}
}

func (l *Logger) SetLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		l.Logger.SetLevel(logrus.DebugLevel)
	case "info":
		l.Logger.SetLevel(logrus.InfoLevel)
	case "warn":
		l.Logger.SetLevel(logrus.WarnLevel)
	case "error":
		l.Logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		l.Logger.SetLevel(logrus.FatalLevel)
	default:
		l.Logger.SetLevel(logrus.InfoLevel)
	}
}

func (l *Logger) SetFormat(format string) {
	if strings.ToLower(format) == "text" {
		l.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		l.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}
}