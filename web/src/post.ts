export const post = (url: string, body: any) =>
  fetch(url, {
    method: 'POST',
    credentials: 'same-origin',
    body: JSON.stringify(body),
    headers: { 'Content-Type': 'application/json' },
  })
