package main

import (
	"os"
	"testing"
)

type Item struct {
	Pos   int    `json:"pos"`
	Title string `json:"title"`
	Year  int    `json:"year"`
}

func Test_prepareTweet(t *testing.T) {
	tests := []struct {
		act  Item
		want string
	}{
		{act: Item{
			Pos:   2,
			Title: "Rozporządzenie @MF_gov_PLN z dnia 31 grudnia 2019 r. w sprawie postępowania kwalifikacyjnego w stosunku do kandydatów ubiegających się o przyjęcie do służby w Służbie Celno-Skarbowej",
			Year:  2020},
			want: "Dz.U. 2020 poz. 2 #DziennikUstaw\nRozporządzenie @MF_gov_PLN z dnia 31 grudnia 2019 r. w sprawie postępowania kwalifikacyjnego w stosunku do kandydatów ubiegających się o przyjęcie do służby w Służbie Celno-Skarbowej\nhttps://dziennikustaw.gov.pl/D2020000000201.pdf",
		},
		{act: Item{
			Pos:   2,
			Title: "Oświadczenie Rządowe z dnia 18 grudnia 2019 r. w sprawie mocy obowiązującej w relacjach między Rzecząpospolitą Polską a Republiką Islandii Konwencji wielostronnej implementującej środki traktatowego prawa podatkowego mające na celu zapobieganie erozji podstawy opodatkowania i przenoszeniu zysku, sporządzonej w Paryżu dnia 24 listopada 2016 r., oraz jej zastosowania w realizacji postanowień Umowy między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii w sprawie unikania podwójnego opodatkowania i zapobiegania uchylaniu się od opodatkowania w zakresie podatków od dochodu i majątku, sporządzonej w Reykjaviku dnia 19 czerwca 1998 r., oraz w realizacji postanowień Protokołu między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii o zmianie Umowy między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii w sprawie unikania podwójnego opodatkowania i zapobiegania uchylaniu się od opodatkowania w zakresie podatków od dochodu i majątku, sporządzonej w Reykjaviku dnia 19 czerwca 1998 r., podpisanego w Reykjaviku dnia 16 maja 2012 r.",
			Year:  2020},
			want: "Dz.U. 2020 poz. 2 #DziennikUstaw\nOświadczenie Rządowe z dnia 18 grudnia 2019 r. w sprawie mocy obowiązującej w relacjach między Rzecząpospolitą Polską a Republiką Islandii Konwencji wielostronnej implementującej środki traktatowego …\nhttps://dziennikustaw.gov.pl/D2020000000201.pdf",
		},
		{act: Item{
			Pos:   2146,
			Title: "Rozporządzenie Ministra Edukacji i Nauki z dnia 1 grudnia 2020 r. zmieniające rozporządzenie w sprawie pomocy de minimis w ramach programu „Wsparcie dla czasopism naukowych”", Year: 2020},
			want: "Dz.U. 2020 poz. 2146 #DziennikUstaw\nRozporządzenie @Nauka_gov_PL z dnia 1 grudnia 2020 r. zmieniające rozporządzenie w sprawie pomocy de minimis w ramach programu „Wsparcie dla czasopism naukowych”\nhttps://dziennikustaw.gov.pl/D2020000214601.pdf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.act.Title, func(t *testing.T) {
			if got := prepareTweet(tt.act.Year, tt.act.Pos, tt.act.Title); got != tt.want {
				t.Errorf("prepareTweet() = \n%v, want \n%v", got, tt.want)
			}
		})
	}
}

