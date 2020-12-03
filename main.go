package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"

	"github.com/gen2brain/go-fitz"

	"golang.org/x/net/html"

	log "github.com/sirupsen/logrus"
)

const url = "https://dziennikustaw.gov.pl"

func main() {
	lat := 52.22548
	long := 21.02839
	truthy := true

	log.SetLevel(log.DebugLevel)

	log.Info("Dziennik Ustaw")

	config := oauth1.NewConfig(os.Getenv("consumerKey"), os.Getenv("consumerSecret"))
	token := oauth1.NewToken(os.Getenv("accessToken"), os.Getenv("accessSecret"))
	// http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := twitter.NewClient(httpClient)

	t, _, err := client.Search.Tweets(&twitter.SearchTweetParams{
		Query:      "-from:Dziennik_Ustaw #DziennikUstaw OR Dziennik Ustaw OR Dz.U.",
		Lang:       "pl",
		ResultType: "recent",
		Since:		time.Now().Add(-24*time.Hour).Format("2006-01-02"),
		Count:      10,
		TweetMode:  "extended",
	})
	if err != nil {
		log.WithError(err).Fatal("Could not find tweets")
	}
	for _, tweet := range t.Statuses {
		log.WithField("ID", tweet.ID).WithField("Date", tweet.CreatedAt).
			WithField("❤ ", tweet.FavoriteCount).WithField("⮔ ", tweet.RetweetCount).
			WithField("Text", tweet.FullText).Info("Like tweet")
		if _, ok := os.LookupEnv("DRY"); ok {
			log.Warn("DRY RUN")
			continue
		}
		liked, _, err := client.Favorites.Create(&twitter.FavoriteCreateParams{ID: tweet.ID})
		if err != nil {
			log.WithField("ID", tweet.ID).WithError(err).Warn("Could not like tweet 💔")
			continue
		}
		log.WithField("ID", liked.ID).WithField("❤ ", liked.FavoriteCount).WithField("⮔ ", liked.RetweetCount).Info("Done")
	}

	tweets, _, err := client.Timelines.HomeTimeline(&twitter.HomeTimelineParams{
		Count:          1,
		ExcludeReplies: &truthy,
		TweetMode:      "extended",
	})
	if err != nil {
		log.WithError(err).Fatal("Could not get tweets from timeline")
	}

	lastTweetedId := 0
	lastTweetedYear := 0
	for _, tweet := range tweets {
		log.WithField("ID", tweet.ID).Debug(tweet.FullText)
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

	log.WithField("Current Year", year).Infof("Last tweeted act Dz.U %d pos %d", lastTweetedYear, lastTweetedId)

	for {
		lastTweetedId++

		tweetText := getTweetText(err, year, lastTweetedId)
		if tweetText == "" {
			log.Info("No data for ", lastTweetedId)
			return
		}
		mediaIds, err := uploadImages(year, lastTweetedId, client)
		if err != nil {
			log.WithError(err).Fatal("Could not upload images")
		}

		if _, ok := os.LookupEnv("DRY"); ok {
			log.WithField("Text", tweetText).Warn("DRY RUN")
			continue
		}
		log.WithField("Text", tweetText).Info("Publishing...")
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
		log.WithField("ID", t.ID).WithField("Text", t.Text).Info("Done")
		if err != nil {
			log.WithError(err).Fatal("Could not publish tweet")
		}
	}
}

func getTweetText(err error, year int, lastTweetedId int) string {
	r, err := http.DefaultClient.Get(fmt.Sprintf("%s/DU/%d/%d", url, year, lastTweetedId))
	if err != nil {
		log.WithError(err).Fatal("Could not get data from Dz.U.")
	}
	if r.StatusCode != http.StatusOK {
		log.WithField("Status", r.Status).Fatal("Unexpected status")
	}
	title := getTitleFromPage(r.Body)
	if title == "" {
		return ""
	}
	return prepareTweet(year, lastTweetedId, title)
}

func uploadImages(year int, lastTweetedId int, client *twitter.Client) ([]int64, error) {
	url := pdfUrl(year, lastTweetedId)
	r, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(r.Status)
	}
	pages, err := convertPDFToJpgs(r.Body)
	r.Body.Close()

	if err != nil {
		return nil, err
	}
	log.Info("Pages to upload: ", len(pages))
	mediaIds := make([]int64, 0, len(pages))
	if _, ok := os.LookupEnv("DRY"); ok {
		return nil, nil
	}
	for _, p := range pages {
		resp, _, err := client.Media.Upload(p, "image/jpeg")
		if err != nil {
			return nil, err
		}
		if resp.ProcessingInfo != nil {
			log.WithField("MediaID", resp.MediaID).Debugf("Still processing: %#v", resp.ProcessingInfo)
			for {
				time.Sleep(100 * time.Millisecond)
				log.WithField("MediaID", resp.MediaID).Debugf("Checking upload status %d", resp.MediaID)
				r, _, err := client.Media.Status(resp.MediaID)
				if err != nil {
					return nil, err
				}
				if r.ProcessingInfo == nil {
					break
				}
				log.WithField("MediaID", resp.MediaID).Debugf("Still processing: %#v", r.ProcessingInfo)
			}
		}
		log.WithField("MediaID", resp.MediaID).Debug("Upload Succesful")
		mediaIds = append(mediaIds, resp.MediaID)
	}
	return mediaIds, nil
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

var emojis = map[string]string{
	"Obwieszczenie": "📢",
	"Umowa":         "🤝",
	"Porozumienie":  "🤝",
}

func trimTitle(title string) string {
	for name, handle := range handles {
		title = strings.ReplaceAll(title, name, handle)
	}
	for word, emoji := range emojis {
		if strings.HasPrefix(title, word) {
			title = emoji + title
		}
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
		log.Warnf("Parsing %s not enough tokens", s)
		return 0, 0
	}
	i := strings.Trim(a[3], "\n")
	id, err := strconv.Atoi(i)
	if err != nil {
		log.Warnf("Parsing %s got %s", s, err)
		return 0, 0
	}
	year, err = strconv.Atoi(a[1])
	if err != nil {
		log.Warnf("Parsing %s got %s", s, err)
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

	log.Debug("Pages: ", doc.NumPage())
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
