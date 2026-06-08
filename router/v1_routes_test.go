package router

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

type routeSignature struct {
	Method string
	Path   string
}

func newV1RouteTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	// 关闭系统性能保护，避免测试机瞬时负载导致 /v1 路由被 503 提前拦截。
	common.SetPerformanceMonitorConfig(common.PerformanceMonitorConfig{
		Enabled: false,
	})
	engine := gin.New()
	store := cookie.NewStore([]byte("router-test-secret"))
	engine.Use(sessions.Sessions("session", store))
	SetDashboardRouter(engine)
	SetRelayRouter(engine)
	SetVideoRouter(engine)
	return engine
}

func expectedV1Routes() []routeSignature {
	return []routeSignature{
		{Method: http.MethodGet, Path: "/v1/dashboard/billing/subscription"},
		{Method: http.MethodGet, Path: "/v1/dashboard/billing/usage"},
		{Method: http.MethodGet, Path: "/v1/models"},
		{Method: http.MethodGet, Path: "/v1/models/:model"},
		{Method: http.MethodGet, Path: "/v1/realtime"},
		{Method: http.MethodPost, Path: "/v1/messages"},
		{Method: http.MethodPost, Path: "/v1/completions"},
		{Method: http.MethodPost, Path: "/v1/chat/completions"},
		{Method: http.MethodPost, Path: "/v1/responses"},
		{Method: http.MethodPost, Path: "/v1/responses/compact"},
		{Method: http.MethodPost, Path: "/v1/edits"},
		{Method: http.MethodPost, Path: "/v1/images/generations"},
		{Method: http.MethodPost, Path: "/v1/images/edits"},
		{Method: http.MethodPost, Path: "/v1/embeddings"},
		{Method: http.MethodPost, Path: "/v1/audio/transcriptions"},
		{Method: http.MethodPost, Path: "/v1/audio/translations"},
		{Method: http.MethodPost, Path: "/v1/audio/speech"},
		{Method: http.MethodPost, Path: "/v1/rerank"},
		{Method: http.MethodPost, Path: "/v1/engines/:model/embeddings"},
		{Method: http.MethodPost, Path: "/v1/models/*path"},
		{Method: http.MethodPost, Path: "/v1/moderations"},
		{Method: http.MethodPost, Path: "/v1/images/variations"},
		{Method: http.MethodGet, Path: "/v1/files"},
		{Method: http.MethodPost, Path: "/v1/files"},
		{Method: http.MethodDelete, Path: "/v1/files/:id"},
		{Method: http.MethodGet, Path: "/v1/files/:id"},
		{Method: http.MethodGet, Path: "/v1/files/:id/content"},
		{Method: http.MethodPost, Path: "/v1/fine-tunes"},
		{Method: http.MethodGet, Path: "/v1/fine-tunes"},
		{Method: http.MethodGet, Path: "/v1/fine-tunes/:id"},
		{Method: http.MethodPost, Path: "/v1/fine-tunes/:id/cancel"},
		{Method: http.MethodGet, Path: "/v1/fine-tunes/:id/events"},
		{Method: http.MethodDelete, Path: "/v1/models/:model"},
		{Method: http.MethodGet, Path: "/v1beta/models"},
		{Method: http.MethodGet, Path: "/v1beta/openai/models"},
		{Method: http.MethodPost, Path: "/v1beta/models/*path"},
		{Method: http.MethodGet, Path: "/v1/videos/:task_id/content"},
		{Method: http.MethodPost, Path: "/v1/video/generations"},
		{Method: http.MethodGet, Path: "/v1/video/generations/:task_id"},
		{Method: http.MethodPost, Path: "/v1/videos/:video_id/remix"},
		{Method: http.MethodPost, Path: "/v1/videos"},
		{Method: http.MethodGet, Path: "/v1/videos/:task_id"},
	}
}

func collectActualV1Routes(engine *gin.Engine) []routeSignature {
	routes := make([]routeSignature, 0)
	for _, route := range engine.Routes() {
		if strings.HasPrefix(route.Path, "/v1") {
			routes = append(routes, routeSignature{
				Method: route.Method,
				Path:   route.Path,
			})
		}
	}
	sortRouteSignatures(routes)
	return routes
}

func sortRouteSignatures(routes []routeSignature) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
}

func routeSignatureStrings(routes []routeSignature) []string {
	result := make([]string, 0, len(routes))
	for _, route := range routes {
		result = append(result, route.Method+" "+route.Path)
	}
	sort.Strings(result)
	return result
}

func normalizeRequestPath(routePath string) string {
	replacer := strings.NewReplacer(
		":model", "mock-model",
		":id", "1",
		":task_id", "task-1",
		":video_id", "video-1",
		"*path", "/gemini-2.5-flash:generateContent",
	)
	return replacer.Replace(routePath)
}

func TestV1RoutesRegistered(t *testing.T) {
	engine := newV1RouteTestEngine()

	expected := expectedV1Routes()
	sortRouteSignatures(expected)

	actual := collectActualV1Routes(engine)

	expectedStrings := routeSignatureStrings(expected)
	actualStrings := routeSignatureStrings(actual)

	if len(expectedStrings) != len(actualStrings) {
		t.Fatalf("`/v1` 路由数量不匹配，expected=%d actual=%d\nexpected=%v\nactual=%v",
			len(expectedStrings), len(actualStrings), expectedStrings, actualStrings)
	}

	for i := range expectedStrings {
		if expectedStrings[i] != actualStrings[i] {
			t.Fatalf("`/v1` 路由清单不匹配\nexpected=%v\nactual=%v", expectedStrings, actualStrings)
		}
	}
}

func TestV1RoutesSmoke(t *testing.T) {
	engine := newV1RouteTestEngine()

	for _, route := range expectedV1Routes() {
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			targetPath := normalizeRequestPath(route.Path)

			var bodyReader strings.Reader
			if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch {
				bodyReader = *strings.NewReader(`{"model":"mock-model","messages":[{"role":"user","content":"hello"}]}`)
			}

			req := httptest.NewRequest(route.Method, targetPath, &bodyReader)
			if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch {
				req.Header.Set("Content-Type", "application/json")
			}

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			if recorder.Code == http.StatusNotFound || recorder.Code == http.StatusMethodNotAllowed {
				t.Fatalf("路由未命中，status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
