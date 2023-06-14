package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	oldApi "github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/g8rswimmer/go-twitter/v2"

	"golang.org/x/net/html"

	log "github.com/sirupsen/logrus"

	"github.com/avast/retry-go"
	"github.com/gen2brain/go-fitz"
)

const url = "https://dziennikustaw.gov.pl"

var (
	lat  = 52.22548
	long = 21.02839

	userID = "1334198651141361666"
)

type authorizer struct{}

func (a *authorizer) Add(_ *http.Request) {}

func main() {

	log.SetLevel(log.DebugLevel)

	log.Info("Dziennik Ustaw")

	ctx := context.Background()

	config := oauth1.NewConfig(os.Getenv("consumerKey"), os.Getenv("consumerSecret"))
	token := oauth1.NewToken(os.Getenv("accessToken"), os.Getenv("accessSecret"))
	// http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := &twitter.Client{
		Authorizer: &authorizer{},
		Client:     httpClient,
		Host:       "https://api.twitter.com",
	}
	oldClient := oldApi.NewClient(httpClient)

	u, err := client.AuthUserLookup(context.Background(), twitter.UserLookupOpts{
		UserFields: []twitter.UserField{
			// UserFieldCreatedAt is the UTC datetime that the user account was created on Twitter.
			twitter.UserFieldCreatedAt,
			// UserFieldDescription is the text of this user's profile description (also known as bio), if the user provided one.
			twitter.UserFieldDescription,
			// UserFieldEntities contains details about text that has a special meaning in the user's description.
			twitter.UserFieldEntities,
			// UserFieldID is the unique identifier of this user.
			twitter.UserFieldID,
			// UserFieldLocation is the location specified in the user's profile, if the user provided one.
			twitter.UserFieldLocation,
			// UserFieldName is the name of the user, as they’ve defined it on their profile
			twitter.UserFieldName,
			// UserFieldPinnedTweetID is the unique identifier of this user's pinned Tweet.
			twitter.UserFieldPinnedTweetID,
			// UserFieldProfileImageURL is the URL to the profile image for this user, as shown on the user's profile.
			twitter.UserFieldProfileImageURL,
			// UserFieldProtected indicates if this user has chosen to protect their Tweets (in other words, if this user's Tweets are private).
			twitter.UserFieldProtected,
			// UserFieldPublicMetrics contains details about activity for this user.
			twitter.UserFieldPublicMetrics,
			// UserFieldURL is the URL specified in the user's profile, if present.
			twitter.UserFieldURL,
			// UserFieldUserName is the Twitter screen name, handle, or alias that this user identifies themselves with
			twitter.UserFieldUserName,
			// UserFieldVerified indicates if this user is a verified Twitter User.
			twitter.UserFieldVerified,
			// UserFieldWithHeld contains withholding details
			twitter.UserFieldWithHeld,
		},
	})
	if err != nil {
		log.WithError(err).Fatal("Could not prepare responses")
	}
	log.WithFields(logLimit(u.RateLimit)).Info(spew.Sdump(u.Raw))

	// 	err := checkHandlesAreValid(client)
	// 	if err != nil {
	// 		log.WithError(err).Fatal("Some handles are invalid")
	// 	}

	//likeTweets(client)
	//responses, err := respondToTweets(client, oldClient)
	//if err != nil {
	//	log.WithError(err).Fatal("Could not prepare responses")
	//}

	newActs, err := prepareNewActs(oldClient)
	if err != nil {
		log.WithError(err).Fatal("Could not prepare new acts")
	}

	log.WithField("NewActs", len(newActs)).Info("Publishing tweets")
	if _, ok := os.LookupEnv("DRY"); ok {
		log.Warn("DRY RUN")
		return
	}
	for _, tw := range append(newActs) {
		t, err := client.CreateTweet(ctx, tw)
		if err != nil {
			log.WithError(err).Fatal("Could not publish tweet")
		}
		log.WithField("Text", t.Tweet.Text).Info("Published")
		err = os.WriteFile("last.txt", []byte(tw.Text), 0x777)
		if err != nil {
			log.WithError(err).Fatal("Could save published tweet")
		}
	}

}

