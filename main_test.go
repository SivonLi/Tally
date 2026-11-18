package main

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestView(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 480, 360)
	tt.SetLocale("en")
	if err := tt.Click("+"); err != nil {
		t.Fatal(err)
	}
	if a.count != 1 || !tt.HasText("1") {
		t.Errorf("count %d after a click, texts %q", a.count, tt.Texts())
	}
	if err := tt.Click("−"); err != nil {
		t.Fatal(err)
	}
	if a.count != 0 || !tt.HasText("0") {
		t.Errorf("count %d after a second click, texts %q", a.count, tt.Texts())
	}
	if err := tt.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Ada")
	if !tt.HasText("Hello, Ada!") {
		t.Errorf("texts %q after typing a name", tt.Texts())
	}
}

func TestViewChinese(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	a := &app{}
	tt := ui.NewTester(a.view, 480, 360)
	tt.SetLocale("zh-Hans")
	if !tt.HasText("你好！") {
		t.Errorf("texts %q in Chinese", tt.Texts())
	}
	if err := tt.Click("姓名"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Ada")
	if !tt.HasText("你好，Ada！") {
		t.Errorf("texts %q after typing a name", tt.Texts())
	}
}

func TestLanguageSwitch(t *testing.T) {
	a := &app{language: "en"}
	// 目录访问器通过 i18n.SetLocaleFunc 读取界面语言，测试把它接到 app 状态。
	i18n.SetLocaleFunc(func() string { return a.language })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	tt := ui.NewTester(a.view, 480, 360)
	if err := tt.Click(i18n.Autonym("zh-Hans")); err != nil {
		t.Fatal(err)
	}
	if a.language != "zh-Hans" {
		t.Errorf("language %q after switching to Chinese", a.language)
	}
	if !tt.HasText("你好！") {
		t.Errorf("texts %q after switching to Chinese", tt.Texts())
	}
	if err := tt.Click(i18n.Autonym("en")); err != nil {
		t.Fatal(err)
	}
	if a.language != "en" {
		t.Errorf("language %q after switching to English", a.language)
	}
	if !tt.HasText("Hello!") {
		t.Errorf("texts %q after switching to English", tt.Texts())
	}
}
