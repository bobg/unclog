import React from 'react'
import './App.css'
import { Loader } from 'semantic-ui-react'

import { Alert, doAlert, setAlertRef } from './Alert'
import { post } from './post'

interface State {
  csrf?: string
  email?: string
  enabled?: boolean
  expired?: boolean
  loaded: boolean
}

class App extends React.Component<{}, State> {
  public state: State = { loaded: false }

  private getData = async () => {
    try {
      const resp = await fetch('/s/data', { method: 'GET' })
      const data = await resp.json()
      const { csrf, email, enabled, expired } = data
      this.setState({ csrf, email, enabled, expired, loaded: true })
    } catch (error) {
      doAlert('Error loading data. Please try reloading this page in a moment.')
    }
  }

  private enable = async () => {
    const { csrf } = this.state
    await post('/s/enable', { csrf })
    // xxx check resp
    this.setState({ enabled: true })
  }

  private disable = async () => {
    const { csrf } = this.state
    await post('/s/disable', { csrf })
    // xxx check resp
    this.setState({ enabled: false })
  }

  private auth = async () => {
    window.location.href = '/s/auth'
  }
  private reauth = this.auth

  public componentDidMount = () => this.getData()

  public render() {
    const { email, enabled, expired, loaded } = this.state

    return (
      <div className='App'>
        <Alert ref={(r: Alert) => setAlertRef(r)} />
        <header>
          <h1>Unclog</h1>
          <h2>U Need Contact Labeling On Gmail</h2>
        </header>
        {loaded ? (
          <>
            {email ? (
              enabled ? (
                <>
                  <p>Unclog is presently enabled for {email}</p>
                  <p>
                    <button onClick={this.disable}>Disable Unclog</button>
                  </p>
                </>
              ) : expired ? (
                <>
                  <p>
                    The authorization for Unclog to access {email} has expired
                  </p>
                  <p>
                    <button onClick={this.reauth}>Reauthorize</button>
                  </p>
                </>
              ) : (
                <>
                  <p>Unclog is authorized but disabled for {email}</p>
                  <p>
                    <button onClick={this.enable}>Enable Unclog</button>
                  </p>
                </>
              )
            ) : (
              <>
                <p>
                  To get started, you must authorize Unclog to access your Gmail
                  account
                </p>
                <p>
                  <button onClick={this.auth}>Authorize</button>
                </p>
                <p>
                  <strong>Note</strong>
                  This preview version of Unclog has not yet undergone a
                  security review by Google. You will see a screen warning that
                  the app is not verified. If you trust Unclog, you can bypass
                  this warning by clicking “Advanced.”
                </p>
                <p>
                  <em>Should</em> you trust Unclog? You can decide for yourself
                  by looking at
                  <a
                    target='_blank'
                    rel='noopener noreferrer'
                    href='https://github.com/bobg/unclog'
                  >
                    Unclog’s source code on GitHub.
                  </a>
                </p>
              </>
            )}
            <p>
              Gmail classifies your incoming mail in a bunch of ways —
              identifying spam, social media updates, transaction receipts, and
              more — but strangely it doesn’t do the one most useful kind of
              automatic classification: labeling the messages that come from
              people in your{' '}
              <a
                target='_blank'
                rel='noopener noreferrer'
                href='https://contacts.google.com/'
              >
                Google Contacts
              </a>
              .
            </p>
            <p>
              This is where Unclog comes in. When you enable Unclog, it compares
              the sender of each incoming message against the e-mail addresses
              in your Google Contacts. If it finds a match, it labels the
              message with a “✔”. If the contact is “starred” in Google
              Contacts, it gets a “★” label.
            </p>
            <p>
              Now you can select the “✔” or the “★” view instead of the Inbox in
              order to see the messages most important to you, minus the clutter
              of the other messages in your Inbox.
            </p>
          </>
        ) : (
          <Loader active size='large' />
        )}
      </div>
    )
  }
}

export default App