func prepareNewActs(old *oldApi.Client) ([]twitter.CreateTweetRequest, error) {
	//tweets, err := client.UserTweetTimeline(context.Background(), userID, twitter.UserTweetTimelineOpts{
	//	Excludes: []twitter.Exclude{twitter.ExcludeReplies, twitter.ExcludeRetweets},
	//})
	//if err != nil {
	//	return nil, fmt.Errorf("could not get tweets from timeline: %w", err)
	//}
	//
	//if len(tweets.Raw.Tweets) < 1 {
	//	return nil, fmt.Errorf("no tweets")
	//}
	//
	//tweet := tweets.Raw.Tweets[0]
	//log.WithFields(logTweet(tweet)).WithFields(logLimit(tweets.RateLimit)).Debug("Latest tweet from timeline")
	//
	//lastTweetedYear, lastTweetedId := getIdFromTweet(tweet.Text)
	lastTweetedYear, lastTweetedId := getLastId()
	if lastTweetedYear*lastTweetedId == 0 {
		log.WithField("Year", lastTweetedYear).WithField("Pos", lastTweetedId).Fatal("There is a problem with obtaining last tweeted act")
	}
	year := time.Now().Year()
	if year != lastTweetedYear {
		lastTweetedId = 0
	}

	log.WithField("Current Year", year).Infof("Last tweeted act Dz.U %d pos %d", lastTweetedYear, lastTweetedId)

	var newActs []twitter.CreateTweetRequest
	for {
		lastTweetedId++

		tweetText := getTweetText(year, 0, lastTweetedId)
		if tweetText == "" {
			log.WithField("Year", year).WithField("Pos", lastTweetedId).Info("No data")
			break
		}
		mediaIds, err := uploadImages(year, 0, lastTweetedId, old)
		if err != nil {
			return nil, fmt.Errorf("could not upload images: %w", err)
		}

		log.WithField("Text", tweetText).Info("Prepared")
		newActs = append(newActs, twitter.CreateTweetRequest{
			DirectMessageDeepLink: "",
			ForSuperFollowersOnly: false,
			QuoteTweetID:          "",
			Text:                  tweetText,
			ReplySettings:         "",
			Media: &twitter.CreateTweetMedia{
				IDs: mediaIds,
			},
		})
	}
	return newActs, nil
}

func likeTweets(client *twitter.Client) {
	likes, err := client.UserLikesLookup(context.Background(), userID, twitter.UserLikesLookupOpts{
		MaxResults: 10,
	})
	if err != nil {
		log.WithError(err).Error("Could not find tweets")
		return
	}
	if len(likes.Raw.Tweets) < 1 {
		log.Infof("No likes since last time")
		return
	}

	log.WithFields(logTweet(likes.Raw.Tweets[0])).Info("Latest liked tweet")

	keywords := []string{
		"#DziennikUstaw", "Dziennik Ustaw", "Dzienniku Ustaw", "Dziennika Ustaw", "Dziennikiem Ustaw", "Dziennikowi Ustaw",
	}

	for _, keyword := range keywords {
		log.WithField("Keyword", keyword).Debug("Search for tweets")
		t, err := client.TweetSearch(context.Background(), "-from:Dziennik_Ustaw "+keyword, twitter.TweetSearchOpts{
			SinceID: likes.Raw.Tweets[0].ID,
		})
		if err != nil {
			log.WithError(err).Fatal("Could not find tweets")
		}
		log.Infof("Found %d tweets to like", len(t.Raw.Tweets))
		for _, tweet := range t.Raw.Tweets {
			log.WithFields(logTweet(tweet)).Info("Like tweet")
			if _, ok := os.LookupEnv("DRY"); ok {
				log.Warn("DRY RUN")
				continue
			}
			liked, err := client.UserLikes(context.Background(), userID, tweet.ID)
			if err != nil {
				log.WithField("ID", tweet.ID).WithError(err).Error("Could not like tweet 💔")
				continue
			}
			log.WithFields(logLimit(liked.RateLimit)).Infof("Liked tweed %s", tweet.ID)
		}
	}
}

