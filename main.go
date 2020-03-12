package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/ddliu/go-httpclient"
)

var targetRelease string
var sourceBug string

func init() {
	httpclient.Defaults(httpclient.Map{
		httpclient.OPT_USERAGENT: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.5 Safari/605.1.15",
		"Accept-Language":        "en-us",
	})

	flag.StringVar(&targetRelease, "target", "", "Specify the target release (eg. '4.3.z')")
	flag.StringVar(&sourceBug, "bug", "", "Specify the source bug (eg. '1812863')")
}

func doBugzillaAuthenticationDance() (*httpclient.HttpClient, error) {
	// first visit bugzilla index page so it gives you the cookie
	getLoginTokenCookie, err := httpclient.Get("https://bugzilla.redhat.com/index.cgi")
	if err != nil {
		return nil, err
	}

	// now that you have cookie, BZ will give you Bugzilla_login_token
	getLoginTokenValue, err := httpclient.WithCookie(getLoginTokenCookie.Cookies()...).Get("https://bugzilla.redhat.com/index.cgi")
	if err != nil {
		return nil, err
	}

	// now perform login via login form
	var (
		loginErr error
		cookies  []*http.Cookie
	)
	loginTokenDoc, _ := goquery.NewDocumentFromReader(getLoginTokenValue.Body)
	loginTokenDoc.Find(".mini_login input").Each(func(i int, selection *goquery.Selection) {
		name, exists := selection.Attr("name")
		if !exists {
			return
		}
		value, _ := selection.Attr("value")
		if name == "Bugzilla_login_token" {
			response, err := httpclient.Post("https://bugzilla.redhat.com/index.cgi", map[string]string{
				"Bugzilla_login":       os.Getenv("BUGZILLA_EMAIL"),
				"Bugzilla_password":    os.Getenv("BUGZILLA_PASSWORD"),
				"Bugzilla_login_token": value,
				"GoAheadAndLogIn":      "1",
			})
			if err != nil {
				loginErr = err
				return
			}
			cookies = response.Cookies()
			return
		}
	})

	return httpclient.WithCookie(cookies...), loginErr
}

func parseCloneBugPostRequest(page io.ReadCloser) map[string]string {
	cloneDoc, _ := goquery.NewDocumentFromReader(page)
	result := map[string]string{}
	cloneDoc.Find(".enter_bug_form input").Each(func(i int, input *goquery.Selection) {
		name, exists := input.Attr("name")
		if !exists {
			return
		}
		value, _ := input.Attr("value")
		result[name] = value
	})
	cloneDoc.Find(".enter_bug_form select").Each(func(i int, selection *goquery.Selection) {
		name, exists := selection.Attr("name")
		if !exists {
			return
		}
		selection.Find("option").Each(func(i int, option *goquery.Selection) {
			_, exists := option.Attr("selected")
			if !exists {
				return
			}
			result[name] = option.AttrOr("value", "")
		})
	})
	cloneDoc.Find(".enter_bug_form textarea").Each(func(i int, textarea *goquery.Selection) {
		name, exists := textarea.Attr("name")
		if !exists {
			return
		}
		result[name] = textarea.Contents().Text()
	})
	delete(result, "maketemplate")
	return result
}

type bug struct {
	description string
	url         string
	id          string
}

func parseClonedBug(page io.Reader) *bug {
	b := &bug{}
	cloneDoc, _ := goquery.NewDocumentFromReader(page)
	cloneDoc.Find("#changeform .bz_short_desc_container a").Each(func(i int, selection *goquery.Selection) {
		anchor, hasHref := selection.Attr("href")
		if len(b.url) > 0 {
			return
		}
		if hasHref {
			if strings.HasPrefix(anchor, "show_bug.cgi?id=") {
				b.url = "https://bugzilla.redhat.com/" + anchor
				b.id = strings.TrimPrefix(anchor, "show_bug.cgi?id=")
			}
		}
	})
	b.description = cloneDoc.Find("#changeform span#short_desc_nonedit_display").First().Contents().Text()
	return b
}

func main() {
	flag.Parse()

	if len(targetRelease) == 0 || len(sourceBug) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	if len(os.Getenv("BUGZILLA_EMAIL")) == 0 || len(os.Getenv("BUGZILLA_PASSWORD")) == 0 {
		log.Fatal("You must set BUGZILLA_EMAIL and BUGZILLA_PASSWORD environment variables.")
	}

	client, err := doBugzillaAuthenticationDance()
	if err != nil {
		log.Fatalf("Failed to authenticate to bugzilla: %v", err)
	}

	clonePage, err := client.Get("https://bugzilla.redhat.com/enter_bug.cgi?product=OpenShift%20Container%20Platform&cloned_bug_id=" + sourceBug)
	if err != nil {
		log.Fatal(err)
	}

	postParams := parseCloneBugPostRequest(clonePage.Body)
	postParams["target_release"] = targetRelease

	cloneResponse, err := client.Post("https://bugzilla.redhat.com/post_bug.cgi", postParams)
	if err != nil {
		log.Fatal(err)
	}

	b := parseClonedBug(cloneResponse.Body)
	fmt.Printf("[%s] Bug %s: %s [%s]\n", postParams["target_release"], b.id, b.description, b.url)
}