func Test_trimTweet(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{
			title: "Rozporządzenie @MF_gov_PLN z dnia 31 grudnia 2019 r. w sprawie postępowania kwalifikacyjnego w stosunku do kandydatów ubiegających się o przyjęcie do służby w Służbie Celno-Skarbowej",
			want:  "Rozporządzenie @MF_gov_PLN z dnia 31 grudnia 2019 r. w sprawie postępowania kwalifikacyjnego w stosunku do kandydatów ubiegających się o przyjęcie do służby w Służbie Celno-Skarbowej",
		},
		{
			title: "Oświadczenie Rządowe z dnia 18 grudnia 2019 r. w sprawie mocy obowiązującej w relacjach między Rzecząpospolitą Polską a Republiką Islandii Konwencji wielostronnej implementującej środki traktatowego prawa podatkowego mające na celu zapobieganie erozji podstawy opodatkowania i przenoszeniu zysku, sporządzonej w Paryżu dnia 24 listopada 2016 r., oraz jej zastosowania w realizacji postanowień Umowy między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii w sprawie unikania podwójnego opodatkowania i zapobiegania uchylaniu się od opodatkowania w zakresie podatków od dochodu i majątku, sporządzonej w Reykjaviku dnia 19 czerwca 1998 r., oraz w realizacji postanowień Protokołu między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii o zmianie Umowy między Rządem Rzeczypospolitej Polskiej a Rządem Republiki Islandii w sprawie unikania podwójnego opodatkowania i zapobiegania uchylaniu się od opodatkowania w zakresie podatków od dochodu i majątku, sporządzonej w Reykjaviku dnia 19 czerwca 1998 r., podpisanego w Reykjaviku dnia 16 maja 2012 r.",
			want:  "Oświadczenie Rządowe z dnia 18 grudnia 2019 r. w sprawie mocy obowiązującej w relacjach między Rzecząpospolitą Polską a Republiką Islandii Konwencji wielostronnej implementującej środki traktatowego …",
		},
		{
			title: "Obwieszczenie Ministra Zdrowia z dnia 21 maja 2020 r. w sprawie ogłoszenia jednolitego tekstu rozporządzenia Ministra Zdrowia w sprawie grzybów dopuszczonych do obrotu lub produkcji przetworów grzybowych, środków spożywczych zawierających grzyby oraz uprawnień klasyfikatora grzybów i grzyboznawcy",
			want:  "📢Obwieszczenie @MZ_gov_PL z dnia 21 maja 2020 r. w sprawie ogłoszenia jednolitego tekstu rozporządzenia @MZ_gov_PL w sprawie grzybów dopuszczonych do obrotu lub produkcji przetworów grzybowych, środk…",
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := trimTitle(tt.title); got != tt.want {
				t.Errorf("prepareTweet() =\n%v, want\n%v", got, tt.want)
			}
		})
	}
}

func Test_getIdFromTweet(t *testing.T) {
	tests := []struct {
		in   string
		id int
		y int
	}{
		{in: "Dz.U. 2020 poz. 1", y: 2020,  id: 1},
		{in: "Dz.U. 2020 poz. 999", y: 2020, id: 999},
		{in: "Dz.U. 2020 poz. 2\nRozporządzenie @MF_gov_PLN z dnia 31 grudnia 2019 r. w sprawie postępowania kwalifikacyjnego w stosunku do kandydatów ubiegających się o przyjęcie do służby w Służbie Celno-Skarbowej\nhttp://api.sejm.gov.pl/eli/acts/DU/2020/2/text.pdf", y: 2020, id: 2},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			y, id := getIdFromTweet(tt.in)
			if y != tt.y {
				t.Errorf("getIdFromTweet() = %v, want %v", y, tt.y)
			}
			if id != tt.id {
				t.Errorf("getIdFromTweet() = %v, want %v", id, tt.id)
			}
		})
	}
}

func Test_getTitleFromPage(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "sample.html", want: "Rozporządzenie Ministra Edukacji i Nauki z dnia 1 grudnia 2020 r. zmieniające rozporządzenie w sprawie pomocy de minimis w ramach programu „Wsparcie dla czasopism naukowych”"},
		{name: "404.html", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, _ := os.Open("testdata/" + tt.name)
			if got := getTitleFromPage(file); got != tt.want {
				t.Errorf("getTitleFromPage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_convertPDFToPng(t *testing.T) {
	file, _ := os.Open("testdata/D2020000000101.pdf")
	out, err := convertPDFToJpgs(file)
	if err != nil {
		t.Errorf("Got %v", err)
	}
	if len(out) != 2 {
		t.Errorf("Got %v", out)
	}
}