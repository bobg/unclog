import React from 'react'
import { Modal, ModalBody, ModalTitle } from 'react-bootstrap'

interface State {
  show: boolean
  text?: string
}

export class Alert extends React.Component<{}, State> {
  public state: State = { show: false }

  private onPress = () => {
    this.setState({ show: false })
  }

  public render() {
    const { show, text } = this.state

    return (
      <Modal show={show}>
        <Modal.Header>
          <ModalTitle>Alert</ModalTitle>
        </Modal.Header>
        <ModalBody>
          <div>{text}</div>
          <button onClick={() => this.setState({ show: false })}>OK</button>
        </ModalBody>
      </Modal>
    )
  }
}

let alertRef: Alert

export const setAlertRef = (r: Alert) => {
  alertRef = r
}

export const doAlert = (text: string) => {
  alertRef.setState({ show: true, text })
}
