package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"

	"golang.org/x/net/html"

	log "github.com/sirupsen/logrus"

	"github.com/avast/retry-go"
	"github.com/gen2brain/go-fitz"
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

	likeTweets(client)
	responses, err := respondToTweets(client)
	if err != nil {
		log.WithError(err).Fatal("Could not prepare responses")
	}

	newActs, err := prepareNewActs(client)
	if err != nil {
		log.WithError(err).Fatal("Could not prepare new acts")
	}

	log.WithField("NewActs", len(newActs)).WithField("Responses", len(responses)).Info("Publishing tweets")
	if _, ok := os.LookupEnv("DRY"); ok {
		log.Warn("DRY RUN")
		return
	}
	for _, tweet := range append(newActs, responses...) {
		t, _, err := client.Statuses.Update(tweet.Status, &tweet)
		if err != nil {
			log.WithError(err).Fatal("Could not publish tweet")
		}
		log.WithFields(logTweet(*t)).Info("Published")
	}

}

func prepareNewActs(client *twitter.Client) ([]twitter.StatusUpdateParams, error) {
	tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
		Count:          1,
		ExcludeReplies: &truthy,
		TweetMode:      "extended",
	})
	if err != nil {
		return nil, fmt.Errorf("could not get tweets from timeline: %w", err)
	}

	if len(tweets) < 1 {
		return nil, fmt.Errorf("no tweets")
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

	log.WithField("Current Year", year).Infof("Last tweeted act Dz.U %d pos %d", lastTweetedYear, lastTweetedId)

	newActs := []twitter.StatusUpdateParams{}
	for {
		lastTweetedId++

		tweetText := getTweetText(year, 0, lastTweetedId)
		if tweetText == "" {
			log.WithField("Year", year).WithField("Pos", lastTweetedId).Info("No data")
			break
		}
		mediaIds, err := uploadImages(year, 0, lastTweetedId, client)
		if err != nil {
			return nil, fmt.Errorf("could not upload images: %w", err)
		}

		log.WithField("Text", tweetText).Info("Prepared")
		newActs = append(newActs, twitter.StatusUpdateParams{
			Status:             tweetText,
			Lat:                &lat,
			Long:               &long,
			DisplayCoordinates: &truthy,
			MediaIds:           mediaIds,
		})
	}
	return newActs, nil
}

func likeTweets(client *twitter.Client) {
	likes, _, err := client.Favorites.List(&twitter.FavoriteListParams{
		UserID: 1334198651141361666,
		Count:  1,
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

func respondToTweets(client *twitter.Client) ([]twitter.StatusUpdateParams, error) {
	flasy := false
	tweets, _, err := client.Timelines.UserTimeline(&twitter.UserTimelineParams{
		Count:          1,
		ExcludeReplies: &flasy,
		TweetMode:      "extended",
	})
	if err != nil {
		return nil, fmt.Errorf("could not get tweets from timeline: %w", err)
	}
	if len(tweets) < 1 {
		log.Infof("No tweets since last time")
		return nil, nil
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
		return nil, fmt.Errorf("could not find tweets: %w", err)
	}
	log.Infof("Found %d tweets to responde", len(t.Statuses))

	responses := make([]twitter.StatusUpdateParams, 0, len(t.Statuses))

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
			return nil, fmt.Errorf("could not find tweets with responses: %w", err)
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
				log.WithError(err).Error("could not upload images - skipping)
				continue
			}
		}

		responses = append(responses, twitter.StatusUpdateParams{
			Status:                    tweetText,
			InReplyToStatusID:         tweet.ID,
			AutoPopulateReplyMetadata: &truthy,
			MediaIds:                  mediaIds,
			Lat:                       &lat,
			Long:                      &long,
			DisplayCoordinates:        &truthy,
		})
	}
	return responses, nil
}

func getTweetText(year, nr, pos int) string {
	var r *http.Response
	err := retry.Do(func() error {
		var err error
		r, err = http.DefaultClient.Get(fmt.Sprintf("%s/DU/%d/%d", url, year, pos))
		if err != nil {
			return err
		}
		if r.StatusCode != http.StatusOK {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.WithField("URL", url).WithField("Status", r.StatusCode).WithField("body", string(body)).Debug("Body")
			}
			return fmt.Errorf("unexpected status: %s", r.Status)
		}
		return err
	})
	if err != nil {
		log.WithError(err).Fatal("Could not get data from Dz.U.")
	}
	title := getTitleFromPage(r.Body)
	if title == "" {
		return ""
	}
	return prepareTweet(year, nr, pos, title)
}

