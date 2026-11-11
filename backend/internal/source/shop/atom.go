package shop

import (
	"bytes"
	"encoding/xml"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AtomLimit is how many of the newest products the catalog's Atom feed
// lists. The feed has no further pages.
const AtomLimit = 25

// atomFeed is the part of the collection feed the source reads. Shopify's
// own elements (s:tag, s:type) are in its namespace; prices and variants
// are never decoded.
type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []atomEntry `xml:"http://www.w3.org/2005/Atom entry"`
}

type atomEntry struct {
	ID        string `xml:"http://www.w3.org/2005/Atom id"`
	Published string `xml:"http://www.w3.org/2005/Atom published"`
	Title     string `xml:"http://www.w3.org/2005/Atom title"`
	Links     []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"http://www.w3.org/2005/Atom link"`
	Summary string   `xml:"http://www.w3.org/2005/Atom summary"`
	Tags    []string `xml:"http://jadedpixel.com/-/spec/shopify tag"`
}

// ParseAtom reads the catalog's Atom feed into products: the numeric ID from
// the entry ID, the handle from the alternate link on Host, the title, the
// publication time, the Shopify tags, and the summary HTML as the
// description. An entry without a usable ID, handle, title or publication
// time is left out.
func ParseAtom(body []byte) ([]Product, error) {
	var feed atomFeed
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Entity = xml.HTMLEntity
	if decoder.Decode(&feed) != nil {
		return nil, &Error{Code: "shop_invalid_atom"}
	}
	products := []Product{}
	for _, entry := range feed.Entries {
		id := atomProductID(entry.ID)
		handle := ""
		for _, link := range entry.Links {
			if link.Rel == "alternate" || link.Rel == "" {
				handle = atomHandle(link.Href)
				break
			}
		}
		title := cleanText(entry.Title, 200)
		published, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.Published))
		if id <= 0 || !handlePattern.MatchString(handle) || title == "" || err != nil {
			continue
		}
		tags := []string{}
		for _, tag := range entry.Tags {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
		products = append(products, Product{ID: id, Handle: handle, Title: title, Tags: tags, PublishedAt: published, Body: entry.Summary})
	}
	return products, nil
}

// atomProductID reads https://<Host>/products/<numeric id>.
func atomProductID(raw string) int64 {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != Host {
		return 0
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(u.Path, "/products/"), 10, 64)
	if err != nil || !strings.HasPrefix(u.Path, "/products/") {
		return 0
	}
	return id
}

// atomHandle reads the handle of https://<Host>/products/<handle>.
func atomHandle(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != Host || !strings.HasPrefix(u.Path, "/products/") {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/products/")
}
