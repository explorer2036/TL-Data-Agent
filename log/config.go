package log

import (
	"TL-Data-Agent/log/lumberjack"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc/grpclog"
)

// none is used to disable logging output as well as to disable stack tracing.
const none zapcore.Level = 100

var levelToZap = map[Level]zapcore.Level{
	DebugLevel: zapcore.DebugLevel,
	InfoLevel:  zapcore.InfoLevel,
	WarnLevel:  zapcore.WarnLevel,
	ErrorLevel: zapcore.ErrorLevel,
	NoneLevel:  none,
}

func init() {
	// use our defaults for starters so that logging works even before everything is fully configured
	_ = Configure(DefaultOptions())
}

// prepZap is a utility function used by the Configure function.
func prepZap(options *Options) (zapcore.Core, zapcore.Core, zapcore.WriteSyncer, error) {
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "scope",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeTime:     formatCSTDate,
	}

	var enc zapcore.Encoder
	if options.JSONEncoding {
		enc = zapcore.NewJSONEncoder(encCfg)
	} else {
		enc = zapcore.NewConsoleEncoder(encCfg)
	}

	var rotaterSink zapcore.WriteSyncer
	if options.RotateOutputPath != "" {
		rotaterSink = zapcore.AddSync(&lumberjack.Logger{
			Filename:   options.RotateOutputPath,
			MaxSize:    options.RotationMaxSize,
			MaxBackups: options.RotationMaxAge,
			MaxAge:     options.RotationMaxBackups,
			LocalTime:  true,
		})
	}

	errSink, closeErrorSink, err := zap.Open(options.ErrorOutputPaths...)
	if err != nil {
		return nil, nil, nil, err
	}

	var outputSink zapcore.WriteSyncer
	if len(options.OutputPaths) > 0 {
		outputSink, _, err = zap.Open(options.OutputPaths...)
		if err != nil {
			closeErrorSink()
			return nil, nil, nil, err
		}
	}

	var sink zapcore.WriteSyncer
	if rotaterSink != nil && outputSink != nil {
		sink = zapcore.NewMultiWriteSyncer(outputSink, rotaterSink)
	} else if rotaterSink != nil {
		sink = rotaterSink
	} else {
		sink = outputSink
	}

	var enabler zap.LevelEnablerFunc = func(lvl zapcore.Level) bool {
		switch lvl {
		case zapcore.ErrorLevel:
			return defaultScope.ErrorEnabled()
		case zapcore.WarnLevel:
			return defaultScope.WarnEnabled()
		case zapcore.InfoLevel:
			return defaultScope.InfoEnabled()
		}
		return defaultScope.DebugEnabled()
	}

	return zapcore.NewCore(enc, sink, zap.NewAtomicLevelAt(zapcore.DebugLevel)),
		zapcore.NewCore(enc, sink, enabler),
		errSink, nil
}

func formatCSTDate(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	buf := t.Format("2006-01-02 15:04:05.000")
	enc.AppendString(buf)
}

func formatDate(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	t = t.UTC()
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	micros := t.Nanosecond() / 1000

	buf := make([]byte, 27)

	buf[0] = byte((year/1000)%10) + '0'
	buf[1] = byte((year/100)%10) + '0'
	buf[2] = byte((year/10)%10) + '0'
	buf[3] = byte(year%10) + '0'
	buf[4] = '-'
	buf[5] = byte((month)/10) + '0'
	buf[6] = byte((month)%10) + '0'
	buf[7] = '-'
	buf[8] = byte((day)/10) + '0'
	buf[9] = byte((day)%10) + '0'
	buf[10] = 'T'
	buf[11] = byte((hour)/10) + '0'
	buf[12] = byte((hour)%10) + '0'
	buf[13] = ':'
	buf[14] = byte((minute)/10) + '0'
	buf[15] = byte((minute)%10) + '0'
	buf[16] = ':'
	buf[17] = byte((second)/10) + '0'
	buf[18] = byte((second)%10) + '0'
	buf[19] = '.'
	buf[20] = byte((micros/100000)%10) + '0'
	buf[21] = byte((micros/10000)%10) + '0'
	buf[22] = byte((micros/1000)%10) + '0'
	buf[23] = byte((micros/100)%10) + '0'
	buf[24] = byte((micros/10)%10) + '0'
	buf[25] = byte((micros)%10) + '0'
	buf[26] = 'Z'

	enc.AppendString(string(buf))
}

func updateScopes(options *Options, core zapcore.Core, errSink zapcore.WriteSyncer) error {
	// init the global I/O funcs
	writeFn.Store(core.Write)
	syncFn.Store(core.Sync)
	errorSink.Store(errSink)

	// snapshot what's there
	allScopes := Scopes()

	// update the output levels of all scopes
	levels := strings.Split(options.outputLevels, ",")
	for _, sl := range levels {
		s, l, err := convertScopedLevel(sl)
		if err != nil {
			return err
		}

		if scope, ok := allScopes[s]; ok {
			scope.SetOutputLevel(l)
		} else {
			return fmt.Errorf("unknown scope '%s' specified", s)
		}
	}

	// update the stack tracing levels of all scopes
	levels = strings.Split(options.stackTraceLevels, ",")
	for _, sl := range levels {
		s, l, err := convertScopedLevel(sl)
		if err != nil {
			return err
		}

		if scope, ok := allScopes[s]; ok {
			scope.SetStackTraceLevel(l)
		} else {
			return fmt.Errorf("unknown scope '%s' specified", s)
		}
	}

	// update the caller location setting of all scopes
	sc := strings.Split(options.logCallers, ",")
	for _, s := range sc {
		if s == "" {
			continue
		}

		if scope, ok := allScopes[s]; ok {
			scope.SetLogCallers(true)
		} else {
			return fmt.Errorf("unknown scope '%s' specified", s)
		}
	}

	return nil
}

// Configure initializes Istio's logging subsystem.
//
// You typically call this once at process startup.
// Once this call returns, the logging system is ready to accept data.
func Configure(options *Options) error {
	core, captureCore, errSink, err := prepZap(options)
	if err != nil {
		return err
	}

	if err = updateScopes(options, core, errSink); err != nil {
		return err
	}

	opts := []zap.Option{
		zap.ErrorOutput(errSink),
		zap.AddCallerSkip(1),
	}

	if defaultScope.GetLogCallers() {
		opts = append(opts, zap.AddCaller())
	}

	l := defaultScope.GetStackTraceLevel()
	if l != NoneLevel {
		opts = append(opts, zap.AddStacktrace(levelToZap[l]))
	}

	captureLogger := zap.New(captureCore, opts...)

	// capture global zap logging and force it through our logger
	_ = zap.ReplaceGlobals(captureLogger)

	// capture standard golang "log" package output and force it through our logger
	_ = zap.RedirectStdLog(captureLogger)

	// capture gRPC logging
	if options.LogGrpc {
		grpclog.SetLogger(zapgrpc.NewLogger(captureLogger.WithOptions(zap.AddCallerSkip(2))))
	}

	return nil
}

// reset by the Configure method
var syncFn atomic.Value

// Sync flushes any buffered log entries.
// Processes should normally take care to call Sync before exiting.
func Sync() error {
	var err error
	if s := syncFn.Load().(func() error); s != nil {
		err = s()
	}

	return err
}