func respondToTweets(client *twitter.Client, old *oldApi.Client) ([]twitter.CreateTweetRequest, error) {
	tweets, err := client.UserTweetTimeline(context.Background(), userID, twitter.UserTweetTimelineOpts{
		Excludes:   []twitter.Exclude{twitter.ExcludeRetweets, twitter.ExcludeReplies},
		StartTime:  time.Time{},
		EndTime:    time.Time{},
		MaxResults: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("could not get tweets from timeline: %w", err)
	}
	if len(tweets.Raw.Tweets) < 1 {
		log.Infof("No tweets since last time")
		return nil, nil
	}

	log.WithFields(logTweet(tweets.Raw.Tweets[0])).Info("Latest responded tweet")

	log.WithField("Keyword", "Dz.U.").Debug("Search for tweets to respond")
	t, err := client.TweetSearch(context.Background(), "-from:Dziennik_Ustaw AND -filter:retweets AND \"Dz.U.\"", twitter.TweetSearchOpts{
		MaxResults: 10,
		SortOrder:  twitter.TweetSearchSortOrderRecency,
		SinceID:    tweets.Raw.Tweets[0].ID,
	})
	if err != nil {
		return nil, fmt.Errorf("could not find tweets: %w", err)
	}
	log.WithFields(logLimit(t.RateLimit)).Infof("Found %d tweets to responde", len(t.Raw.Tweets))

	responses := make([]twitter.CreateTweetRequest, 0, len(t.Raw.Tweets))

	for _, tweet := range t.Raw.Tweets {
		log.WithFields(logTweet(tweet)).Info("Respond tweet")

		year, nr, pos := extractActFromTweet(tweet.Text)
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

		previouslyTweeted, err := client.TweetSearch(context.Background(), fmt.Sprintf("from:Dziennik_Ustaw AND \"Dz.U. %d Poz. %d\"", year, pos), twitter.TweetSearchOpts{
			SortOrder:  twitter.TweetSearchSortOrderRecency,
			MaxResults: 10,
		})
		if err != nil {
			return nil, fmt.Errorf("could not find tweets with responses: %w", err)
		}
		tweetText := ""
		var mediaIds []string
		if len(previouslyTweeted.Raw.Tweets) > 0 {
			log.WithFields(logTweet(previouslyTweeted.Raw.Tweets[0])).Infof("Found tweet with act")
			tweetText = fmt.Sprintf("https://twitter.com/Dziennik_Ustaw/status/%s", previouslyTweeted.Raw.Tweets[0].ID)
		} else {
			log.Infof("Preparing new tweet")
			text := getTweetText(year, nr, pos)
			if text == "" {
				log.WithField("ID", tweet.ID).Warn("No data for ", pos)
				continue
			}
			tweetText = text
			mediaIds, err = uploadImages(year, nr, pos, old)
			if err != nil {
				log.WithError(err).Error("could not upload images - skipping")
				continue
			}
		}

		responses = append(responses, twitter.CreateTweetRequest{
			Text: tweetText,
			Media: &twitter.CreateTweetMedia{
				IDs: mediaIds,
			},
			Reply: &twitter.CreateTweetReply{
				InReplyToTweetID: tweet.ID,
			},
		})
	}
	return responses, nil
}

var client = &http.Client{Transport: &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}}

