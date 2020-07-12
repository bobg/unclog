package unclog

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"cloud.google.com/go/datastore"
	"google.golang.org/appengine"
)

type Server struct {
	addr     string
	dsClient *datastore.Client
}

func NewServer(dsClient *datastore.Client) *Server {
	return &Server{dsClient: dsClient}
}

func (s *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/auth", s.handleAuth)
	mux.HandleFunc("/auth2", s.handleAuth2)
	mux.HandleFunc("/push", s.handlePush)
	mux.HandleFunc("/enable", s.handleEnable)
	mux.HandleFunc("/disable", s.handleDisable)

	httpSrv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	if appengine.IsAppEngine() {
		return httpSrv.ListenAndServe()
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		httpSrv.Shutdown(context.TODO())
	}()

	err := httpSrv.ListenAndServe()
	<-done
	return err
}

var homeURL *url.URL

func init() {
	if appengine.IsAppEngine() {
		homeURL = &url.URL{
			Scheme: "https",
			Host:   "unclog.email", // xxx acquire domain
			Path:   "/",
		}
	} else {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		homeURL = &url.URL{
			Scheme: "http",
			Host:   "localhost:" + port,
			Path:   "/",
		}
	}
}
