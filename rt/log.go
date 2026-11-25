package rt

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

var (
	logger    = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	secretKey = regexp.MustCompile(`(?i)(password|token|apikey|authorization|secret)`)
)

// Log implements LOG level, message [, fields] as one JSON line with time,
// level, msg, requestId and the fields (Requirement 10.3). Secret-looking keys
// are redacted (Requirement 10.5).
// TODO(task 2.4): redact nested maps too, and send lines to the playground log stream.
func Log(c *Ctx, level Value, msg Value, fields *Map) {
	var lvl slog.Level
	switch strings.ToUpper(ToString(level)) {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "INFO":
		lvl = slog.LevelInfo
	case "WARN":
		lvl = slog.LevelWarn
	case "ERROR":
		lvl = slog.LevelError
	default:
		panic(Errorf(KindTypeMismatch, "LOG level must be DEBUG, INFO, WARN or ERROR, got %q", ToString(level)))
	}
	ctx := context.Background()
	attrs := make([]slog.Attr, 0, 8)
	if c != nil {
		ctx = c.Context()
		if c.Req != nil {
			attrs = append(attrs, slog.String("requestId", c.Req.ID))
		}
	}
	for _, k := range fields.Keys() {
		v, _ := fields.Get(k)
		if secretKey.MatchString(k) {
			v = "[REDACTED]"
		}
		attrs = append(attrs, slog.Any(k, v))
	}
	logger.LogAttrs(ctx, lvl, ToString(msg), attrs...)
}
