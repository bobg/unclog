# Unclog - U Need Contact Labeling On Gmail

This is the code implementing [the Unclog website](https://unclog.email/) and service.

When you enable Unclog,
it automatically adds labels to your incoming messages in Gmail.
Messages from users in your Google Contacts get a “✔” label.
_Starred_ users in your Google Contacts get a “★” label.

You can use these labels as a quick way to see messages only from the people you know and care about,
without all the other clutter.

## Source tree

The top level directory contains the Go code that powers the back end of the website,
and the Unclog service itself.

Unclog runs under [Google App Engine](https://cloud.google.com/appengine),
and the top level includes config files for App Engine too.

The content of the website appears in the subtree `web`.
This includes Typescript, React, and CSS code,
plus miscellaneous other files.

The `cmd/unclog` directory contains a program used to test and administer the Unclog service.

## Theory of operation

The static contents of the website are served at the URL /,
corresponding to the contents of the directory web/build.
A bare / serves index.html.

Dynamic requests originating from the website are served at URLs beginning with /s/.

Requests originating from the cron job and the Google Cloud task queue are served at URLs beginning with /t/.

Two special URLs, /auth2 and /push, serve requests originating with Google services
(the OAuth flow and Gmail pubsub notifications, respectively).

A user who authorizes Unclog gets a Google OAuth token.
The token is used to create a pubsub subscription that notifies Unclog (at /push) when new mail arrives.

This notification causes an update task to be placed in the task queue.
A mechanism prevents multiple notifications from creating more than one update task per minute.
(See below.)

When the update task fires (at /t/update),
it first reads the user’s contacts from the Google Contacts API,
then scans newly arrived messages for any that either:

- are from e-mail addresses in the contacts, so need the proper labels added; or
- have one of these labels but _aren’t_ from addresses in the contacts, so need labels removed.

Any needed changes are made.

A cron job fires once per hour (at /t/cron).
It has two jobs:

1. Renew Gmail pubsub subscriptions that are close to expiring;
2. Queue a “catch-up” update task for any user that has had no update in over a day.

Catch-up updates appear to be needed because Gmail pubsub notifications can stop arriving
(for unknown reasons).
A catch-up update that finds that changes were needed
assumes this is what has happened
and tries to renew user’s the pubsub subscription.

## Update-limiting mechanism

There is a mechanism that prevents multiple push notifications from creating more than one update task per minute.

Each user record includes a timestamp called `NextUpdate`.

Most of the time, the value of `NextUpdate` is some time in the past.

When a new push notification arrives, the value of `NextUpdate` is checked and:

- If it is in the past (the normal case):
    - An update task is queued for right now, and
    - `NextUpdate` is set to one minute in the future.
- Otherwise, `NextUpdate` is in the future, and an update task is queued for that future time.

During the minute after the first push arrives,
all subsequent pushes will enqueue tasks for the same future time.
These tasks will all have the same `Name` field,
derived from the user’s e-mail address
and the trigger time for the task.
Google Cloud Tasks deduplicates tasks with identical names.

In this way, N pushes arriving during a given one-minute interval
will produce a single update task.
