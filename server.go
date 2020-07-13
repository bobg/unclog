package unclog

import (
	"context"
	"expvar"
	"net/http"
	"net/url"
	"os"

	"cloud.google.com/go/datastore"
	"github.com/bobg/mid"
	"google.golang.org/appengine"
)

type Server struct {
	addr     string
	dsClient *datastore.Client

	pushCalls      *expvar.Int
	pushCollisions *expvar.Int
	pushErrs       *expvar.Int
	pushCumSecs    *expvar.Float
}

func NewServer(dsClient *datastore.Client) *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	return &Server{
		addr:           addr,
		dsClient:       dsClient,
		pushCalls:      expvar.NewInt("pushcalls"),
		pushCollisions: expvar.NewInt("pushcollisions"),
		pushErrs:       expvar.NewInt("pusherrs"),
		pushCumSecs:    expvar.NewFloat("pushcumsecs"),
	}
}

func (s *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/auth", s.handleAuth)
	mux.HandleFunc("/auth2", s.handleAuth2)
	mux.HandleFunc("/enable", s.handleEnable)
	mux.HandleFunc("/disable", s.handleDisable)
	mux.Handle("/push", mid.Err(s.handlePush))
	mux.Handle("/vars", expvar.Handler())

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
			Host:   "unclog.appspot.com", // TODO: acquire a domain
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
