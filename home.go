package unclog

import (
	stderrs "errors"
	"html/template"
	"net/http"
	"time"

	"github.com/bobg/aesite"
	"github.com/pkg/errors"
)

type homedata struct {
	U       *user
	Enabled bool
	Csrf    string
}

func (s *Server) handleHome(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	var data homedata

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if aesite.IsNoSession(err) {
		sess, err = aesite.NewSession(ctx, s.dsClient, nil)
		if err != nil {
			return errors.Wrap(err, "creating new session")
		}
		sess.SetCookie(w)
	} else if err != nil {
		return errors.Wrap(err, "getting session")
	} else {
		var u user
		err = sess.GetUser(ctx, s.dsClient, &u)
		if stderrs.Is(err, aesite.ErrAnonymous) {
			// ok
		} else if err != nil {
			return errors.Wrap(err, "getting session user")
		} else if u.Token == "" {
			// ok
		} else {
			data.U = &u
			data.Enabled = u.WatchExpiry.After(time.Now())
		}
	}

	csrf, err := sess.CSRFToken()
	if err != nil {
		return errors.Wrap(err, "creating CSRF token")
	}
	data.Csrf = csrf

	tmpl, err := template.New("").Parse(home)
	if err != nil {
		return errors.Wrap(err, "parsing template")
	}

	err = tmpl.Execute(w, data)
	return errors.Wrap(err, "rendering template")
}

const home = `
<html>
  <head>
    <title>Unclog</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
  </head>
  <body>
    <h1>Unclog - U Need Contact Labeling On Gmail</h1>
    {{ if .U }}
      {{ if .Enabled }}
        <p>
          Unclog is presently enabled for {{ .U.Email }}.
        </p>
        <form method="POST" action="/disable">
          <p>
            Press to disable Unclog.
            <input type="hidden" name="csrf" value="{{ .Csrf }}">
            <button type="submit">Disable</button>
          </p>
        </form>
      {{ else }}
        <p>
          Unclog is presently disabled for {{ .U.Email }}.
        </p>
				<form method="POST" action="/enable">
          <p>
						Press to enable Unclog.
						<input type="hidden" name="csrf" value="{{ .Csrf }}">
						<button type="submit">Enable</button>
          </p>
				</form>
      {{ end }}
    {{ else }}
      <form method="POST" action="/auth">
				<p>
					Press to get started.
					You will be asked to grant permissions to Unclog.
					<button type="submit">Go</button>
				</p>
      </form>
    {{ end }}
  </body>
</html>
`
