// Caminho: catraca/internal/device/set_image_test.go

package device

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// iDFace de mentira: login, e user_set_image aceitando só o usuário 7.
func fakeIDFace(t *testing.T, got *[]byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login.fcgi":
			io.WriteString(w, `{"session":"abc"}`)
		case "/user_set_image.fcgi":
			if r.URL.Query().Get("session") != "abc" || r.Header.Get("Content-Type") != "application/octet-stream" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			*got, _ = io.ReadAll(r.Body)
			if r.URL.Query().Get("user_id") == "7" {
				io.WriteString(w, `{"user_id":7,"success":true}`)
				return
			}
			io.WriteString(w, `{"success":false,"errors":[{"code":2,"message":"Face muito pequena"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSetUserImage(t *testing.T) {
	var got []byte
	srv := fakeIDFace(t, &got)
	defer srv.Close()
	c := NewControlID(strings.TrimPrefix(srv.URL, "http://"), "admin", "admin", 0)

	if err := c.SetUserImage(context.Background(), 7, []byte("JPEGDATA")); err != nil || string(got) != "JPEGDATA" {
		t.Fatalf("aceita: %v %q", err, got)
	}
	err := c.SetUserImage(context.Background(), 8, []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "Face muito pequena") {
		t.Fatalf("recusa deveria trazer o motivo do aparelho: %v", err)
	}
}
