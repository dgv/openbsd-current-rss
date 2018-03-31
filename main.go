package main

import (
	"encoding/xml"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"appengine"
	"appengine/urlfetch"

	"golang.org/x/net/html"
)

const OPENBSD_CURRENT_URL = "http://www.openbsd.org/faq/current.html"

var entries    = make([]Entry, 0)

type Atom struct {
	XMLName xml.Name `xml:"feed"`
	Xmlns   string   `xml:"xmlns,attr"`
	Title   string   `xml:"title"`
	Link    []Link   `xml:"link"`
	Updated string   `xml:"updated"`
	Id      string   `xml:"id"`
	Name    string   `xml:"author>name"`
	Email   string   `xml:"author>email"`
	Entry   []Entry  `xml:"entry"`
}

type Entry struct {
	Title   string `xml:"title"`
	Link    Link   `xml:"link"`
	Updated string `xml:"updated"`
	Id      string `xml:"id"`
	Content string `xml:"content"`
	Type    string `xml:"type,attr"`
}

type Link struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

func serveError(c appengine.Context, w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "Internal Server Error")
	c.Errorf("%v", err)
}

// parse entries by h3 tag
func parseEntries(body io.ReadCloser) (entries []Entry) {
	z := html.NewTokenizer(body)
	depth := 0
	var id, date, title, content string
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			// end of the document, we're done
			return
		case html.TextToken:
			if depth > 0 {
				//println(string(z.Text()))
				// parse title <date> - <title>
				_t := string(z.Text())
				t := strings.Split(_t, "-")
				if len(t) >= 2 {
					d := strings.Split(strings.TrimSpace(t[0]), "/")
					if len(d) == 3 {
						dd, _ := strconv.Atoi(d[2])
						mm, _ := strconv.Atoi(d[1])
						yyyy, _ := strconv.Atoi(d[0])
						date = time.Date(yyyy, time.Month(mm), dd, 0, 0, 0, 0, time.Local).Format(time.RFC3339)
					}
					title = _t[strings.Index(_t, "-")+2:]
				} else {
					title += _t
				}
			}
			if title != "" {
				if data := string(z.Text()); data != "" {
					content += data
				}
			}
		case html.StartTagToken, html.EndTagToken:
			t := z.Token()
			// write previous entry or the last one (before hr)
			if title != "" && ((t.Data == "h3" && tt == html.StartTagToken) || t.Data == "hr") {
				entries = append(entries, Entry{Title: strings.TrimSpace(title), Updated: date, Id: id, Content: content, Type: "html", Link: Link{Href: id}})
			}
			if t.Data == "h3" {
				if tt == html.StartTagToken {
					depth++
					title = ""
					content = ""
				} else {
					depth--
				}
				for _, a := range t.Attr {
					if a.Key == "id" {
						id = OPENBSD_CURRENT_URL + "#" + a.Val
						break
					}
				}
			} else if title != "" {
				if tt == html.StartTagToken {
					if depth > 0 {
						title += "<"
					} else {
						content += "<"
					}
				} else {
					if depth > 0 {
						title += "</"
					} else {
						content += "</"
					}
				}
				if depth > 0 {
					title += t.Data
				} else {
					content += t.Data
				}
				for _, a := range t.Attr {
					if depth > 0 {
						title += " " + a.Key + "=\"" + a.Val + "\""
					} else {
						content += " " + a.Key + "=\"" + a.Val + "\""
					}
				}
				if depth > 0 {
					title += ">"
				} else {
					content += ">"
				}
			}
		}
	}
}

func handle(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if len(entries) == 0 {
		c.Infof("loading entries")
		client := urlfetch.Client(c)
		res, err := client.Get(OPENBSD_CURRENT_URL)
		if err != nil {
			serveError(c, w, err)
			return
		}
		defer res.Body.Close()
		entries = parseEntries(res.Body)
	}
	// encode entries into atom(rss)
	v := &Atom{Xmlns: "http://www.w3.org/2005/Atom", Title: "OpenBSD Current Updates", Updated: time.Now().Format(time.RFC3339), Id: "http://openbsd-current-rss.appspot.com", Name: "sthen", Email: "sthen@openbsd.org", Link: []Link{{"http://openbsd-current-rss.appspot.com", "self"}, {Href: "http://openbsd-current-rss.appspot.com"}}, Entry: entries}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	enc := xml.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		serveError(c, w, err)
	}
}

func reload(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
		c.Infof("reloading entries")
		client := urlfetch.Client(c)
		res, err := client.Get(OPENBSD_CURRENT_URL)
		if err != nil {
			serveError(c, w, err)
			return
		}
		defer res.Body.Close()
		entries = parseEntries(res.Body)
}

func init() {
	http.HandleFunc("/", handle)
	http.HandleFunc("/reload", reload)
}
