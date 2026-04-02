package static_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"blackbox/server/internal/static"
	"github.com/stretchr/testify/assert"
)

func testFS() fs.FS {
	return fstest.MapFS{
		"web/dist/index.html":     {Data: []byte("<html>app</html>")},
		"web/dist/assets/main.js": {Data: []byte("console.log('hi')")},
	}
}

func TestHandler_ServesExistingFile(t *testing.T) {
	h := static.Handler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/assets/main.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_ServesIndexForRoot(t *testing.T) {
	h := static.Handler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "app")
}

func TestHandler_FallsBackToIndexForUnknownRoute(t *testing.T) {
	h := static.Handler(testFS())
	for _, path := range []string{"/setup", "/timeline", "/settings/profile"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "expected fallback for %s", path)
		assert.Contains(t, w.Body.String(), "app", "expected index.html content for %s", path)
	}
}
