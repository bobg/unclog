package unclog

import (
	stderrs "errors"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type homedata struct {
	U       *user
	Enabled bool
	Expired bool
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
		} else {
			data.U = &u
			if u.Token != "" {
				client, err := s.oauthClient(ctx, &u)
				if err != nil {
					return errors.Wrapf(err, "creating oauth client for %s", u.Email)
				}
				gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
				if err != nil {
					return errors.Wrapf(err, "creating gmail client for %s", u.Email)
				}
				_, err = gmailSvc.Users.GetProfile("me").Do()
				if err != nil {
					log.Printf("Getting profile for %s: %s", u.Email, err)
					u.Token = ""
					_, err = s.dsClient.Put(ctx, u.Key(), &u)
					if err != nil {
						return errors.Wrapf(err, "updating user %s after token expiry", u.Email)
					}
					data.Expired = true
				} else {
					// xxx check prof.EmailAddress == u.Email?
					data.Enabled = u.WatchExpiry.After(time.Now())
				}
			}
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
    {{ if (and .U (not .Expired)) }}
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
          {{ if .Expired }}
            Unclog’s permissions have expired.
            Press to reauthorize Unclog.
          {{ else }}
						Press to get started.
						You will be asked to grant permissions to Unclog.
          {{ end }}
					<button type="submit">Go</button>
				</p>
      </form>
    {{ end }}
  </body>
</html>
`
