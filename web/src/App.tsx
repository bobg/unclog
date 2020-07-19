import React from 'react'
import './App.css'
import { Button, Card, Spinner } from 'react-bootstrap'
import styled from 'styled-components'

import { Alert, doAlert, setAlertRef } from './Alert'
import { post } from './post'

interface State {
  csrf?: string
  email?: string
  enabled?: boolean
  expired?: boolean
  num_threads?: number
  num_labeled?: number
  loaded: boolean
}

const MyCard = styled(Card)`
  margin: auto;
  max-width: 40rem;
`

class App extends React.Component<{}, State> {
  public state: State = { loaded: false }

  private getData = async () => {
    try {
      const resp = await fetch('/s/data', { method: 'GET' })
      const data = await resp.json()
      const { csrf, email, enabled, expired, num_threads, num_labeled } = data
      this.setState({
        csrf,
        email,
        enabled,
        expired,
        num_threads,
        num_labeled,
        loaded: true,
      })
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
    const num_threads = this.state.num_threads || 0
    const num_labeled = this.state.num_labeled || 0

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
              <>
                <MyCard>
                  <Card.Body>
                    {enabled ? (
                      <>
                        <Card.Text>
                          Unclog is presently enabled for {email}
                        </Card.Text>
                        <Button onClick={this.disable}>Disable Unclog</Button>
                      </>
                    ) : expired ? (
                      <>
                        <Card.Text>
                          The authorization for Unclog to access {email} has
                          expired
                        </Card.Text>
                        <Button onClick={this.reauth}>Reauthorize</Button>
                      </>
                    ) : (
                      <>
                        <Card.Text>
                          Unclog is authorized but disabled for {email}
                        </Card.Text>
                        <Button onClick={this.enable}>Enable Unclog</Button>
                      </>
                    )}
                  </Card.Body>
                </MyCard>
                {num_threads > 0 ? (
                  <MyCard>
                    <Card.Body>
                      <Card.Text>
                        Unclog has labeled{' '}
                        {((num_labeled * 100) / num_threads).toFixed(1)}% of
                        your e-mail.
                      </Card.Text>
                    </Card.Body>
                  </MyCard>
                ) : null}
              </>
            ) : (
              <>
                <MyCard>
                  <Card.Body>
                    <Card.Text>
                      To get started, you must authorize Unclog to access your
                      Gmail account
                    </Card.Text>
                    <Button onClick={this.auth}>Authorize</Button>
                    <Card.Text>
                      <strong>Note</strong> This preview version of Unclog has
                      not yet undergone a security review by Google. You will
                      see a screen warning that the app is not verified. If you
                      trust Unclog, you can bypass this warning by clicking
                      “Advanced.”
                    </Card.Text>
                    <Card.Text>
                      <em>Should</em> you trust Unclog? You can decide for
                      yourself by looking at{' '}
                      <a
                        target='_blank'
                        rel='noopener noreferrer'
                        href='https://github.com/bobg/unclog'
                      >
                        {' '}
                        Unclog’s source code on GitHub.
                      </a>
                    </Card.Text>
                  </Card.Body>
                </MyCard>
              </>
            )}
            <MyCard>
              <Card.Body>
                <Card.Title>About Unclog</Card.Title>
                <Card.Text>
                  Gmail classifies your incoming mail in a bunch of ways —
                  identifying spam, social media updates, transaction receipts,
                  and more — but strangely it doesn’t do the one most useful
                  kind of automatic classification: labeling the messages that
                  come from people in your{' '}
                  <a
                    target='_blank'
                    rel='noopener noreferrer'
                    href='https://contacts.google.com/'
                  >
                    {' '}
                    Google Contacts
                  </a>
                  .
                </Card.Text>
                <Card.Text>
                  This is where Unclog comes in. When you enable Unclog, it
                  compares the sender of each incoming message against the
                  e-mail addresses in your Google Contacts. If it finds a match,
                  it labels the message with a “✔”. If the contact is “starred”
                  in Google Contacts, it gets a “★” label.
                </Card.Text>
                <Card.Text>
                  Now you can select the “✔” or the “★” view instead of the
                  Inbox in order to see the messages most important to you,
                  minus the clutter of the other messages in your Inbox.
                </Card.Text>
              </Card.Body>
            </MyCard>
          </>
        ) : (
          <Spinner animation='border' role='status' />
        )}
      </div>
    )
  }
}

export default App
