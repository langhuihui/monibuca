package plugin_vmlog

import (
	"net/http"

	"log/slog"

	slogcommon "github.com/samber/slog-common"
)

var SourceKey = "source"
var ContextKey = "extra"
var RequestKey = "request"
var ErrorKeys = []string{"error", "err"}
var RequestIgnoreHeaders = false

type Converter func(addSource bool, replaceAttr func(groups []string, a slog.Attr) slog.Attr, loggerAttr []slog.Attr, groups []string, record *slog.Record) map[string]any

func DefaultConverter(addSource bool, replaceAttr func(groups []string, a slog.Attr) slog.Attr, loggerAttr []slog.Attr, groups []string, record *slog.Record) map[string]any {
	// aggregate all attributes
	attrs := slogcommon.AppendRecordAttrsToAttrs(loggerAttr, groups, record)

	// developer formatters
	if addSource {
		attrs = append(attrs, slogcommon.Source(SourceKey, record))
	}
	attrs = slogcommon.ReplaceAttrs(replaceAttr, []string{}, attrs...)
	attrs = slogcommon.RemoveEmptyAttrs(attrs)

	// handler formatter
	extra := slogcommon.AttrsToMap(attrs...)

	payload := map[string]any{
		"timestamp": record.Time.UTC(),
		"level":     record.Level.String(),
		"message":   record.Message,
	}

	for _, errorKey := range ErrorKeys {
		if v, ok := extra[errorKey]; ok {
			if err, ok := v.(error); ok {
				payload[errorKey] = slogcommon.FormatError(err)
				delete(extra, errorKey)
				break
			}
		}
	}

	if v, ok := extra[RequestKey]; ok {
		if req, ok := v.(*http.Request); ok {
			payload[RequestKey] = slogcommon.FormatRequest(req, RequestIgnoreHeaders)
			delete(extra, RequestKey)
		}
	}

	if user, ok := extra["user"]; ok {
		payload["user"] = user
		delete(extra, "user")
	}

	payload[ContextKey] = extra

	return payload
}
