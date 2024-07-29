package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sony/gobreaker"
)

//server

type ExampleServer struct {
	addr      string
	logger    *log.Logger
	isEnabled bool
}

func NewExampleServer(addr string) *ExampleServer {
	return &ExampleServer{
		addr:      addr,
		logger:    log.New(os.Stdout, "Server\t", log.LstdFlags),
		isEnabled: true,
	}
}

func (s *ExampleServer) ListenAndServe() error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if s.isEnabled {
			s.logger.Println("responded with OK")
			w.WriteHeader(http.StatusOK)
		} else {
			s.logger.Println("responded with Error")
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/toggle", func(w http.ResponseWriter, r *http.Request) {
		s.isEnabled = !s.isEnabled
		s.logger.Println("toggled. Is enabled:", s.isEnabled)
		w.WriteHeader(http.StatusOK)
	})

	return http.ListenAndServe(s.addr, nil)
}

// Client
type NotificationClient interface {
	Send() error
}

type SmsClient struct {
	baseUrl string
}

func NewSmsClient(baseUrl string) *SmsClient {
	return &SmsClient{
		baseUrl: baseUrl,
	}
}

func (s *SmsClient) Send() error {
	url := s.baseUrl + "/"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("bad response")
	}

	return nil
}

// circuit_breaker
type ClientCircuitBreakerProxy struct {
	client NotificationClient
	logger *log.Logger
	gb     *gobreaker.CircuitBreaker
}

func shouldBeSwitchedToOpen(counts gobreaker.Counts) bool {
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return counts.Requests >= 3 && failureRatio >= 0.6
}

func NewClientCircuitBreakerProxy(client NotificationClient) *ClientCircuitBreakerProxy {
	logger := log.New(os.Stdout, "CB\t", log.LstdFlags)

	cfg := gobreaker.Settings{
		Interval:    5 * time.Second,
		Timeout:     7 * time.Second,
		ReadyToTrip: shouldBeSwitchedToOpen,
		OnStateChange: func(_ string, from gobreaker.State, to gobreaker.State) {
			logger.Println("state changed from", from.String(), "to", to.String())
		},
	}

	return &ClientCircuitBreakerProxy{
		client: client,
		logger: logger,
		gb:     gobreaker.NewCircuitBreaker(cfg),
	}
}

func (c *ClientCircuitBreakerProxy) Send() error {
	_, err := c.gb.Execute(func() (interface{}, error) {
		err := c.client.Send()
		return nil, err
	})
	return err
}

func main() {
	logger := log.New(os.Stdout, "Main\t", log.LstdFlags)
	server := NewExampleServer(":8080")

	go func() {
		_ = server.ListenAndServe()
	}()

	client := NewSmsClient("http://127.0.0.1:8080")

	for {
		err := client.Send()
		time.Sleep(1 * time.Second)
		if err != nil {
			logger.Println("caught an error", err)
		}
	}
}
