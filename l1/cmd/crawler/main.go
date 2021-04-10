package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	// максимально допустимое число ошибок при парсинге
	errorsLimit = 100000

	// число результатов, которые хотим получить
	resultsLimit = 10000
)

var (
	// адрес в интернете (например, https://en.wikipedia.org/wiki/Lionel_Messi)
	url string

	// насколько глубоко нам надо смотреть (например, 10)
	depthLimit     int
	parserTimeout  int
	pageTimeout    int
	linksTimeout   int
	headerTimeout  int
	requestTimeout int
)

// Как вы помните, функция инициализации стартует первой
func init() {
	// задаём и парсим флаги
	flag.StringVar(&url, "url", "", "url address")
	flag.IntVar(&depthLimit, "depth", 3, "max depth for run")
	flag.IntVar(&parserTimeout, "parser_timeout", 240, "max processing time")
	flag.IntVar(&pageTimeout, "page_timeout", 60, "max processing time for one page")
	flag.IntVar(&linksTimeout, "links_timeout", 20, "max search time for links on one page")
	flag.IntVar(&headerTimeout, "request_timeout", 20, "max search time for a title on one page")
	flag.IntVar(&requestTimeout, "header_timeout", 15, "max load time of one page")
	flag.Parse()

	// Проверяем обязательное условие
	if url == "" {
		log.Print("no url set by flag")
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func main() {
	started := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(parserTimeout)*time.Second)

	crawler := newCrawler(depthLimit, pageTimeout, linksTimeout, headerTimeout, requestTimeout)

	go watchSignals(cancel, crawler)
	defer cancel()

	// создаём канал для результатов
	results := make(chan crawlResult)
	// создаём канал для результатов
	doneCh := make(chan struct{}, 1)

	// запускаем горутину для чтения из каналов
	done := watchCrawler(ctx, results, doneCh, errorsLimit, resultsLimit)

	// запуск основной логики
	// внутри есть рекурсивные запуски анализа в других горутинах
	wg := sync.WaitGroup{}
	crawler.run(ctx, url, results, doneCh, &wg, 0)

	// ждём завершения работы чтения в своей горутине
	<-done

	log.Println(time.Since(started))
}

// ловим сигналы выключения
func watchSignals(cancel context.CancelFunc, crawler *crawler) {
	osSignalChan := make(chan os.Signal)

	signal.Notify(osSignalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	for {
		sig := <-osSignalChan
		log.Printf("got signal %+v", sig)
		if sig == syscall.SIGUSR1 {
			crawler.increaseDepth(10)
		} else {
			log.Printf("got signal %q", sig.String())
			break
		}
	}
	cancel()
}

func watchCrawler(ctx context.Context, results <-chan crawlResult, done chan struct{}, maxErrors, maxResults int) chan struct{} {
	readersDone := make(chan struct{}, 1)

	go func() {
		defer func() {
			readersDone <- struct{}{}
			close(readersDone)
		}()
		for {
			select {
			case <-ctx.Done():
				log.Println("ctx.Done")
				return

			case result := <-results:
				if result.err != nil {
					maxErrors--
					if maxErrors <= 0 {
						log.Println("max errors exceeded")
						return
					}
					continue
				}

				log.Printf("crawling result: %v", result.msg)
				maxResults--
				if maxResults <= 0 {
					log.Println("got max results")
					return
				}
			case <-done:
				log.Println("done")
				return
			}
		}
	}()

	return readersDone
}
