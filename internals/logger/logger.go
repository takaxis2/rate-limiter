package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2" // 로그 로테이션을 위한 패키지
)

var log *zap.Logger

func Init(env string) {
	// 로그 파일 경로 설정
	logPath := "logs"
	if err := os.MkdirAll(logPath, 0744); err != nil {
		panic(fmt.Sprintf("can't create log directory: %v", err))
	}

	// 현재 날짜로 로그파일 생성
	now := time.Now()
	logfile := filepath.Join(logPath, fmt.Sprintf("%s.log", now.Format("2006-01-02")))

	// 로그 로테이션 설정
	writer := &lumberjack.Logger{
		Filename:   logfile,
		MaxSize:    100, // 파일 최대 크기 (MB)
		MaxBackups: 5,   // 보관할 이전 로그 파일 수
		// MaxAge:     30,   // 로그 파일 보관 일수
		Compress: true, // 로그 파일 압축 여부
	}

	// 파일과 콘솔에 모두 로그를 출력하기 위한 설정
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 로그 레벨 설정
	var level zapcore.Level
	if env == "production" || env == "prod" {
		level = zapcore.InfoLevel
	} else {
		level = zapcore.DebugLevel
	}

	// 콘솔 출력을 위한 인코더
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	// 파일 출력을 위한 인코더
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)

	// 코어 생성
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(fileEncoder, zapcore.AddSync(writer), level),
	)

	// 로거 생성
	log = zap.New(core,
		zap.AddCaller(),                       // 호출자 정보 추가
		zap.AddStacktrace(zapcore.ErrorLevel), // 에러 발생 시 스택트레이스 추가
	)
}

// Sync flushes any buffered log entries
func Sync() error {
	return log.Sync()
}

func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}

// 필요한 경우 Warn 레벨도 추가
func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}
