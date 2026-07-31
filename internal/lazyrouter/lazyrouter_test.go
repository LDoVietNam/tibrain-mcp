package lazyrouter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockTool is a minimal Tool implementation used throughout the tests.
type mockTool struct {
	name        string
	description string
	calls       int32
	returnVal   interface{}
	returnErr   error
}

func (m *mockTool) Name() string         { return m.name }
func (m *mockTool) Description() string  { return m.description }
func (m *mockTool) IsLoaded() bool       { return true }

func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (interface{}, error) {
	atomic.AddInt32(&m.calls, 1)
	return m.returnVal, m.returnErr
}

func testCtx() context.Context { return context.Background() }

// ---------------------------------------------------------------------------
// 1. NewLazyTool
// ---------------------------------------------------------------------------

func TestNewLazyTool(t *testing.T) {
	t.Parallel()

	loader := func(_ string) (Tool, error) { return nil, nil }
	tool := NewLazyTool("my-tool", "does something", loader)

	if tool == nil {
		t.Fatal("NewLazyTool returned nil")
	}
	if tool.Name() != "my-tool" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "my-tool")
	}
	if tool.Description() != "does something" {
		t.Errorf("Description() = %q, want %q", tool.Description(), "does something")
	}
	if tool.IsLoaded() {
		t.Error("tool should not be loaded immediately after creation")
	}
	if tool.tool != nil {
		t.Error("internal tool field should be nil until first load")
	}
}

// ---------------------------------------------------------------------------
// 2. Lazy loading on first Execute
// ---------------------------------------------------------------------------

func TestLazyTool_LoaderCalledOnce(t *testing.T) {
	t.Parallel()

	var callCount int32
	loader := func(_ string) (Tool, error) {
		atomic.AddInt32(&callCount, 1)
		return &mockTool{name: "loaded"}, nil
	}

	tool := NewLazyTool("lazy", "desc", loader)

	// Not loaded before any Execute.
	if tool.IsLoaded() {
		t.Fatal("IsLoaded should be false before first Execute")
	}

	// First Execute triggers the loader.
	if _, err := tool.Execute(testCtx(), nil); err != nil {
		t.Fatalf("first Execute failed: %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Errorf("loader called %d times, want 1", got)
	}
	if !tool.IsLoaded() {
		t.Error("IsLoaded should be true after first Execute")
	}
	if tool.tool == nil {
		t.Error("internal tool must be set after load")
	}

	// Subsequent Execute calls must reuse the loaded tool, not call loader again.
	for i := 0; i < 3; i++ {
		if _, err := tool.Execute(testCtx(), nil); err != nil {
			t.Fatalf("Execute #%d failed: %v", i+2, err)
		}
	}
	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Errorf("loader called %d times after repeated Execute, want 1", got)
	}
}

