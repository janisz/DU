package main

import (
	"fmt"
	"golang.org/x/net/html"
	"io"
	"os"
	"strconv"

	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/dghubble/go-twitter/twitter"
)

const url = "https://dziennikustaw.gov.pl"

func main() {
	log.Println("Dziennik Ustaw")

	// oauth2 configures a client that uses app credentials to keep a fresh token
	config := &clientcredentials.Config{
		ClientID:     os.Getenv("ClientID"),
		ClientSecret: os.Getenv("ClientSecret"),
		TokenURL:     "https://api.twitter.com/oauth2/token",
	}
	// http.Client will automatically authorize Requests
	httpClient := config.Client(oauth2.NoContext)

	// Twitter client
	client := twitter.NewClient(httpClient)

	t, _, err := client.Search.Tweets(&twitter.SearchTweetParams{
		Query:      "(from:Dziennik_Ustaw) filter:links -filter:replies",
		Lang:       "pl",
		ResultType: "recent",
		Count:      1,
		TweetMode:  "extended",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%#v", t)

	lastTweetedId := 2140
	for _, tweet := range t.Statuses {
		i := getIdFromTweet(tweet.FullText)
		if i > lastTweetedId {
			lastTweetedId = i
		}
	}

	year := time.Now().Year()
	for {
		lastTweetedId++
		r, err := http.DefaultClient.Get(fmt.Sprintf("%s/DU/%d/%d", url, year, lastTweetedId))
		if err != nil {
			log.Fatal(err)
		}
		if r.StatusCode != http.StatusOK {
			log.Fatal(r.Status)
		}
		title := getTitleFromPage(r.Body)
		if title == "" {
			log.Println("No data for ", lastTweetedId)
			return
		}
		tweet := prepareTweet(year, lastTweetedId, title)
		fmt.Println(tweet)
	}
}

const MaxTitleLength = 230

func getTitleFromPage(body io.ReadCloser) string {
	z := html.NewTokenizer(body)

	title := false
	for {
		tt := z.Next()
		switch {
		case tt == html.TextToken:
			if title {
				return z.Token().String()
			}
		case tt == html.ErrorToken:
			// End of the document, we're done
			return ""
		case tt == html.StartTagToken:
			t := z.Token()
			if t.Data == "h2" {
				title = true
				continue
			}
		}
	}

}

func prepareTweet(year, id int, title string) string {
	return strings.Join([]string{
		fmt.Sprintf("Dz.U. %d poz. %d", year, id), // 21 chars (Dz.U. YYYY poz. XXXX\n)
		trimTitle(title), // < 280-21-23 = 235 (1 for new line)
		fmt.Sprintf("%s/D%d%07d01.pdf", url, year, id), // 23 chars (The current length of a URL in a Tweet is 23 characters, even if the length of the URL would normally be shorter.)
	}, "\n")
}

var handles = map[string]string{
	"Ministra Zdrowia":                            "@MZ_gov_PL",
	"Ministra Infrastruktury":                     "@MI_gov_PL",
	"Ministra Sportu":                             "@Sport_gov_PL",
	"Prezesa Rady Ministrów":                      "@PremierRP",
	"Ministra Obrony Narodowej":                   "@MON_gov_PL",
	"Ministra Finansów":                           "@MF_gov_PL",
	"Ministra Sprawiedliwości":                    "@MS_gov_PL",
	"Ministra Spraw Zagranicznych":                "@MSZ_RP",
	"Ministra Spraw Wewnętrznych i Administracji": "@MSWiA_gov_PL",
	"Ministra Edukacji Narodowej":                 "@MEN_gov_PL",
	"Ministra Nauki i Szkolnictwa Wyższego":       "@NAUKA_gov_PL",
	"Ministra Kultury i Dziedzictwa Narodowego":   "@MKiDN_gov_PL",
	"Ministra Rolnictwa i Rozwoju Wsi":            "@MRiRW_gov_PL",
}

func trimTitle(title string) string {
	for name, handle := range handles {
		title = strings.ReplaceAll(title, name, handle)
	}

	runes := []rune(title)
	if len(runes) < MaxTitleLength {
		return title
	}
	return string(runes[:MaxTitleLength-1]) + "…"
}

func getIdFromTweet(s string) int {
	a := strings.Split(strings.Split(s, "\n")[0], " ")
	if len(a) < 4 {
		log.Printf("Parsing %s not enought tokens", s)
		return 0
	}
	i := strings.Trim(a[3], "\n")
	id, err := strconv.Atoi(i)
	if err != nil {
		log.Printf("Parsing %s got %s", s, err)
		return 0
	}
	return id

}