func uploadImages(year, nr, pos int, client *twitter.Client) ([]int64, error) {
	r, err := getPDF(year, nr, pos)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	pages, err := convertPDFToJpgs(r.Body)
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

func getPDF(year int, nr int, pos int) (r *http.Response, err error) {
	url := pdfUrl(year, nr, pos)
	return r, retry.Do(func() error {
		r, err = http.DefaultClient.Get(url)
		log.WithField("URL", url).Infof("GET images")
		if err != nil {
			return fmt.Errorf("could not fetch images %w", err)
		}
		if r.StatusCode != http.StatusOK {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.WithField("URL", url).WithField("Status", r.StatusCode).WithField("body", string(body)).Debug("Body")
			}
			return fmt.Errorf("invalid status %s", r.Status)
		}
		return nil
	})
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

func prepareTweet(year, nr, id int, title string) string {
	return strings.Join([]string{
		fmt.Sprintf("Dz.U. %d poz. %d", year, id), // 22 chars (Dz.U. YYYY poz. XXXX\n)
		trimTitle(title),     // < 280-22-23 ~ 230 (1 for new line)
		pdfUrl(year, nr, id), // 23 chars (The current length of a URL in a Tweet is 23 characters, even if the length of the URL would normally be shorter.)
	}, "\n")
}

func pdfUrl(year, nr, pos int) string {
	return fmt.Sprintf("%s/D%d%03d%04d01.pdf", url, year, nr, pos)
}

var handles = map[string]string{
	"Ministra Aktywów Państwowych":                       "@MAPgovPL",
	"Ministra Edukacji i Nauki":                          "@MEIN_gov_PL",
	"Ministra Finansów ":                                 "@MF_gov_PL ",
	"Ministra Finansów, Funduszy i Polityki Regionalnej": "@MF_gov_PL",
	"Ministra Funduszy i Polityki Regionalnej":           "@MFiPR_gov_PL",
	"Ministra Infrastruktury":                            "@MI_gov_PL",
	"Ministra Klimatu":                                   "@MKiS_gov_PL",
	"Ministra Klimatu i Środowiska":                      "@MKiS_gov_PL",
	"Ministra Kultury i Dziedzictwa Narodowego":          "@MKiDN_gov_PL",
	"Ministra Kultury, Dziedzictwa Narodowego i Sportu":  "@MKiDN_gov_PL @SPORT_gov_PL",
	"Ministra Nauki i Szkolnictwa Wyższego":              "@MEIN_gov_PL",
	"Ministra Obrony Narodowej":                          "@MON_gov_PL",
	"Ministra Rodziny i Polityki Społecznej":             "@MRiPS_gov_PL",
	"Ministra Rolnictwa i Rozwoju Wsi":                   "@MRiRW_gov_PL",
	"Ministra Rozwoju, Pracy i Technologii":              "@MRPiT_gov_PL",
	"Ministra Sportu":                                    "@Sport_gov_PL",
	"Ministra Spraw Wewnętrznych i Administracji":        "@MSWiA_gov_PL",
	"Ministra Spraw Zagranicznych":                       "@MSZ_RP",
	"Ministra Sprawiedliwości":                           "@MS_gov_PL",
	"Ministra Zdrowia":                                   "@MZ_gov_PL",
	"Państwowej Komisji Wyborczej":                       "@PanstwKomWyb",
	"Prezesa Rady Ministrów":                             "@PremierRP",
	"Prezydenta Rzeczypospolitej Polskiej":               "@PrezydentPL",
	"Sejmu Rzeczypospolitej Polskiej":                    "@KancelariaSejmu",
	"Trybunału Konstytucyjnego":                          "@TK_gov_PL",
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

	split := strings.Split(title, " ")
	title = ""
	for _, part := range split {
		t := title + part + " "
		if len(t) > MaxTitleLength {
			break
		}
		title = t
	}

	return title + "…"
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
		"ID":   t.ID,
		"Date": t.CreatedAt,
		"❤ ":   t.FavoriteCount,
		"⮔ ":   t.RetweetCount,
		"Text": t.FullText,
	}
}
