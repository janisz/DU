package main

import (
	"bytes"
	"context"
	"crypto/tls"
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
		log.WithFields(logLimit(t.RateLimit)).WithField("Text", t.Tweet.Text).Info("Published")
		err = os.WriteFile("last.txt", []byte(tw.Text), 0x777)
		if err != nil {
			log.WithError(err).Fatal("Could save published tweet")
		}
	}

}

func prepareNewActs(old *oldApi.Client) ([]twitter.CreateTweetRequest, error) {
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
		var media *twitter.CreateTweetMedia
		if len(mediaIds) > 0 {
			media = &twitter.CreateTweetMedia{
				IDs: mediaIds,
			}
		}
		warsaw := "535f0c2de0121451"
		newActs = append(newActs, twitter.CreateTweetRequest{
			ForSuperFollowersOnly: false,
			Text:                  tweetText,
			Media:                 media,
			Geo: &twitter.CreateTweetGeo{
				PlaceID: warsaw,
			},
		})
	}
	return newActs, nil
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
		req.Header.Set("User-Agent", "curl/7.58.0")
		req.Header.Set("Accept", "*/*")
		r, err = client.Do(req)
		if err != nil {
			return err
		}
		if r.StatusCode != http.StatusOK {
			body, err := io.ReadAll(r.Body)
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
		mID := resp.MediaIDString

		if resp.ProcessingInfo != nil {
			log.WithField("MediaID", mID).Debugf("Still processing: %#v", resp.ProcessingInfo)
			for {
				time.Sleep(100 * time.Millisecond)
				log.WithField("MediaID", mID).Debugf("Checking upload status %s", mID)
				r, _, err := client.Media.Status(resp.MediaID)
				if err != nil {
					return nil, err
				}
				if r.ProcessingInfo == nil {
					break
				}
				log.WithField("MediaID", mID).Debugf("Still processing: %#v", r.ProcessingInfo)
			}
		}
		log.WithField("MediaID", mID).Debug("Upload Succesful")
		mediaIds = append(mediaIds, mID)
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
	"Marszałka Sejmu Rzeczypospolitej Polskiej":          "@szymon_holownia",
	"Ministra Aktywów Państwowych":                       "@MAPgovPL",
	"Ministra Edukacji":                          	      "@MEN_GOVPL",
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

func logLimit(t *twitter.RateLimit) log.Fields {
	return log.Fields{
		"Limit":     t.Limit,
		"Reset":     t.Reset.Time().Sub(time.Now()).String(),
		"Remaining": t.Remaining,
	}
}
