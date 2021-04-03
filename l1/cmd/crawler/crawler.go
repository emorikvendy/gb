package main

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"sync"
	"time"
)

type crawlResult struct {
	err error
	msg string
}

type crawler struct {
	sync.Mutex
	visited        map[string]string
	maxDepth       int
	pageTimeout    int
	linksTimeout   int
	headerTimeout  int
	requestTimeout int
}

func newCrawler(maxDepth, pageTimeout, linksTimeout, headerTimeout, requestTimeout int) *crawler {
	return &crawler{
		visited:        make(map[string]string),
		maxDepth:       maxDepth,
		pageTimeout:    pageTimeout,
		linksTimeout:   linksTimeout,
		headerTimeout:  headerTimeout,
		requestTimeout: requestTimeout,
	}
}

func (c *crawler) increaseDepth(value int) {
	c.maxDepth += value
	log.Printf("current depth is %d", c.maxDepth)
}

// рекурсивно сканируем страницы
func (c *crawler) run(ctx context.Context, url string, results chan<- crawlResult, done chan struct{}, wg *sync.WaitGroup, depth int) {
	if depth == 0 {
		defer func() {
			wg.Wait()
			log.Printf("exit url: %s OK", url)
			done <- struct{}{}
			close(done)
		}()
	} else {
		defer func() {
			log.Printf("exit url: %s OK", url)
			wg.Done()
		}()
	}
	log.Printf("start url: %s", url)
	// просто для того, чтобы успевать следить за выводом программы, можно убрать :)
	time.Sleep(20 * time.Second)
	localCtx, localCancel := context.WithTimeout(context.Background(), time.Duration(c.pageTimeout)*time.Second)
	defer localCancel()
	// проверяем что контекст исполнения актуален
	select {
	case <-ctx.Done():
		log.Printf("exit url: %s ctx.Done", url)
		return

	default:
		// проверка глубины
		if depth >= c.maxDepth {
			log.Printf("exit url: %s depth %d >= %d", url, depth, c.maxDepth)
			return
		}

		page, err := parse(url, c.requestTimeout)
		if err != nil {
			// ошибку отправляем в канал, а не обрабатываем на месте
			results <- crawlResult{
				err: errors.Wrapf(err, "parse page %s", url),
			}
			log.Printf("exit url: %s err", url)
			return
		}
		select {
		case <-localCtx.Done():
			log.Printf("exit url: %s localCtx.Done", url)
			return
		default:
		}

		hCtx, hCancel := context.WithTimeout(context.Background(), time.Duration(c.headerTimeout)*time.Second)
		title := pageTitle(page, hCtx)
		hCancel()
		select {
		case <-localCtx.Done():
			log.Printf("exit url: %s localCtx.Done", url)
			return
		default:
		}
		lCtx, lCancel := context.WithTimeout(context.Background(), time.Duration(c.linksTimeout)*time.Second)
		links := pageLinks(nil, page, lCtx)
		lCancel()
		select {
		case <-localCtx.Done():
			log.Printf("exit url: %s localCtx.Done", url)
			return
		default:
		}

		// блокировка требуется, т.к. мы модифицируем мапку в несколько горутин
		c.Lock()
		c.visited[url] = title
		c.Unlock()

		// отправляем результат в канал, не обрабатывая на месте
		results <- crawlResult{
			err: nil,
			msg: fmt.Sprintf("%s -> %s\n", url, title),
		}

		select {
		case <-localCtx.Done():
			log.Printf("exit url: %s localCtx.Done", url)
			return
		default:
		}
		// рекурсивно ищем ссылки
		for link := range links {
			// если ссылка не найдена, то запускаем анализ по новой ссылке
			if c.checkVisited(link) {
				continue
			}
			wg.Add(1)
			go c.run(ctx, link, results, done, wg, depth+1)
		}
	}
}

func (c *crawler) checkVisited(url string) bool {
	c.Lock()
	defer c.Unlock()

	_, ok := c.visited[url]
	return ok
}