func TestLazyTool_ExecuteDelegatesToLoadedTool(t *testing.T) {
	t.Parallel()

	inner := &mockTool{name: "inner", returnVal: "result"}
	loader := func(_ string) (Tool, error) { return inner, nil }

	tool := NewLazyTool("delegating", "d", loader)
	got, err := tool.Execute(testCtx(), map[string]interface{}{"k": "v"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got != "result" {
		t.Errorf("Execute returned %v, want 'result'", got)
	}
	if inner.calls != 1 {
		t.Errorf("inner tool called %d times, want 1", inner.calls)
	}
}

func TestLazyTool_ExecuteLoaderErrorPropagates(t *testing.T) {
	t.Parallel()

	loadErr := errors.New("load boom")
	var callCount int32
	loader := func(_ string) (Tool, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, loadErr
	}

	tool := NewLazyTool("failing", "d", loader)

	if _, err := tool.Execute(testCtx(), nil); err != loadErr {
		t.Fatalf("Execute error = %v, want %v", err, loadErr)
	}
	// Loader error must leave the tool in a usable (un-loaded) state so a
	// future retry can attempt loading again.
	if tool.IsLoaded() {
		t.Error("tool should remain unloaded when loader fails")
	}
	if tool.tool != nil {
		t.Error("internal tool must remain nil when loader fails")
	}

	// A subsequent Execute should retry the loader.
	loader2 := func(_ string) (Tool, error) {
		return &mockTool{name: "recovered"}, nil
	}
	tool.loader = loader2
	if _, err := tool.Execute(testCtx(), nil); err != nil {
		t.Fatalf("retry Execute failed: %v", err)
	}
	if !tool.IsLoaded() {
		t.Error("tool should be loaded after successful retry")
	}
}

// ---------------------------------------------------------------------------
// 3. Router.Register / Enable / Disable / GetTool / ListTools / ToolStatus
// ---------------------------------------------------------------------------

func TestRouter_Register(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tool := NewLazyTool("alpha", "a tool", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(tool, true)

	// Registered tool is retrievable and listed when enabled.
	got, ok := r.GetTool("alpha")
	if !ok {
		t.Fatal("GetTool returned false for registered+enabled tool")
	}
	if got.Name() != "alpha" {
		t.Errorf("GetTool name = %q, want %q", got.Name(), "alpha")
	}
	if names := r.ListTools(); len(names) != 1 || names[0] != "alpha" {
		t.Errorf("ListTools = %v, want [alpha]", names)
	}
}

func TestRouter_EnableDisable(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	enabledTool := NewLazyTool("on", "on", func(_ string) (Tool, error) { return &mockTool{}, nil })
	disabledTool := NewLazyTool("off", "off", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(enabledTool, true)
	r.Register(disabledTool, false)

	// Initially enabled tool is listed and retrievable; disabled is not.
	names := sortedNames(t, r.ListTools())
	if len(names) != 1 || names[0] != "on" {
		t.Errorf("initial ListTools = %v, want [on]", names)
	}
	if _, ok := r.GetTool("off"); ok {
		t.Error("GetTool should return false for disabled tool")
	}

	// Enable "off" and verify it becomes visible.
	r.Enable("off")
	if _, ok := r.GetTool("off"); !ok {
		t.Error("GetTool should return true after Enable")
	}
	names = sortedNames(t, r.ListTools())
	if len(names) != 2 {
		t.Errorf("after Enable, ListTools = %v, want 2 tools", names)
	}

	// Disable "on" and verify it leaves the list.
	r.Disable("on")
	if _, ok := r.GetTool("on"); ok {
		t.Error("GetTool should return false after Disable")
	}
	names = sortedNames(t, r.ListTools())
	if len(names) != 1 || names[0] != "off" {
		t.Errorf("after Disable, ListTools = %v, want [off]", names)
	}

	// Enabling a non-existent name should be a no-op (not panic).
	r.Enable("does-not-exist")
	r.Disable("does-not-exist")
}

func TestRouter_GetTool_MissingAndDisabled(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tool := NewLazyTool("present", "d", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(tool, false)

	if _, ok := r.GetTool("absent"); ok {
		t.Error("GetTool on absent name should return false")
	}
	if _, ok := r.GetTool("present"); ok {
		t.Error("GetTool should return false while disabled")
	}
	r.Enable("present")
	if _, ok := r.GetTool("present"); !ok {
		t.Error("GetTool should return true after enabling")
	}
}

func TestRouter_ToolStatus(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	loaded := NewLazyTool("loaded", "d", func(_ string) (Tool, error) {
		return &mockTool{name: "loaded"}, nil
	})
	disabled := NewLazyTool("disabled", "d", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(loaded, true)
	r.Register(disabled, false)

	// Trigger a load so the loaded flag flips.
	if _, err := loaded.Execute(testCtx(), nil); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	status := r.ToolStatus()
	if len(status) != 2 {
		t.Fatalf("ToolStatus returned %d tools, want 2", len(status))
	}

	lst, ok := status["loaded"]
	if !ok {
		t.Fatal("ToolStatus missing 'loaded' entry")
	}
	if lst["enabled"] != true || lst["loaded"] != true {
		t.Errorf("loaded status = %v, want enabled=true loaded=true", lst)
	}

	dst, ok := status["disabled"]
	if !ok {
		t.Fatal("ToolStatus missing 'disabled' entry")
	}
	if dst["enabled"] != false {
		t.Errorf("disabled enabled = %v, want false", dst["enabled"])
	}
	if dst["loaded"] != false {
		t.Errorf("disabled loaded = %v, want false", dst["loaded"])
	}
	// lastUsed should be present and non-zero for the loaded tool.
	if lu, ok := lst["lastUsed"].(time.Time); !ok || lu.IsZero() {
		t.Error("loaded tool lastUsed should be a non-zero time.Time")
	}
}

func TestRouter_ToolStatus_Empty(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	if status := r.ToolStatus(); len(status) != 0 {
		t.Errorf("empty router ToolStatus = %v, want empty", status)
	}
}

func TestRouter_ListTools_DisabledEmpty(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tool := NewLazyTool("hidden", "d", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(tool, false)

	if names := r.ListTools(); len(names) != 0 {
		t.Errorf("disabled-only router ListTools = %v, want empty", names)
	}
}

// ---------------------------------------------------------------------------
// 4. Concurrent access safety
// ---------------------------------------------------------------------------

func TestLazyTool_ConcurrentLoadOnce(t *testing.T) {
	t.Parallel()

	var callCount int32
	loader := func(_ string) (Tool, error) {
		atomic.AddInt32(&callCount, 1)
		return &mockTool{name: "concurrent"}, nil
	}
	tool := NewLazyTool("concurrent", "d", loader)

	// Hammer Execute from many goroutines; the loader must run exactly once
	// and no goroutine should observe a partially-initialized state.
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := tool.Execute(testCtx(), nil); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Execute error: %v", err)
	}
	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Errorf("loader called %d times concurrently, want exactly 1", got)
	}
	if !tool.IsLoaded() {
		t.Error("tool should be loaded after concurrent access completes")
	}
}

func TestRouter_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tools := make([]*LazyTool, 0, 20)
	for i := 0; i < 20; i++ {
		tl := NewLazyTool(fmt.Sprintf("t%d", i), "d", func(_ string) (Tool, error) {
			return &mockTool{name: fmt.Sprintf("t%d", i)}, nil
		})
		tools = append(tools, tl)
		// Alternate enable state on registration.
		r.Register(tl, i%2 == 0)
	}

	var wg sync.WaitGroup

	// Reader goroutines.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.ListTools()
			_ = r.ToolStatus()
		}()
	}
	// Mutator goroutines flipping enable state.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.Enable(fmt.Sprintf("t%d", idx%20))
			r.Disable(fmt.Sprintf("t%d", (idx+1)%20))
		}(i)
	}
	// GetTool callers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = r.GetTool(fmt.Sprintf("t%d", idx%20))
		}(i)
	}

	wg.Wait()

	// Final state sanity: every registered tool has a status entry.
	status := r.ToolStatus()
	if len(status) != 20 {
		t.Errorf("ToolStatus has %d entries, want 20", len(status))
	}
}

