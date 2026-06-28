package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
)

func main() {
	urlStr := "https://en.wikipedia.org/wiki/List_of_countries_by_GDP_(nominal)"
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ketch/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		panic(err)
	}
	html := string(body)
	fmt.Printf("RAW html bytes: %d, status: %d\n", len(html), resp.StatusCode)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		panic(err)
	}
	rawTables := doc.Find("table").Length()
	fmt.Printf("RAW tables: %d\n", rawTables)

	p := readability.NewParser()
	u, err := url.Parse(urlStr)
	if err != nil {
		panic(err)
	}
	article, err := p.Parse(strings.NewReader(html), u)
	if err != nil {
		fmt.Println("parse err:", err)
		return
	}
	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		panic(err)
	}
	rd := buf.String()
	fmt.Printf("READABILITY html bytes: %d\n", len(rd))
	rdoc, err := goquery.NewDocumentFromReader(strings.NewReader(rd))
	if err != nil {
		panic(err)
	}
	fmt.Printf("READABILITY tables: %d\n", rdoc.Find("table").Length())
	if err := os.WriteFile("/tmp/rd.html", []byte(rd), 0644); err != nil {
		panic(err)
	}
	if err := os.WriteFile("/tmp/raw.html", []byte(html), 0644); err != nil {
		panic(err)
	}
}
