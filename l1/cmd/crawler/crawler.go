package main

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type crawlResult struct {
	err error
	msg string
}

type crawler struct {
	sync.Mutex
	visited        map[string]string
	maxDepth       int64
	pageTimeout    int
	linksTimeout   int
	headerTimeout  int
	requestTimeout int
}

func newCrawler(maxDepth, pageTimeout, linksTimeout, headerTimeout, requestTimeout int) *crawler {
	return &crawler{
		visited:        make(map[string]string),
		maxDepth:       int64(maxDepth),
		pageTimeout:    pageTimeout,
		linksTimeout:   linksTimeout,
		headerTimeout:  headerTimeout,
		requestTimeout: requestTimeout,
	}
}

func (c *crawler) increaseDepth(value int64) {
	atomic.AddInt64(&c.maxDepth, value)
	//c.maxDepth += value
	log.Printf("current depth is %d", atomic.LoadInt64(&c.maxDepth))
}

// рекурсивно сканируем страницы
func (c *crawler) run(ctx context.Context, url string, results chan<- crawlResult, done chan struct{}, wg *sync.WaitGroup, depth int64) {
	if depth == 0 {
		defer func() {
			wg.Wait()
			done <- struct{}{}
			close(done)
		}()
	} else {
		defer func() {
			wg.Done()
		}()
	}
	// просто для того, чтобы успевать следить за выводом программы, можно убрать :)
	time.Sleep(2 * time.Second)
	localCtx, localCancel := context.WithTimeout(context.Background(), time.Duration(c.pageTimeout)*time.Second)
	defer localCancel()
	// проверяем что контекст исполнения актуален
	select {
	case <-ctx.Done():
		return

	default:
		// проверка глубины
		if depth >= atomic.LoadInt64(&c.maxDepth) {
			return
		}

		page, err := parse(url, c.requestTimeout)
		if err != nil {
			results <- crawlResult{
				err: errors.Wrapf(err, "parse page %s", url),
			}
			return
		}
		select {
		case <-localCtx.Done():
			return
		default:
		}

		hCtx, hCancel := context.WithTimeout(context.Background(), time.Duration(c.headerTimeout)*time.Second)
		defer hCancel()
		title, err := pageTitle(hCtx, page)
		if err != nil {
			results <- crawlResult{
				err: errors.Wrapf(err, "parse page %s", url),
			}
			return
		}
		select {
		case <-localCtx.Done():
			return
		default:
		}
		lCtx, lCancel := context.WithTimeout(context.Background(), time.Duration(c.linksTimeout)*time.Second)
		defer lCancel()
		links, err := pageLinks(lCtx, nil, page)
		if err != nil {
			results <- crawlResult{
				err: errors.Wrapf(err, "parse page %s", url),
			}
			return
		}
		lCancel()
		select {
		case <-localCtx.Done():
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
