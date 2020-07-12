package unclog

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/bobg/aesite"
)

type homedata struct {
	U       *user
	Enabled bool
	Csrf    string
}

func (s *Server) handleHome(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var data homedata

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if aesite.IsNoSession(err) {
		sess, err = aesite.NewSession(ctx, s.dsClient, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("creating new session: %s", err), http.StatusInternalServerError)
			return
		}
		sess.SetCookie(w)
	} else if err != nil {
		http.Error(w, fmt.Sprintf("getting session: %s", err), http.StatusInternalServerError)
		return
	} else {
		var u user
		err = sess.GetUser(ctx, s.dsClient, &u)
		if errors.Is(err, aesite.ErrAnonymous) {
			// ok
		} else if err != nil {
			http.Error(w, fmt.Sprintf("getting session user: %s", err), http.StatusInternalServerError)
			return
		} else {
			data.U = &u
			data.Enabled = u.WatchExpiry.After(time.Now())
		}
	}

	csrf, err := sess.CSRFToken()
	if err != nil {
		http.Error(w, fmt.Sprintf("creating CSRF token: %s", err), http.StatusInternalServerError)
		return
	}
	data.Csrf = csrf

	tmpl, err := template.New("").Parse(home)
	if err != nil {
		http.Error(w, fmt.Sprintf("parsing template: %s", err), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("rendering template: %s", err), http.StatusInternalServerError)
		return
	}
}

const home = `
<html>
  <head>
    <title>Unclog</title>
  </head>
  <body>
    <h1>Unclog - U Need Contact Labeling On Gmail</h1>
    {{ if .U }}
      {{ if .Enabled }}
        <form method="POST" action="/disable">
          <input type="hidden" value="{{ .Csrf }}">
          <button type="submit">Disable</button>
        </form>
      {{ else }}
        <form method="POST" action="/enable">
          <input type="hidden" value="{{ .Csrf }}">
          <button type="submit">Enable</button>
        </form>
      {{ end }}
    {{ else }}
      <form method="POST" action="/auth">
				<p>
					Press here to get started.
					You will be asked to grant permissions to Unclog.
				</p>
				<p>
					<button type="submit">Go</button>
				</p>
      </form>
    {{ end }}
  </body>
</html>
`
