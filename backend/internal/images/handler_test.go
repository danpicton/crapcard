package images_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/images"
)

type env struct {
	db    *db.DB
	user  int64
	other int64
	cfg   images.Config
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	users := auth.NewUserRepo(database)
	ctx := context.Background()
	u, err := users.Create(ctx, "dan", "h", true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	other, err := users.Create(ctx, "someone-else", "h", false)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	return &env{db: database, user: u.ID, other: other.ID, cfg: images.DefaultConfig()}
}

func (e *env) serve(t *testing.T, userID int64, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	images.NewHandlerWith(e.db, e.cfg).Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := &auth.User{ID: userID, Username: "test"}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// pngBytes builds a real PNG so content sniffing sees a genuine image.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// uploadRequest posts raw bytes the way a clipboard paste does.
func uploadRequest(body []byte, contentType string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/images", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	return req
}

func TestUploadStoresAnImageAndReturnsItsURL(t *testing.T) {
	e := newEnv(t)
	data := pngBytes(t, 4, 4)

	rec := e.serve(t, e.user, uploadRequest(data, "image/png"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Fatalf("no image id returned")
	}
	// The URL is what gets embedded in the markdown, so it must be usable
	// as-is by the editor.
	if !strings.HasPrefix(got.URL, "/api/images/") || !strings.HasSuffix(got.URL, got.ID) {
		t.Fatalf("url = %q, want /api/images/<id>", got.URL)
	}
}

func TestUploadedImageCanBeFetchedBack(t *testing.T) {
	e := newEnv(t)
	data := pngBytes(t, 4, 4)

	rec := e.serve(t, e.user, uploadRequest(data, "image/png"))
	var created struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = e.serve(t, e.user, httptest.NewRequest(http.MethodGet, created.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fetch status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type = %q, want image/png", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("fetched bytes differ from what was uploaded")
	}
}

func TestUploadRejectsNonImageContent(t *testing.T) {
	// A declared Content-Type must not be trusted: the bytes are sniffed, so
	// an executable relabelled as a PNG is refused.
	e := newEnv(t)

	cases := map[string][]byte{
		"text":    []byte("just some text, definitely not a png"),
		"elf":     {0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0},
		"html":    []byte("<html><script>alert(1)</script></html>"),
		"empty":   {},
		"svg xml": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.serve(t, e.user, uploadRequest(body, "image/png"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for %s", rec.Code, name)
			}
		})
	}
}

func TestUploadRejectsOversizedImages(t *testing.T) {
	e := newEnv(t)
	e.cfg.MaxImageSize = 100 // bytes

	rec := e.serve(t, e.user, uploadRequest(pngBytes(t, 64, 64), "image/png"))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestUploadRejectsWhenTheUserIsOverQuota(t *testing.T) {
	e := newEnv(t)
	data := pngBytes(t, 8, 8)
	e.cfg.MaxTotalBytes = int64(len(data)) + 1

	if rec := e.serve(t, e.user, uploadRequest(data, "image/png")); rec.Code != http.StatusCreated {
		t.Fatalf("first upload status = %d, want 201", rec.Code)
	}
	if rec := e.serve(t, e.user, uploadRequest(data, "image/png")); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("second upload status = %d, want 413 once over quota", rec.Code)
	}

	// The quota is per user, so someone else is unaffected.
	if rec := e.serve(t, e.other, uploadRequest(data, "image/png")); rec.Code != http.StatusCreated {
		t.Fatalf("other user's upload status = %d, want 201", rec.Code)
	}
}

func TestImagesAreScopedToTheirOwner(t *testing.T) {
	e := newEnv(t)

	rec := e.serve(t, e.user, uploadRequest(pngBytes(t, 4, 4), "image/png"))
	var created struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = e.serve(t, e.other, httptest.NewRequest(http.MethodGet, created.URL, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another user fetched the image: status %d", rec.Code)
	}
}

func TestFetchingAMissingImageIs404(t *testing.T) {
	e := newEnv(t)

	rec := e.serve(t, e.user, httptest.NewRequest(http.MethodGet, "/api/images/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServedImagesCarryHardeningHeaders(t *testing.T) {
	// Images are user-supplied bytes served from the app's own origin, so the
	// response must not be sniffable into something executable.
	e := newEnv(t)

	rec := e.serve(t, e.user, uploadRequest(pngBytes(t, 4, 4), "image/png"))
	var created struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = e.serve(t, e.user, httptest.NewRequest(http.MethodGet, created.URL, nil))
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("Content-Security-Policy = %q, want a sandbox directive", csp)
	}
}

func TestImagesCascadeOnUserDelete(t *testing.T) {
	e := newEnv(t)

	rec := e.serve(t, e.user, uploadRequest(pngBytes(t, 4, 4), "image/png"))
	var created struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if err := auth.NewUserRepo(e.db).Delete(context.Background(), e.user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	rec = e.serve(t, e.user, httptest.NewRequest(http.MethodGet, created.URL, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("image survived its owner: status %d", rec.Code)
	}
}
