package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"

	"github.com/gen2brain/go-fitz"

	"golang.org/x/net/html"
)

const url = "https://dziennikustaw.gov.pl"

func main() {
	log.Println("Dziennik Ustaw")

	config := oauth1.NewConfig(os.Getenv("consumerKey"), os.Getenv("consumerSecret"))
	token := oauth1.NewToken(os.Getenv("accessToken"), os.Getenv("accessSecret"))
	// http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

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

	lastTweetedId := 0
	lastTweetedYear := 0
	for _, tweet := range t.Statuses {
		log.Println(tweet.ID, tweet.FullText)
		y, i := getIdFromTweet(tweet.FullText)
		if i > lastTweetedId {
			lastTweetedId = i
		}
		if y > lastTweetedYear {
			lastTweetedYear = y
		}
	}
	year := time.Now().Year()
	if year != lastTweetedYear {
		lastTweetedId = 0
	}

	log.Println("Pos:", lastTweetedId)
	log.Println("Year:", lastTweetedYear)
	log.Println("Year", year)

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
		tweetText := prepareTweet(year, lastTweetedId, title)
		log.Println(tweetText)

		url := pdfUrl(year, lastTweetedId)
		r, err = http.DefaultClient.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		if r.StatusCode != http.StatusOK {
			log.Fatal(r)
		}
		pages, err := convertPDFToJpgs(r.Body)
		r.Body.Close()

		log.Printf("Pages: %v", len(pages))
		if err != nil {
			log.Fatal(err)
		}
		mediaIds := make([]int64, 0, len(pages))
		for _, p := range pages {
			resp, _, err := client.Media.Upload(p, "image/jpeg")
			if err != nil {
				log.Fatal(resp)
			}
			mediaIds = append(mediaIds, resp.MediaID)
		}

		lat := 52.22548
		long := 21.02839
		truthy := true
		t, _, err := client.Statuses.Update(tweetText, &twitter.StatusUpdateParams{
			Status:             "",
			InReplyToStatusID:  0,
			PossiblySensitive:  nil,
			Lat:                &lat,
			Long:               &long,
			PlaceID:            "",
			DisplayCoordinates: &truthy,
			TrimUser:           nil,
			MediaIds:           mediaIds,
			TweetMode:          "",
		})
		log.Println(t.ID, t.Text)
		if err != nil {
			log.Fatal(err)
		}
	}
}

const MaxTitleLength = 200

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
		fmt.Sprintf("Dz.U. %d poz. %d #DziennikUstaw", year, id), // 37 chars (Dz.U. YYYY poz. XXXX #DziennikUstaw\n)
		trimTitle(title), // < 280-37-23 ~ 200 (1 for new line)
		pdfUrl(year, id), // 23 chars (The current length of a URL in a Tweet is 23 characters, even if the length of the URL would normally be shorter.)
	}, "\n")
}

func pdfUrl(year int, id int) string {
	return fmt.Sprintf("%s/D%d%07d01.pdf", url, year, id)
}

var handles = map[string]string{
	"Ministra Zdrowia":                            "@MZ_gov_PL",
	"Ministra Infrastruktury":                     "@MI_gov_PL",
	"Ministra Sportu":                             "@Sport_gov_PL",
	"Prezesa Rady Ministrów":                      "@PremierRP",
	"Prezydenta Rzeczypospolitej Polskiej":        "@PrezydentPL",
	"Ministra Obrony Narodowej":                   "@MON_gov_PL",
	"Ministra Finansów":                           "@MF_gov_PL",
	"Ministra Sprawiedliwości":                    "@MS_gov_PL",
	"Ministra Spraw Zagranicznych":                "@MSZ_RP",
	"Ministra Spraw Wewnętrznych i Administracji": "@MSWiA_gov_PL",
	"Ministra Edukacji Narodowej":                 "@MEN_gov_PL",
	"Ministra Nauki i Szkolnictwa Wyższego":       "@Nauka_gov_PL",
	"Ministra Kultury i Dziedzictwa Narodowego":   "@MKiDN_gov_PL",
	"Ministra Rolnictwa i Rozwoju Wsi":            "@MRiRW_gov_PL",
	"Trybunału Konstytucyjnego":                   "@TK_gov_PL",
	"Sejmu Rzeczypospolitej Polskiej":             "@KancelariaSejmu",
	"Ministra Edukacji i Nauki":                   "@Nauka_gov_PL",
	"Ministra Klimatu":                            "@MKiS_gov_PL",
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

func getIdFromTweet(s string) (year, id int) {
	a := strings.Split(strings.Split(s, "\n")[0], " ")
	if len(a) < 4 {
		log.Printf("Parsing %s not enought tokens", s)
		return 0, 0
	}
	i := strings.Trim(a[3], "\n")
	id, err := strconv.Atoi(i)
	if err != nil {
		log.Printf("Parsing %s got %s", s, err)
		return 0, 0
	}
	year, err = strconv.Atoi(a[1])
	if err != nil {
		log.Printf("Parsing %s got %s", s, err)
		return 0, 0
	}
	return year, id
}

func convertPDFToJpgs(pdf io.Reader) ([][]byte, error) {
	doc, err := fitz.NewFromReader(pdf)
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	log.Printf("Pages: %d\n%v", doc.NumPage(), doc.Metadata())
	if doc.NumPage() > 4 {
		return nil, nil
	}

	result := make([][]byte, 0, doc.NumPage())

	// Extract pages as images
	for n := 0; n < doc.NumPage(); n++ {
		img, err := doc.Image(n)
		if err != nil {
			return nil, err
		}

		var b bytes.Buffer

		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		if err != nil {
			return nil, err
		}

		result = append(result, b.Bytes())
	}
	return result, nil
}