func getTweetText(year, nr, pos int) string {
	var r *http.Response
	err := retry.Do(func() error {
		var err error
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/DU/%d/%d", url, year, pos), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Android 4.4; Tablet; rv:41.0) Gecko/41.0 Firefox/41.0")
		r, err = client.Do(req)
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

func uploadImages(year, nr, pos int, client *oldApi.Client) ([]string, error) {
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
	mediaIds := make([]string, 0, len(pages))
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
		mediaIds = append(mediaIds, fmt.Sprintf("%d", resp.MediaID))
	}
	return mediaIds, nil
}

func getPDF(year int, nr int, pos int) (r *http.Response, err error) {
	url := pdfUrl(year, nr, pos)
	return r, retry.Do(func() error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Android 4.4; Tablet; rv:41.0) Gecko/41.0 Firefox/41.0")
		r, err = client.Do(req)
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
	poz := fmt.Sprintf("%d", id)
	if id == 100 {
		poz = "💯"
	}
	return strings.Join([]string{
		fmt.Sprintf("Dz.U. %d poz. %s", year, poz), // 22 chars (Dz.U. YYYY poz. XXXX\n)
		trimTitle(title),     // < 280-22-23 ~ 230 (1 for new line)
		pdfUrl(year, nr, id), // 23 chars (The current length of a URL in a Tweet is 23 characters, even if the length of the URL would normally be shorter.)
	}, "\n")
}

func pdfUrl(year, nr, pos int) string {
	return fmt.Sprintf("%s/D%d%03d%04d01.pdf", url, year, nr, pos)
}

var handles = map[string]string{
	"Agencji Restrukturyzacji i Modernizacji Rolnictwa":  "@ARiMR_GOV_PL",
	"Centralnego Biura Antykorupcyjnego":                 "@CBAgovPL",
	"Centralnym Biurze Antykorupcyjnym":                  "@CBAgovPL",
	"Głównego Inspektora Transportu Drogowego":           "@ITD_gov",
	"Ministra Aktywów Państwowych":                       "@MAPgovPL",
	"Ministra Edukacji i Nauki":                          "@MEIN_gov_PL",
	"Ministra Finansów ":                                 "@MF_gov_PL ",
	"Ministra Finansów, Funduszy i Polityki Regionalnej": "@MF_gov_PL",
	"Ministra Funduszy i Polityki Regionalnej":           "@MFiPR_gov_PL",
	"Ministra Infrastruktury":                            "@MI_gov_PL",
	"Ministra Klimatu i Środowiska":                      "@MKiS_gov_PL",
	"Ministra Klimatu":                                   "@MKiS_gov_PL",
	"Ministra Kultury i Dziedzictwa Narodowego":          "@kultura_gov_pl",
	"Ministra Kultury, Dziedzictwa Narodowego i Sportu":  "@kultura_gov_pl",
	"Ministra Nauki i Szkolnictwa Wyższego":              "@MEIN_gov_PL",
	"Ministra Obrony Narodowej":                          "@MON_gov_PL",
	"Ministra Rodziny i Polityki Społecznej":             "@MRiPS_gov_PL",
	"Ministra Rolnictwa i Rozwoju Wsi":                   "@MRiRW_gov_PL",
	"Ministra Rozwoju i Technologii":                     "@MRiTGOVPL",
	"Ministra Rozwoju, Pracy i Technologii":              "@MRiTGOVPL",
	"Ministra Sportu":                                    "@Sport_gov_PL",
	"Ministra Spraw Wewnętrznych i Administracji":        "@MSWiA_gov_PL",
	"Ministra Spraw Zagranicznych":                       "@MSZ_RP",
	"Ministra Sprawiedliwości":                           "@MS_gov_PL",
	"Ministra Zdrowia":                                   "@MZ_gov_PL",
	"Państwowej Komisji Wyborczej":                       "@PanstwKomWyb",
	"Państwowej Straży Pożarnej":                         "@KGPSP",
	"Prezesa Rady Ministrów":                             "@PremierRP",
	"Prezydenta Rzeczypospolitej Polskiej":               "@PrezydentPL",
	"Sejmu Rzeczypospolitej Polskiej":                    "@KancelariaSejmu",
	"Straży Granicznej":                                  "@Straz_Graniczna",
	"Trybunału Konstytucyjnego":                          "@TK_gov_PL",
}

var emojis = map[string]string{
	"Obwieszczenie": "📢",
	"Umowa":         "🤝",
	"Porozumienie":  "🤝",
}

//func checkHandlesAreValid(client *twitter.Client) error {
//
//	names := make(map[string]struct{}, len(handles))
//	for _, n := range handles {
//		names[strings.Trim(strings.ToLower(n), " ")] = struct{}{}
//	}
//
//	var checked []string
//
//	var cursor int64
//	for {
//		friends, _, err := client.Friends.List(&twitter.FriendListParams{
//			UserID: userID,
//			Cursor: cursor,
//		})
//		if err != nil {
//			return fmt.Errorf("could not get list of frineds: %q", err)
//		}
//
//		for _, u := range friends.Users {
//			handle := strings.ToLower("@" + u.ScreenName)
//			delete(names, handle)
//			checked = append(checked, handle)
//		}
//
//		cursor = friends.NextCursor
//		if cursor == 0 {
//			break
//		}
//	}
//	if len(names) != 0 {
//		return fmt.Errorf("not found %d handles: %v in %v", len(names), names, strings.Join(checked, ", "))
//	}
//
//	return nil
//
//}

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

func getLastId() (year, id int) {
	file, err := os.ReadFile("last.txt")
	if err != nil {
		log.WithError(err)
		return 0, 0
	}
	return getIdFromTweet(string(file))
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

func logTweet(t *twitter.TweetObj) log.Fields {
	return log.Fields{
		"ID":   t.ID,
		"Date": t.CreatedAt,
		"❤ ":   t.PublicMetrics.Likes,
		"⮔ ":   t.PublicMetrics.Retweets,
		"Text": t.Text,
	}
}

func logLimit(t *twitter.RateLimit) log.Fields {
	return log.Fields{
		"Limit":     t.Limit,
		"Reset":     t.Reset.Time().Sub(time.Now()).String(),
		"Remaining": t.Remaining,
	}
}
