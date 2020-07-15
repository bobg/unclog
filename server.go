package unclog

import (
	"context"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/datastore"
	"github.com/bobg/mid"
	"golang.org/x/oauth2"
	"google.golang.org/appengine"
)

type Server struct {
	addr      string
	dsClient  *datastore.Client
	oauthConf *oauth2.Config
}

func NewServer(dsClient *datastore.Client) *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	return &Server{
		addr:     addr,
		dsClient: dsClient,
	}
}

func (s *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/", mid.Err(s.handleHome))
	mux.Handle("/auth", mid.Err(s.handleAuth))
	mux.Handle("/auth2", mid.Err(s.handleAuth2))
	mux.Handle("/enable", mid.Err(s.handleEnable))
	mux.Handle("/disable", mid.Err(s.handleDisable))
	mux.Handle("/push", mid.Err(s.handlePush))

	mux.Handle("/t/renew", mid.Log(mid.Err(s.handleRenew)))

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

func (s *Server) checkCron(req *http.Request) error {
	if !appengine.IsAppEngine() {
		return nil
	}
	h := strings.TrimSpace(req.Header.Get("X-Appengine-Cron"))
	if h != "true" {
		return mid.CodeErr{C: http.StatusUnauthorized}
	}
	return nil
}
