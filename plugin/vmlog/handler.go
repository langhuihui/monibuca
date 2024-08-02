package plugin_vmlog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vlinsert"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vlinsert/insertutils"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vlselect"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vlstorage"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/buildinfo"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/envflag"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/httpserver"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logstorage"
	slogcommon "github.com/samber/slog-common"
	"log/slog"
	"net/http"
	"sync"
)

var _ slog.Handler = (*VmLogHandler)(nil)

type VmLogHandler struct {
	mu        sync.RWMutex
	opts      slog.HandlerOptions
	attrs     []slog.Attr
	groups    []string
	cp        insertutils.CommonParams
	converter Converter
}

func readLine(line []byte, timeField, msgField string, lmp insertutils.LogMessageProcessor) (bool, error) {

	p := logstorage.GetJSONParser()
	if err := p.ParseLogMessage(line); err != nil {
		return false, fmt.Errorf("cannot parse json-encoded log entry: %w", err)
	}
	ts, err := insertutils.ExtractTimestampRFC3339NanoFromFields(timeField, p.Fields)
	if err != nil {
		return false, fmt.Errorf("cannot get timestamp: %w", err)
	}
	logstorage.RenameField(p.Fields, msgField, "_msg")
	lmp.AddRow(ts, p.Fields)
	logstorage.PutJSONParser(p)

	return true, nil
}

func writeLog(sc []byte, cp insertutils.CommonParams) bool {
	lr := logstorage.GetLogRows(cp.StreamFields, cp.IgnoreFields)
	lmp := cp.NewLogMessageProcessor()

	ok, err := readLine(sc, cp.TimeField, cp.MsgField, lmp)
	lmp.MustClose()
	if err != nil {
		logger.Errorf("cannot read %s", err)
		return false
	}

	if !ok {
		return false
	}

	vlstorage.MustAddRows(lr)
	logstorage.PutLogRows(lr)
	return true
}

func (h *VmLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *VmLogHandler) Handle(ctx context.Context, r slog.Record) error {
	var attrFromContext []func(ctx context.Context) []slog.Attr

	fromContext := slogcommon.ContextExtractor(ctx, attrFromContext)
	payload := h.converter(h.opts.AddSource, h.opts.ReplaceAttr, append(h.attrs, fromContext...), h.groups, &r)

	// 序列化为 JSON 并输出
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeLog(jsonData, h.cp)
	return nil
}

func (h *VmLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &VmLogHandler{
		cp:        h.cp,
		opts:      h.opts,
		attrs:     newAttrs,
		groups:    h.groups,
		converter: h.converter,
	}
}

func (h *VmLogHandler) WithGroup(name string) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &VmLogHandler{
		cp:        h.cp,
		opts:      h.opts,
		attrs:     h.attrs,
		groups:    append(append([]string(nil), h.groups...), name),
		converter: h.converter,
	}
}

func init() {
	envflag.Parse()
	buildinfo.Init()
	logger.Init()
	vlstorage.Init()
	vlselect.Init()
	vlinsert.Init()
}

func NewVmLogHandler(opts *slog.HandlerOptions, converter Converter) (*VmLogHandler, error) {

	if err := vlstorage.CanWriteData(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	}
	if converter == nil {
		converter = DefaultConverter
	}
	// 配置日志参数
	cp := insertutils.CommonParams{
		TenantID:     logstorage.TenantID{0, 0},
		TimeField:    "timestamp",
		MsgField:     "message",
		StreamFields: []string{""}, //todo 配置文件取
	}
	handler := &VmLogHandler{cp: cp, opts: *opts, converter: converter}

	return handler, nil
}

func requestHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/" {
		if r.Method != http.MethodGet {
			return false
		}
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h2>M7S VictoriaLogs</h2></br>")
		fmt.Fprintf(w, "See docs at <a href='https://docs.victoriametrics.com/victorialogs/'>https://docs.victoriametrics.com/victorialogs/</a></br>")
		fmt.Fprintf(w, "Useful endpoints:</br>")
		httpserver.WriteAPIHelp(w, [][2]string{
			{"select/vmui", "Web UI"},
			//{"metrics", "available service metrics"},
			//{"flags", "command-line flags"},
		})
		return true
	}
	if vlinsert.RequestHandler(w, r) {
		return true
	}
	if vlselect.RequestHandler(w, r) {
		return true
	}
	return false
}
