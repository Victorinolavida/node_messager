package logger

import (
	"context"
	"log"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerKey string

var (
	globalLogger     *zap.SugaredLogger
	once             sync.Once
	loggerContextKey loggerKey = loggerKey("ctx_logger")
)

func newLogger(debugMode, prettyLog bool) *zap.SugaredLogger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	if prettyLog {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	logLevel := zapcore.InfoLevel
	if debugMode {
		logLevel = zapcore.DebugLevel
	}

	consoleOutput := zapcore.Lock(os.Stdout)
	core := zapcore.NewCore(consoleEncoder, consoleOutput, logLevel)

	return zap.New(core, zap.AddCaller()).Sugar()
}

func GetGlobalLogger(params ...bool) *zap.SugaredLogger {
	once.Do(func() {
		var log *zap.SugaredLogger
		if len(params) == 0 {
			log = newLogger(true, true)
		} else if len(params) == 1 {
			log = newLogger(params[0], params[0])
		} else {
			log = newLogger(params[0], params[1])
		}
		globalLogger = log
	})
	return globalLogger
}

func CloseLogger() {
	if globalLogger != nil {
		err := globalLogger.Sync()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func GetContextLogger(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		ctx = context.Background()
	}
	log, ok := ctx.Value(loggerContextKey).(*zap.SugaredLogger)
	if !ok {
		log := newLogger(true, true)
		log.Warn("logger not found in context, creating a new one")
		return log
	}
	return log
}

func SetContextLogger(ctx context.Context, log *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, loggerContextKey, log)
}

func NewLogger(params ...bool) *zap.SugaredLogger {
	var prettyLog, debugMode bool
	if len(params) == 0 {
		prettyLog = true
		debugMode = true
	} else if len(params) == 1 {
		prettyLog = params[0]
		debugMode = params[0]
	} else {
		debugMode = params[0]
		prettyLog = params[1]
	}
	return newLogger(debugMode, prettyLog)
}
