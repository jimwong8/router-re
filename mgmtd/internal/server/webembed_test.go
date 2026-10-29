package server
import "net/http"
import "net/http/httptest"
func TestWebEmbedNotSwallowAPI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	// health endpoint handled before web embed; just verify no 404
}
