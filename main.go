package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/locales"
)

// app is the state the window shows. Its view builds the interface from
// it, on the main thread, whenever the window needs a frame: after input,
// after Window.Update, and while something animates.
type app struct {
	name     string
	count    int
	language string
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(16).Padding(24).Children(func() {
		ui.Text(c, locales.AppTitle()).FontSize(26).Bold()
		ui.RadioGroup(c, func() {
			for _, tag := range []string{"en", "zh-Hans"} {
				if ui.Radio(c, &a.language, tag, i18n.Autonym(tag)).Changed() {
					mygo.App.SetLocale(a.language)
				}
			}
		}).Label(locales.LanguageLabel())
		ui.TextInput(c, &a.name).Placeholder(locales.NamePlaceholder()).Label(locales.NameLabel()).Width(240)
		greeting := locales.Greeting()
		if a.name != "" {
			greeting = locales.GreetingName(a.name)
		}
		ui.Text(c, greeting).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "−").Label(locales.Decrement()).Width(44).Clicked() {
				a.count--
			}
			ui.Textf(c, "%d", a.count).FontSize(20).Width(48).TextAlign(ui.Center)
			if ui.PrimaryButton(c, "+").Label(locales.Increment()).Width(44).Clicked() {
				a.count++
			}
		})
	})
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		a.language = mygo.App.Locale()
		mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppTitle(),
			Width:     480,
			Height:    360,
			MinWidth:  320,
			MinHeight: 280,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
