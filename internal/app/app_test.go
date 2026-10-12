package app

import (
	"ape-player/internal/config"
	"ape-player/internal/emby"
	"context"
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, e := New()
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestSourcesAndLoginViews(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("添加 Emby 来源") {
		t.Fatal("source onboarding missing")
	}
	if e := tt.Click("偏好设置"); e != nil {
		t.Fatal(e)
	}
	if !tt.HasText("播放偏好") {
		t.Fatal("settings navigation failed")
	}
	a.cfg.Sources = []config.Source{{ID: "test", Name: "家庭影院"}}
	a.cfg.Active = "test"
	a.page = "login"
	tt.Frame()
	if !tt.HasText("欢迎回来") {
		t.Fatal("login missing")
	}
	if e := tt.Click("登录用户名"); e != nil {
		t.Fatal(e)
	}
	tt.Type("alice")
	if a.username != "alice" {
		t.Fatalf("input %q", a.username)
	}
	if dir := os.Getenv("APE_TEST_ARTIFACTS"); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		f, _ := os.Create(filepath.Join(dir, "login.png"))
		if f != nil {
			_ = png.Encode(f, tt.Image())
			_ = f.Close()
		}
	}
}
func TestSourceSwitchCancelsRequests(t *testing.T) {
	a := testApp(t)
	ctx := a.ctx
	a.activate("")
	if ctx.Err() != context.Canceled {
		t.Fatal("old source requests not cancelled")
	}
	if a.page != "sources" {
		t.Fatal(a.page)
	}
}
func TestLibraryAndPlayerLayouts(t *testing.T) {
	a := testApp(t)
	a.page = "library"
	a.items = []emby.Item{{ID: "movie", Name: "测试电影", ProductionYear: 2026}}
	a.total = 1
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("测试电影") {
		t.Fatal("grid missing")
	}
	a.page = "player"
	a.playback.Item = emby.Item{Name: "播放测试"}
	tt.Frame()
	if !tt.HasText("返回") {
		t.Fatal("player controls missing")
	}
}

func TestSourceAccountPasswordFields(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	if tt.HasText("来源账号") {
		t.Fatal("source form should start hidden")
	}
	if err := tt.Click("添加 Emby 来源"); err != nil {
		t.Fatal(err)
	}
	if e := tt.Click("来源账号"); e != nil {
		t.Fatal(e)
	}
	tt.Type("alice")
	if e := tt.Click("来源密码"); e != nil {
		t.Fatal(e)
	}
	tt.Type("my password")
	if a.draft.Username != "alice" || a.sourcePassword != "my password" {
		t.Fatal("source credentials not bound to inputs")
	}
	if !tt.HasText("保存并登录") {
		t.Fatal("direct login action missing")
	}
	tt.Key(0, ui.KeyEscape)
	if a.sourceEditorOpen || a.sourcePassword != "" {
		t.Fatal("dismissed modal retained password")
	}
	if e := tt.Click("偏好设置"); e != nil {
		t.Fatal(e)
	}
	if e := tt.Click("来源管理"); e != nil {
		t.Fatal(e)
	}
	if a.sourcePassword != "" {
		t.Fatal("draft password retained after returning to sources")
	}
}

func TestSavingSourceSubmitsCredentials(t *testing.T) {
	a := testApp(t)
	got := make(chan map[string]string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users/AuthenticateByName" {
			t.Errorf("unexpected login path %s", r.URL.Path)
		}
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		got <- body
		<-r.Context().Done()
	}))
	defer srv.Close()
	a.draft = config.Source{Name: "Home", URL: srv.URL, Username: " alice "}
	a.sourcePassword = "secret"
	a.saveSource()
	defer a.cancel()
	select {
	case body := <-got:
		if body["Username"] != "alice" || body["Pw"] != "secret" {
			t.Fatalf("incorrect login request: username=%q", body["Username"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("saving source did not start password authentication")
	}
}

func TestSourceEditorIsolationAndValidation(t *testing.T) {
	a := testApp(t)
	a.cfg.Sources = []config.Source{{ID: "first", Name: "家庭影院", URL: "https://example.com", Username: "alice", Token: "saved-token", UserID: "u", Remember: true}}
	tt := ui.NewTester(a.view, 940, 640)
	if err := tt.Click("编辑来源：家庭影院"); err != nil {
		t.Fatal(err)
	}
	if !a.sourceEditorOpen || a.editing != "first" || a.draft.Name != "家庭影院" || a.sourcePassword != "" {
		t.Fatal("edit modal did not load the selected source")
	}
	if err := tt.Click("来源名称"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("changed")
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.sourceEditorOpen || a.cfg.Sources[0].Name != "家庭影院" || a.draft.Name != "" {
		t.Fatal("cancel mutated the stored source")
	}
	if err := tt.Click("添加 Emby 来源"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存并连接"); err != nil {
		t.Fatal(err)
	}
	if !a.sourceEditorOpen || a.sourceError == "" || len(a.cfg.Sources) != 1 || !tt.HasText(a.sourceError) {
		t.Fatal("invalid address should leave an error inside the modal")
	}
	tt.ClickAt(10, 400)
	if a.sourceEditorOpen || a.sourceError != "" || tt.HasText("来源地址") {
		t.Fatal("backdrop click did not dismiss the form")
	}
}

func TestSavedSourcesRequireSelectionAndAllowReturn(t *testing.T) {
	a := testApp(t)
	requests := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		<-r.Context().Done()
	}))
	defer srv.Close()
	a.cfg.Sources = []config.Source{
		{ID: "first", Name: "家庭影院", URL: srv.URL, Username: "alice", Token: "saved-token", UserID: "u", Remember: true},
		{ID: "second", Name: "备用服务", URL: srv.URL + "/second", Username: "bob"},
	}
	a.cfg.Active = "first"
	a.save()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer b.cancel()
	tt := ui.NewTester(b.view, 1280, 820)
	if b.page != "sources" || b.client != nil || !tt.HasText("家庭影院") || !tt.HasText("备用服务") || tt.HasText("继续观看") {
		t.Fatal("startup should show source selection despite a saved active source")
	}
	if err = tt.Click("进入服务：家庭影院"); err != nil {
		t.Fatal(err)
	}
	select {
	case path := <-requests:
		if path != "/Users/u" {
			t.Fatal(path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selected service was not activated")
	}
	if b.page != "home" || b.client == nil || b.client.Token != "saved-token" {
		t.Fatal("saved login not used for chosen service")
	}
	oldctx := b.ctx
	if err = tt.Click("返回来源管理"); err != nil {
		t.Fatal(err)
	}
	if b.page != "sources" || b.client != nil || oldctx.Err() != context.Canceled || len(b.sections) != 0 || len(b.libraries) != 0 {
		t.Fatal("return did not cancel and clear the active service")
	}
	if b.cfg.Sources[0].Token != "saved-token" {
		t.Fatal("return unexpectedly logged out")
	}
	if err = tt.Click("服务：备用服务"); err != nil {
		t.Fatal(err)
	}
	if b.page != "login" || b.username != "bob" || b.cfg.Active != "second" || b.client.Token != "" {
		t.Fatal("source selection leaked credentials or skipped login")
	}
}
