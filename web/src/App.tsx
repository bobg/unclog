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
      const { csrf, email, enabled, expired } = data
      this.setState({
        csrf,
        email,
        enabled,
        expired,
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
            ) : (
              <MyCard>
                <Card.Body>
                  <Card.Text>
                    To get started, you must authorize Unclog to access your
                    Gmail account
                  </Card.Text>
                  <Button onClick={this.auth}>Authorize</Button>
                  <Card.Text>
                    <strong>Note</strong> This preview version of Unclog has not
                    yet undergone a security review by Google. You will see a
                    screen warning that the app is not verified. If you trust
                    Unclog, you can bypass this warning by clicking “Advanced.”
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
            )}
            <MyCard>
              <Card.Body>
                <Card.Title>About Unclog</Card.Title>
                <Card.Text>Gmail has a blind spot.</Card.Text>
                <Card.Text>
                  It automatically classifies your mail in a lot of ways. It can
                  tell spam apart from social-media updates, and transaction
                  receipts apart from calendar invites.
                </Card.Text>
                <Card.Text>
                  But among all the e-mail you receive, it does not help you
                  find messages from <em>your contacts</em> — usually the
                  messages you’re most interested in seeing.
                </Card.Text>
                <Card.Text>
                  It should be able to.{' '}
                  <a
                    target='_blank'
                    rel='noopener noreferrer'
                    href='https://contacts.google.com/'
                  >
                    Google Contacts
                  </a>{' '}
                  already knows about the people closest to you. But it doesn’t.
                </Card.Text>
                <Card.Text>
                  This is where Unclog comes in. Unclog adds labels to your
                  incoming e-mail: “✔” if the sender is in your contacts, and
                  “★” if they’re <em>starred</em> in your contacts.
                </Card.Text>
                <Card.Text>
                  Select the “✔” or the “★” label view instead of your Inbox in
                  order to see correspondence with friends and family minus all
                  the other clutter.
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
