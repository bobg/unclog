test:
	go build ./cmd/unclog
	./unclog serve -test

deploy:
	gcloud app deploy --project unclog app.yaml cron.yaml
