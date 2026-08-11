.PHONY: test lint build run smoke docker

# Everything: unit suites both sides, then the whole stack as one binary.
test:
	$(MAKE) -C backend test
	cd frontend && npm test
	$(MAKE) smoke

lint:
	$(MAKE) -C backend lint
	cd frontend && npm run check

build:
	$(MAKE) -C backend build-prod

run:
	$(MAKE) -C backend run

# Builds the real artefact and drives the study loop over HTTP.
smoke: build
	./scripts/smoke.sh backend/server

docker:
	docker build -t crapcard .
