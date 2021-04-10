package main

import (
	"context"
	"fmt"
	"golang.org/x/net/html"
	"net/http"
	"time"
)

// парсим страницу
func parse(url string, requestTimeout int) (*html.Node, error) {
	// что здесь должно быть вместо http.Get? :)
	client := http.Client{
		Timeout: time.Duration(requestTimeout) * time.Second,
	}
	r, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("can't get page")
	}
	b, err := html.Parse(r.Body)
	if err != nil {
		return nil, fmt.Errorf("can't parse page")
	}
	return b, err
}

// ищем заголовок на странице
func pageTitle(ctx context.Context, n *html.Node) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("parsing the header took too long")
	default:
		var title string
		var err error
		if n.Type == html.ElementNode && n.Data == "title" {
			return n.FirstChild.Data, nil
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			title, err = pageTitle(ctx, c)
			if err != nil {
				return title, err
			}
			if title != "" {
				break
			}
		}
		return title, nil
	}
}

// ищем все ссылки на страницы. Используем мапку чтобы избежать дубликатов
func pageLinks(ctx context.Context, links map[string]struct{}, n *html.Node) (map[string]struct{}, error) {
	if links == nil {
		links = make(map[string]struct{})
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("fetching links took too long")
	default:
		var err error
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				if _, ok := links[a.Val]; !ok && len(a.Val) > 2 && a.Val[:2] == "//" {
					links["http://"+a.Val[2:]] = struct{}{}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			links, err = pageLinks(ctx, links, c)
			if err != nil {
				return nil, err
			}
		}
		return links, nil
	}
}
