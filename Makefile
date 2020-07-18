test:
	go build ./cmd/unclog
	(cd web; npm run-script build)
	./unclog serve -test

check:
	go vet ./...
	(cd web; npx tsc --project . --noEmit)

deploy:
	gcloud app deploy --project unclog app.yaml cron.yaml
