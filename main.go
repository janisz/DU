package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"regexp"
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

var (
	lat    = 52.22548
	long   = 21.02839
	truthy = true
)

func main() {

	log.SetLevel(log.DebugLevel)

	log.Info("Dziennik Ustaw")

	config := oauth1.NewConfig(os.Getenv("consumerKey"), os.Getenv("consumerSecret"))
	token := oauth1.NewToken(os.Getenv("accessToken"), os.Getenv("accessSecret"))
	// http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := twitter.NewClient(httpClient)

	tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
		Count:          1,
		ExcludeReplies: &truthy,
		TweetMode:      "extended",
	})
	if err != nil {
		log.WithError(err).Fatal("Could not get tweets from timeline")
	}

	if len(tweets) < 1 {
		log.Fatal("No tweets")
	}

	tweet := tweets[0]
	log.WithFields(logTweet(tweet)).Debug("Latest tweet from timeline")

	lastTweetedYear, lastTweetedId := getIdFromTweet(tweet.FullText)
	if lastTweetedYear*lastTweetedId == 0 {
		log.WithField("Year", lastTweetedYear).WithField("Pos", lastTweetedId).Fatal("There is a problem with obtaining last tweeted act")
	}
	year := time.Now().Year()
	if year != lastTweetedYear {
		lastTweetedId = 0
	}

	likeTweets(client)
	respondToTweets(client)

	log.WithField("Current Year", year).Infof("Last tweeted act Dz.U %d pos %d", lastTweetedYear, lastTweetedId)

	for {
		lastTweetedId++

		tweetText := getTweetText(year, 0, lastTweetedId)
		if tweetText == "" {
			log.Info("No data for ", lastTweetedId)
			return
		}
		mediaIds, err := uploadImages(year, 0, lastTweetedId, client)
		if err != nil {
			log.WithError(err).Fatal("Could not upload images")
		}

		if _, ok := os.LookupEnv("DRY"); ok {
			log.WithField("Text", tweetText).Warn("DRY RUN")
			continue
		}
		log.WithField("Text", tweetText).Info("Publishing...")
		t, _, err := client.Statuses.Update(tweetText, &twitter.StatusUpdateParams{
			Lat:                &lat,
			Long:               &long,
			DisplayCoordinates: &truthy,
			MediaIds:           mediaIds,
		})
		log.WithField("ID", t.ID).WithField("Text", t.Text).Info("Done")
		if err != nil {
			log.WithError(err).Fatal("Could not publish tweet")
		}
	}
}

func likeTweets(client *twitter.Client) {
	likes, _, err := client.Favorites.List(&twitter.FavoriteListParams{
		UserID:  1334198651141361666,
		Count:   1,
	})
	if err != nil {
		log.WithError(err).Error("Could not find tweets")
		return
	}
	if len(likes) < 1 {
		log.Infof("No likes since last time")
		return
	}

	log.WithFields(logTweet(likes[0])).Info("Latest liked tweet")

	keywords := []string{
		"#DziennikUstaw", "Dziennik Ustaw", "Dzienniku Ustaw", "Dziennika Ustaw", "Dziennikiem Ustaw", "Dziennikowi Ustaw",
	}

	for _, keyword := range keywords {
		log.WithField("Keyword", keyword).Debug("Search for tweets")
		t, _, err := client.Search.Tweets(&twitter.SearchTweetParams{
			Query:      "-from:Dziennik_Ustaw " + keyword,
			Lang:       "pl",
			ResultType: "recent",
			SinceID:    likes[0].ID,
			Count:      100,
			TweetMode:  "extended",
		})
		if err != nil {
			log.WithError(err).Fatal("Could not find tweets")
		}
		log.Infof("Found %d tweets to like", len(t.Statuses))
		for _, tweet := range t.Statuses {
			log.WithFields(logTweet(tweet)).Info("Like tweet")
			if _, ok := os.LookupEnv("DRY"); ok {
				log.Warn("DRY RUN")
				continue
			}
			liked, _, err := client.Favorites.Create(&twitter.FavoriteCreateParams{ID: tweet.ID})
			if err != nil {
				log.WithField("ID", tweet.ID).WithError(err).Error("Could not like tweet 💔")
				continue
			}
			log.WithField("ID", liked.ID).WithField("❤ ", liked.FavoriteCount).WithField("⮔ ", liked.RetweetCount).Infof("Liked tweed %d", tweet.ID)
		}
	}
}