// ---------------------------------------------------------------------------
// 5. ResponseToOpenAI
// ---------------------------------------------------------------------------

func TestResponseToOpenAI_SuccessMap(t *testing.T) {
	t.Parallel()

	result := map[string]interface{}{"answer": "42"}
	out := ResponseToOpenAI(result, nil)
	if content, ok := out["content"].(map[string]interface{}); !ok {
		t.Fatalf("expected content map, got %T", out["content"])
	} else if content["answer"] != "42" {
		t.Errorf("content answer = %v, want 42", content["answer"])
	}
	if _, hasErr := out["error"]; hasErr {
		t.Error("success response should not contain an error key")
	}
}

func TestResponseToOpenAI_SuccessNonMap(t *testing.T) {
	t.Parallel()

	// Non-map results are wrapped under "content" verbatim.
	out := ResponseToOpenAI("plain-string", nil)
	if out["content"] != "plain-string" {
		t.Errorf("content = %v, want 'plain-string'", out["content"])
	}
	if _, hasErr := out["error"]; hasErr {
		t.Error("success response should not contain an error key")
	}
}

func TestResponseToOpenAI_Error(t *testing.T) {
	t.Parallel()

	err := errors.New("something broke")
	out := ResponseToOpenAI(nil, err)
	// On error the response must carry the error message and no content.
	errMap, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error map, got %T", out["error"])
	}
	if errMap["message"] != "something broke" {
		t.Errorf("error message = %v, want 'something broke'", errMap["message"])
	}
	if _, hasContent := out["content"]; hasContent {
		t.Error("error response should not contain a content key")
	}
}

