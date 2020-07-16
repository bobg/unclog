package unclog

import (
	"context"
	"net/http"
	"os"
	"sync"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/datastore"
	"github.com/bobg/mid"
	"golang.org/x/oauth2"
	"google.golang.org/appengine"
)

type Server struct {
	addr       string
	dsClient   *datastore.Client
	ctClient   *cloudtasks.Client
	projectID  string
	locationID string

	mu        sync.Mutex // protects the following cached values
	oauthConf *oauth2.Config
	masterKey string
}

func NewServer(dsClient *datastore.Client, ctClient *cloudtasks.Client, projectID, locationID string) *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	return &Server{
		addr:       addr,
		dsClient:   dsClient,
		ctClient:   ctClient,
		projectID:  projectID,
		locationID: locationID,
	}
}

func (s *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()

	// User-initiated.
	mux.Handle("/", mid.Err(s.handleHome))
	mux.Handle("/auth", mid.Err(s.handleAuth))
	mux.Handle("/enable", mid.Err(s.handleEnable))
	mux.Handle("/disable", mid.Err(s.handleDisable))

	// OAuth-flow-initiated.
	mux.Handle("/auth2", mid.Err(s.handleAuth2))

	// Pubsub-initiated.
	mux.Handle("/push", mid.Err(s.handlePush))

	// Cron-initiated.
	mux.Handle("/t/renew", mid.Log(mid.Err(s.handleRenew)))

	// Taskqueue-initiated.
	mux.Handle("/t/update", mid.Log(mid.Err(s.handleUpdate)))

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