func respondToTweets(client *twitter.Client) {
	flasy := false
	tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
		Count:          1,
		ExcludeReplies: &flasy,
		TweetMode:      "extended",
	})
	if err != nil {
		log.WithError(err).Fatal("Could not get tweets from timeline")
	}
	if err != nil {
		log.WithError(err).Error("Could not find tweets")
		return
	}
	if len(tweets) < 1 {
		log.Infof("No tweets since last time")
		return
	}

	log.WithFields(logTweet(tweets[0])).Info("Latest responded tweet")

	log.WithField("Keyword", "Dz.U.").Debug("Search for tweets to respond")
	t, _, err := client.Search.Tweets(&twitter.SearchTweetParams{
		Query:      "-from:Dziennik_Ustaw AND -filter:retweets AND \"Dz.U.\"",
		Lang:       "pl",
		ResultType: "recent",
		SinceID:    tweets[0].ID,
		Count:      100,
		TweetMode:  "extended",
	})
	if err != nil {
		log.WithError(err).Fatal("Could not find tweets")
	}
	log.Infof("Found %d tweets to responde", len(t.Statuses))
	for _, tweet := range t.Statuses {
		log.WithFields(logTweet(tweet)).Info("Respond tweet")

		year, nr, pos := extractActFromTweet(tweet.FullText)
		if year == 0 && nr == 0 {
			log.WithField("ID", tweet.ID).Debug("Use current year")
			year = time.Now().Year()
		}
		if pos == 0 {
			continue
		}
		log.WithField("ID", tweet.ID).Debugf("Tweet reference Dz.U. %d Poz. %d", year, pos)
		if year < 2012 && nr == 0 {
			log.WithField("ID", tweet.ID).Infof("Acts before 2012 must have number")
			continue
		}

		previouslyTweeted, _, err := client.Search.Tweets(&twitter.SearchTweetParams{
			Query:      fmt.Sprintf("from:Dziennik_Ustaw AND \"Dz.U. %d Poz. %d\"", year, pos),
			Lang:       "pl",
			ResultType: "recent",
			Count:      1,
			TweetMode:  "extended",
		})
		if err != nil {
			log.WithError(err).Fatal("Could not find tweets")
		}
		tweetText := ""
		var mediaIds []int64
		if len(previouslyTweeted.Statuses) > 0 {
			log.WithFields(logTweet(previouslyTweeted.Statuses[0])).Infof("Found tweet with act")
			tweetText = fmt.Sprintf("https://twitter.com/Dziennik_Ustaw/status/%d", previouslyTweeted.Statuses[0].ID)
		} else {
			log.Infof("Preparing new tweet")
			text := getTweetText(year, nr, pos)
			if text == "" {
				log.WithField("ID", tweet.ID).Warn("No data for ", pos)
				continue
			}
			tweetText = text
			mediaIds, err = uploadImages(year, nr, pos, client)
			if err != nil {
				log.WithField("ID", tweet.ID).WithError(err).Fatal("Could not upload images")
			}
		}
		if _, ok := os.LookupEnv("DRY"); ok {
			log.WithField("ID", tweet.ID).WithField("Text", tweetText).Warn("DRY RUN")
			continue
		}
		log.WithField("Text", tweetText).Info("Publishing...")
		t, _, err := client.Statuses.Update(tweetText, &twitter.StatusUpdateParams{
			InReplyToStatusID:         tweet.ID,
			AutoPopulateReplyMetadata: &truthy,
			MediaIds:                  mediaIds,
			Lat:                       &lat,
			Long:                      &long,
			DisplayCoordinates:        &truthy,
		})
		if err != nil {
			log.WithError(err).Error("Could not publish tweet")
		}
		log.WithField("ID", t.ID).WithField("Text", t.Text).Infof("Responded to %d", tweet.ID)
	}
}

func getTweetText(year, nr, pos int) string {
	r, err := http.DefaultClient.Get(fmt.Sprintf("%s/DU/%d/%d", url, year, pos))
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
	return prepareTweet(year, nr, pos, title)
}

func uploadImages(year, nr, pos int, client *twitter.Client) ([]int64, error) {
	url := pdfUrl(year, nr, pos)
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

func prepareTweet(year, nr, id int, title string) string {
	return strings.Join([]string{
		fmt.Sprintf("Dz.U. %d poz. %d #DziennikUstaw", year, id), // 37 chars (Dz.U. YYYY poz. XXXX #DziennikUstaw\n)
		trimTitle(title), // < 280-37-23 ~ 200 (1 for new line)
		pdfUrl(year, nr, id), // 23 chars (The current length of a URL in a Tweet is 23 characters, even if the length of the URL would normally be shorter.)
	}, "\n")
}

func pdfUrl(year, nr, pos int) string {
	return fmt.Sprintf("%s/D%d%03d%04d01.pdf", url, year, nr, pos)
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
	"Państwowej Komisji Wyborczej":                "@PanstwKomWyb",
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

func extractActFromTweet(tweet string) (year, nr, pos int) {
	r := regexp.MustCompile(`(?i)Dz\.\s*U\.\s*z?\s*(?P<year>\d{4})?\s*(r\.?)?\s*(Nr\s*(?P<nr>\d{1,3}),?\s*)?(\s*[Pp]oz)?\.((?P<nr>\d{1,3})\.)?\s*(?P<pos>\d{1,4})`)
	match := r.FindStringSubmatch(tweet) // TODO: Find all matches not just first one
	for i, name := range r.SubexpNames() {
		if i > len(match) {
			return year, nr, pos
		}
		switch name {
		case "year":
			year, _ = strconv.Atoi(match[i])
		case "nr":
			if nr != 0 {
				break
			}
			nr, _ = strconv.Atoi(match[i])
		case "pos":
			pos, _ = strconv.Atoi(match[i])
		}
	}
	return year, nr, pos
}

func logTweet(t twitter.Tweet) log.Fields {
	return log.Fields{
		"ID": t.ID,
		"Date": t.CreatedAt,
		"❤ ": t.FavoriteCount,
		"⮔ ": t.RetweetCount,
		"Text": t.FullText,
	}
}