func TestResponseToOpenAI_ErrorTakesPrecedence(t *testing.T) {
	t.Parallel()

	// When both result and error are provided, the error branch wins.
	out := ResponseToOpenAI("ignored", errors.New("boom"))
	if _, ok := out["error"]; !ok {
		t.Error("expected error key when error is non-nil")
	}
	if _, ok := out["content"]; ok {
		t.Error("did not expect content key when error is non-nil")
	}
}

func TestJSONResponse(t *testing.T) {
	t.Parallel()

	data := map[string]interface{}{"ok": true, "n": 2}
	b, err := JSONResponse(data)
	if err != nil {
		t.Fatalf("JSONResponse error: %v", err)
	}
	s := string(b)
	if !contains(s, "\"ok\":true") || !contains(s, "\"n\":2") {
		t.Errorf("JSONResponse payload = %q, expected to contain ok and n keys", s)
	}
}

// ---------------------------------------------------------------------------
// 6. OpenAICompatibleTool format
// ---------------------------------------------------------------------------

func TestOpenAICompatibleTool_ToOpenAIFormat(t *testing.T) {
	t.Parallel()

	tool := NewLazyTool("oai", "an openai compat tool", func(_ string) (Tool, error) { return &mockTool{}, nil })
	o := &OpenAICompatibleTool{lazyTool: tool}

	out := o.ToOpenAIFormat()
	if out["type"] != "function" {
		t.Errorf("type = %v, want 'function'", out["type"])
	}
	fn, ok := out["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("function field is not a map: %T", out["function"])
	}
	if fn["name"] != "oai" {
		t.Errorf("function name = %v, want 'oai'", fn["name"])
	}
	if fn["description"] != "an openai compat tool" {
		t.Errorf("function description = %v, want the provided description", fn["description"])
	}
	params, ok := fn["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters field is not a map: %T", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v, want 'object'", params["type"])
	}
	if _, ok := params["properties"]; !ok {
		t.Error("parameters should contain a properties key")
	}
	if _, ok := params["required"]; !ok {
		t.Error("parameters should contain a required key")
	}
}

func TestOpenAICompatibleTool_getParameters(t *testing.T) {
	t.Parallel()

	tool := NewLazyTool("p", "desc", func(_ string) (Tool, error) { return &mockTool{}, nil })
	o := &OpenAICompatibleTool{lazyTool: tool}

	params := o.getParameters()
	if params["type"] != "object" {
		t.Errorf("type = %v, want 'object'", params["type"])
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties is not a map: %T", params["properties"])
	}
	if len(props) != 0 {
		t.Errorf("expected empty properties, got %d", len(props))
	}
	req, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("required is not []string: %T", params["required"])
	}
	if len(req) != 0 {
		t.Errorf("expected empty required, got %v", req)
	}
}

// ---------------------------------------------------------------------------
// Disabled tools cannot execute or be listed
// ---------------------------------------------------------------------------

func TestDisabledTool_NotListed(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tool := NewLazyTool("disabled-tool", "d", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(tool, false)

	names := r.ListTools()
	if len(names) != 0 {
		t.Errorf("disabled tool appeared in ListTools: %v", names)
	}
}

func TestDisabledTool_NotRetrievable(t *testing.T) {
	t.Parallel()

	r := NewRouter()
	tool := NewLazyTool("nope", "d", func(_ string) (Tool, error) { return &mockTool{}, nil })
	r.Register(tool, false)

	if _, ok := r.GetTool("nope"); ok {
		t.Error("GetTool should return false for a disabled tool")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sortedNames(t *testing.T, names []string) []string {
	t.Helper()
	cp := append([]string(nil), names...)
	sort.Strings(cp)
	return cp
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
