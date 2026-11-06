// Caminho: catraca/internal/device/notice_test.go

package device

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnNoticeRecebeAvisosMenosVivo(t *testing.T) {
	var got []string
	OnNotice = func(path, body string) { got = append(got, path+" "+body) }
	defer func() { OnNotice = nil }()

	s := &Server{}
	h := s.handler(func(context.Context, int64) Decision { return Decision{} }, func() []map[string]string { return nil })
	post := func(path, body string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d", path, rec.Code)
		}
	}
	post("/device_is_alive.fcgi", `{}`)
	post("/api/notifications/catra_event", `{"event":{"type":7}}`)

	if len(got) != 1 || got[0] != `POST /api/notifications/catra_event {"event":{"type":7}}` {
		t.Fatalf("avisos registrados: %q", got)
	}
}
